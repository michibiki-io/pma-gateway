<?php
declare(strict_types=1);

function gateway_env(string $name, string $fallback = ''): string {
    $value = getenv($name);
    return $value === false || $value === '' ? $fallback : $value;
}

function gateway_secret(string $valueName, string $fileName, string $fallback = ''): string {
    $value = getenv($valueName);
    $file = getenv($fileName);
    if ($value !== false && $value !== '') {
        return trim($value);
    }
    if ($file !== false && $file !== '' && is_readable($file)) {
        return trim((string) file_get_contents($file));
    }
    return $fallback;
}

function gateway_public_base(): string {
    $base = trim(gateway_env('PMA_GATEWAY_PUBLIC_BASE_PATH', '/dbadmin'));
    if ($base === '' || $base === '/') {
        return '';
    }
    return '/' . trim($base, '/');
}

function gateway_subpath(string $name, string $fallback): string {
    $value = trim(gateway_env($name, $fallback));
    if ($value === '' || $value === '/') {
        return '/';
    }
    return '/' . trim($value, '/');
}

function gateway_join(string ...$parts): string {
    $items = [];
    foreach ($parts as $part) {
        $part = trim($part, '/');
        if ($part !== '') {
            $items[] = $part;
        }
    }
    return '/' . implode('/', $items);
}

function gateway_redirect(string $path): void {
    header('Location: ' . $path, true, 302);
    exit;
}

function gateway_fail(string $frontendBase): void {
    gateway_redirect($frontendBase . '?error=signon_failed');
}

function gateway_expire_cookie(string $name, array $paths, bool $secure, string $sameSite): void {
    $seen = [];
    foreach ($paths as $path) {
        if ($path === '' || isset($seen[$path])) {
            continue;
        }
        $seen[$path] = true;
        setcookie($name, '', [
            'expires' => time() - 3600,
            'path' => $path,
            'secure' => $secure,
            'httponly' => true,
            'samesite' => $sameSite,
        ]);
    }
}

$publicBase = gateway_public_base();
$frontendBase = rtrim(gateway_join($publicBase, gateway_subpath('PMA_GATEWAY_FRONTEND_PATH', '/_gateway')), '/') . '/';
$pmaBase = rtrim(gateway_join($publicBase, gateway_subpath('PMA_GATEWAY_PMA_PATH', '/_pma')), '/') . '/';
$cookiePath = $publicBase === '' ? '/' : $publicBase . '/';
$ticket = isset($_GET['ticket']) ? trim((string) $_GET['ticket']) : '';

if ($ticket === '') {
    gateway_fail($frontendBase);
}

$internalSecret = gateway_secret('PMA_GATEWAY_INTERNAL_SHARED_SECRET', 'PMA_GATEWAY_INTERNAL_SHARED_SECRET_FILE');
$internalURL = gateway_env('PMA_GATEWAY_INTERNAL_REDEEM_URL', 'http://127.0.0.1:8080/internal/v1/signon/redeem');
$payload = json_encode(['ticket' => $ticket], JSON_THROW_ON_ERROR);
$httpOptions = [
    'http' => [
        'method' => 'POST',
        'header' => [
            'Content-Type: application/json',
            'Accept: application/json',
            'X-PMA-Gateway-Internal-Secret: ' . $internalSecret,
        ],
        'content' => $payload,
        'timeout' => 10,
        'ignore_errors' => true,
    ],
];

$response = @file_get_contents($internalURL, false, stream_context_create($httpOptions));
if ($response === false) {
    gateway_fail($frontendBase);
}

$status = 0;
if (isset($http_response_header) && is_array($http_response_header)) {
    foreach ($http_response_header as $header) {
        if (preg_match('/^HTTP\/\S+\s+(\d+)/', $header, $matches)) {
            $status = (int) $matches[1];
            break;
        }
    }
}
if ($status < 200 || $status >= 300) {
    gateway_fail($frontendBase);
}

$credential = json_decode((string) $response, true);
if (!is_array($credential)
    || !isset($credential['dbHost'], $credential['dbPort'], $credential['dbUser'], $credential['dbPassword'])) {
    gateway_fail($frontendBase);
}

$secure = (!empty($_SERVER['HTTPS']) && $_SERVER['HTTPS'] !== 'off')
    || strtolower($_SERVER['HTTP_X_FORWARDED_PROTO'] ?? '') === 'https';

gateway_expire_cookie('phpMyAdmin', [$pmaBase, rtrim($pmaBase, '/'), $cookiePath, '/'], $secure, 'Strict');

session_name('PmaGatewaySignon');
session_set_cookie_params([
    'lifetime' => 0,
    'path' => $cookiePath,
    'secure' => $secure,
    'httponly' => true,
    'samesite' => 'Lax',
]);
session_start();
$_SESSION['PMA_single_signon_user'] = (string) $credential['dbUser'];
$_SESSION['PMA_single_signon_password'] = (string) $credential['dbPassword'];
$_SESSION['PMA_single_signon_host'] = (string) $credential['dbHost'];
$_SESSION['PMA_single_signon_port'] = (string) $credential['dbPort'];
$_SESSION['PMA_single_signon_HMAC_secret'] = hash('sha256', $internalSecret . ':' . session_id());
session_write_close();

unset($credential, $internalSecret, $ticket, $payload, $response);
gateway_redirect($pmaBase);
