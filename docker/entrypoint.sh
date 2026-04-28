#!/usr/bin/env sh
set -eu

normalize_public_base() {
  value="${1:-/dbadmin}"
  value="$(printf '%s' "$value" | sed -e 's#//*#/#g' -e 's#/$##')"
  if [ "$value" = "/" ] || [ -z "$value" ]; then
    printf ''
  else
    case "$value" in
      /*) printf '%s' "$value" ;;
      *) printf '/%s' "$value" ;;
    esac
  fi
}

normalize_subpath() {
  value="${1:-/}"
  value="$(printf '%s' "$value" | sed -e 's#//*#/#g' -e 's#/$##')"
  if [ -z "$value" ]; then
    printf '/'
  else
    case "$value" in
      /*) printf '%s' "$value" ;;
      *) printf '/%s' "$value" ;;
    esac
  fi
}

join_path() {
  base="$1"
  sub="$2"
  if [ -z "$base" ]; then
    printf '%s' "$sub"
  elif [ "$sub" = "/" ]; then
    printf '%s' "$base"
  else
    printf '%s%s' "$base" "$sub"
  fi
}

secret_value() {
  value_name="$1"
  file_name="$2"
  eval value="\${$value_name:-}"
  eval file="\${$file_name:-}"
  if [ -n "$value" ]; then
    printf '%s' "$value"
    return
  fi
  if [ -n "$file" ] && [ -r "$file" ]; then
    tr -d '\r\n' < "$file"
  fi
}

generate_random_secret() {
  head -c 32 /dev/urandom | base64 | tr -d '\r\n'
}

is_truthy() {
  value="$(printf '%s' "${1:-}" | tr '[:upper:]' '[:lower:]')"
  case "$value" in
    1|true|yes|on) return 0 ;;
    *) return 1 ;;
  esac
}

export PMA_GATEWAY_PUBLIC_BASE_PATH="$(normalize_public_base "${PMA_GATEWAY_PUBLIC_BASE_PATH:-/dbadmin}")"
export PMA_GATEWAY_PMA_PATH="$(normalize_subpath "${PMA_GATEWAY_PMA_PATH:-/_pma}")"
export PMA_GATEWAY_FRONTEND_PATH="$(normalize_subpath "${PMA_GATEWAY_FRONTEND_PATH:-/_gateway}")"
export PMA_GATEWAY_API_PATH="$(normalize_subpath "${PMA_GATEWAY_API_PATH:-/_api}")"
export PMA_GATEWAY_SIGNON_PATH="$(normalize_subpath "${PMA_GATEWAY_SIGNON_PATH:-/_signon.php}")"
if is_truthy "${PMA_GATEWAY:-true}"; then
  export PMA_GATEWAY_ENABLED=true
else
  export PMA_GATEWAY_ENABLED=false
fi

internal_shared_secret="$(secret_value PMA_GATEWAY_INTERNAL_SHARED_SECRET PMA_GATEWAY_INTERNAL_SHARED_SECRET_FILE)"
if [ -z "$internal_shared_secret" ] && is_truthy "${PMA_GATEWAY_DEV_INSECURE_EPHEMERAL_KEY:-false}"; then
  export PMA_GATEWAY_INTERNAL_SHARED_SECRET="$(generate_random_secret)"
fi

PMA_BASE_RAW="$(join_path "$PMA_GATEWAY_PUBLIC_BASE_PATH" "$PMA_GATEWAY_PMA_PATH")"
FRONTEND_BASE_RAW="$(join_path "$PMA_GATEWAY_PUBLIC_BASE_PATH" "$PMA_GATEWAY_FRONTEND_PATH")"
API_BASE_RAW="$(join_path "$PMA_GATEWAY_PUBLIC_BASE_PATH" "$PMA_GATEWAY_API_PATH")/v1"
SIGNON_RAW="$(join_path "$PMA_GATEWAY_PUBLIC_BASE_PATH" "$PMA_GATEWAY_SIGNON_PATH")"

export PMA_BASE="${PMA_BASE_RAW%/}/"
export FRONTEND_BASE="${FRONTEND_BASE_RAW%/}/"
export API_BASE="${API_BASE_RAW%/}"
export SIGNON_URL="$SIGNON_RAW"
export FRONTEND_CONFIG_PATH="${FRONTEND_BASE}config.js"
export PUBLIC_HEALTH_PATH="$(join_path "$PMA_GATEWAY_PUBLIC_BASE_PATH" "/healthz")"
export PUBLIC_READY_PATH="$(join_path "$PMA_GATEWAY_PUBLIC_BASE_PATH" "/readyz")"

if [ -z "$PMA_GATEWAY_PUBLIC_BASE_PATH" ]; then
  export PUBLIC_ENTRY_REGEX="^/$"
  export ROOT_ENTRY_REDIRECT=""
else
  export PUBLIC_ENTRY_REGEX="^${PMA_GATEWAY_PUBLIC_BASE_PATH}/?$"
  export ROOT_ENTRY_REDIRECT="RedirectMatch 302 \"^/$\" \"${PMA_GATEWAY_PUBLIC_BASE_PATH}/\""
fi

mkdir -p /var/lib/pma-gateway /tmp/php-sessions /tmp/php-conf.d /tmp/apache2-run /tmp/apache2-lock
chown -R www-data:www-data /var/lib/pma-gateway /tmp/php-sessions /tmp/php-conf.d /tmp/apache2-run /tmp/apache2-lock 2>/dev/null || true

frontend_index=/opt/pma-gateway/frontend/index.html
if [ -f "$frontend_index" ]; then
  frontend_index_tmp="$(mktemp)"
  awk -v base="$FRONTEND_BASE" '
    /<base href=/ { next }
    /<head>/ && !inserted {
      print
      print "    <base href=\"" base "\" />"
      inserted = 1
      next
    }
    { print }
  ' "$frontend_index" > "$frontend_index_tmp"
  mv "$frontend_index_tmp" "$frontend_index"
  chmod 0644 "$frontend_index"
  chown www-data:www-data "$frontend_index" 2>/dev/null || true
fi

if [ "$PMA_GATEWAY_ENABLED" = "true" ]; then
  mkdir -p /opt/pma-gateway/frontend/admin
  ln -sf index.html /opt/pma-gateway/frontend/account
  ln -sf ../index.html /opt/pma-gateway/frontend/admin/credentials
  ln -sf ../index.html /opt/pma-gateway/frontend/admin/mappings
  ln -sf ../index.html /opt/pma-gateway/frontend/admin/audit
fi

if [ "$PMA_GATEWAY_ENABLED" = "true" ]; then
  GATEWAY_ROUTE_BLOCK=$(cat <<EOF
    ProxyPass "${FRONTEND_CONFIG_PATH}" "http://127.0.0.1:8081${FRONTEND_CONFIG_PATH}" retry=0
    ProxyPassReverse "${FRONTEND_CONFIG_PATH}" "http://127.0.0.1:8081${FRONTEND_CONFIG_PATH}"
    ProxyPass "${API_BASE}/" "http://127.0.0.1:8081${API_BASE}/" retry=0
    ProxyPassReverse "${API_BASE}/" "http://127.0.0.1:8081${API_BASE}/"

    Alias "${SIGNON_URL}" "/opt/pma-gateway/php/pma-signon.php"
    <Files "pma-signon.php">
        Require all granted
    </Files>

    Alias "${FRONTEND_BASE}" "/opt/pma-gateway/frontend/"
    <Directory "/opt/pma-gateway/frontend">
        Options FollowSymLinks
        AllowOverride None
        DirectoryIndex index.html
        FallbackResource ${FRONTEND_BASE}index.html
        Require all granted
    </Directory>
EOF
)
else
  GATEWAY_ROUTE_BLOCK=$(cat <<EOF
    RedirectMatch 302 "^${FRONTEND_BASE%/}(/.*)?$" "${PMA_BASE}"
    RedirectMatch 302 "^${API_BASE}(/.*)?$" "${PMA_BASE}"
    RedirectMatch 302 "^${SIGNON_URL}$" "${PMA_BASE}"
EOF
)
fi
export GATEWAY_ROUTE_BLOCK

php_session_gc_maxlifetime="${PMA_GATEWAY_PHP_SESSION_GC_MAXLIFETIME:-${PMA_GATEWAY_PHPMYADMIN_LOGIN_COOKIE_VALIDITY:-3600}}"
php_session_cookie_path="/"
if [ -n "$PMA_GATEWAY_PUBLIC_BASE_PATH" ]; then
  php_session_cookie_path="${PMA_GATEWAY_PUBLIC_BASE_PATH}/"
fi
session_store="${PMA_GATEWAY_PHP_SESSION_STORE:-files}"
if [ "$session_store" = "redis" ]; then
  redis_locking_enabled="${PMA_GATEWAY_REDIS_SESSION_LOCKING_ENABLED:-1}"
  redis_lock_retries="${PMA_GATEWAY_REDIS_SESSION_LOCK_RETRIES:-100}"
  redis_lock_wait_time="${PMA_GATEWAY_REDIS_SESSION_LOCK_WAIT_TIME:-50000}"
  redis_lock_expire="${PMA_GATEWAY_REDIS_SESSION_LOCK_EXPIRE:-30}"
  redis_url="$(secret_value PMA_GATEWAY_REDIS_SESSION_URL PMA_GATEWAY_REDIS_SESSION_URL_FILE)"
  if [ -z "$redis_url" ]; then
    redis_host="${PMA_GATEWAY_REDIS_HOST:-redis}"
    redis_port="${PMA_GATEWAY_REDIS_PORT:-6379}"
    redis_db="${PMA_GATEWAY_REDIS_DATABASE:-0}"
    redis_prefix="${PMA_GATEWAY_REDIS_PREFIX:-pma-gateway:}"
    redis_password="$(secret_value PMA_GATEWAY_REDIS_PASSWORD PMA_GATEWAY_REDIS_PASSWORD_FILE)"
    redis_url="tcp://${redis_host}:${redis_port}?database=${redis_db}&prefix=${redis_prefix}&persistent=1&timeout=2.5&read_timeout=2.5"
    if [ -n "$redis_password" ]; then
      redis_url="${redis_url}&auth=${redis_password}"
    fi
  fi
  {
    printf 'session.gc_maxlifetime=%s\n' "$php_session_gc_maxlifetime"
    printf 'session.cookie_path="%s"\n' "$php_session_cookie_path"
    echo 'session.save_handler=redis'
    printf 'session.save_path="%s"\n' "$redis_url"
    printf 'redis.session.locking_enabled=%s\n' "$redis_locking_enabled"
    printf 'redis.session.lock_retries=%s\n' "$redis_lock_retries"
    printf 'redis.session.lock_wait_time=%s\n' "$redis_lock_wait_time"
    printf 'redis.session.lock_expire=%s\n' "$redis_lock_expire"
  } > /tmp/php-conf.d/pma-gateway-session.ini
else
  {
    printf 'session.gc_maxlifetime=%s\n' "$php_session_gc_maxlifetime"
    printf 'session.cookie_path="%s"\n' "$php_session_cookie_path"
    echo 'session.save_handler=files'
    printf 'session.save_path="%s"\n' "${PHP_SESSION_SAVE_PATH:-/tmp/php-sessions}"
  } > /tmp/php-conf.d/pma-gateway-session.ini
fi
chown www-data:www-data /tmp/php-conf.d/pma-gateway-session.ini 2>/dev/null || true

envsubst '${PUBLIC_HEALTH_PATH} ${PUBLIC_READY_PATH} ${ROOT_ENTRY_REDIRECT} ${PUBLIC_ENTRY_REGEX} ${PMA_BASE} ${GATEWAY_ROUTE_BLOCK}' \
  < /opt/pma-gateway/apache-site.conf.template > /tmp/pma-gateway-apache.conf
chown www-data:www-data /tmp/pma-gateway-apache.conf 2>/dev/null || true

if [ "$(id -u)" = "0" ]; then
  exec gosu www-data supervisord -c /etc/supervisor/conf.d/pma-gateway.conf
fi

exec supervisord -c /etc/supervisor/conf.d/pma-gateway.conf
