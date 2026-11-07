#!/usr/bin/env bash
set -euo pipefail

image="${1:?usage: check-php-runtime.sh IMAGE}"

docker run --rm --entrypoint php "$image" -r '
if (PHP_MAJOR_VERSION !== 8 || PHP_MINOR_VERSION !== 4 || version_compare(PHP_VERSION, "8.4.27", "<")) {
    fwrite(STDERR, "Patched PHP 8.4 >= 8.4.27 is required\n"); exit(1);
}
foreach (["mysqli", "pdo_mysql", "redis"] as $extension) {
    if (!extension_loaded($extension)) {
        fwrite(STDERR, "Missing extension: $extension\n"); exit(1);
    }
}
echo "PHP ", PHP_VERSION, ": required extensions loaded\n";
'

for source in /opt/pma-gateway/php/pma-signon.php /opt/phpmyadmin/config.inc.php; do
    docker run --rm --entrypoint php "$image" -l "$source"
done

# Check the exact pool used by supervisord, rather than the unrelated CLI INI.
# An enabled global INI must not override the pool's administrative protection.
config="$(docker run --rm --entrypoint php-fpm "$image" \
    -d session.upload_progress.enabled=1 \
    --fpm-config /opt/pma-gateway/php-fpm.conf -tt 2>&1)" || {
    printf '%s\n' "$config" >&2
    exit 1
}
printf '%s\n' "$config"
if ! [[ "$config" =~ php_admin_(flag|value)\[session\.upload_progress\.enabled\][[:space:]]*=[[:space:]]*(off|0|no|false)([[:space:]]|$) ]]; then
    echo 'FPM must enforce session.upload_progress.enabled=off' >&2
    exit 1
fi
