#!/usr/bin/env sh
set -eu

normalize_public_base() {
  value="${1:-/}"
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

regex_escape() {
  printf '%s' "${1:-}" | sed -e 's/[][(){}.^$+*?|\\]/\\&/g'
}

fs_join() {
  root="$1"
  path="${2:-}"
  path="${path#/}"
  if [ -z "$path" ]; then
    printf '%s' "$root"
  else
    printf '%s/%s' "$root" "$path"
  fi
}

write_proxy_location() {
  path="$1"
  target="$2"
  cat <<EOF
    location = ${path} {
        proxy_set_header Host \$http_host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-Host \$http_host;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header X-Forwarded-Port \$server_port;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_connect_timeout ${NGINX_PROXY_CONNECT_TIMEOUT};
        proxy_send_timeout ${NGINX_PROXY_SEND_TIMEOUT};
        proxy_read_timeout ${NGINX_PROXY_READ_TIMEOUT};
        proxy_pass http://127.0.0.1:8081${target};
    }
EOF
}

export PMA_GATEWAY_PUBLIC_BASE_PATH="$(normalize_public_base "${PMA_GATEWAY_PUBLIC_BASE_PATH:-/}")"
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
export PMA_BASE_NO_SLASH="${PMA_BASE%/}"
export NGINX_CLIENT_MAX_BODY_SIZE="${PMA_GATEWAY_NGINX_CLIENT_MAX_BODY_SIZE:-0}"
export NGINX_PROXY_CONNECT_TIMEOUT="${PMA_GATEWAY_NGINX_PROXY_CONNECT_TIMEOUT:-300s}"
export NGINX_PROXY_READ_TIMEOUT="${PMA_GATEWAY_NGINX_PROXY_READ_TIMEOUT:-300s}"
export NGINX_PROXY_SEND_TIMEOUT="${PMA_GATEWAY_NGINX_PROXY_SEND_TIMEOUT:-300s}"
export NGINX_FASTCGI_READ_TIMEOUT="${PMA_GATEWAY_NGINX_FASTCGI_READ_TIMEOUT:-300s}"
export NGINX_FASTCGI_SEND_TIMEOUT="${PMA_GATEWAY_NGINX_FASTCGI_SEND_TIMEOUT:-300s}"
export PMA_BASE_REGEX="$(regex_escape "$PMA_BASE")"

if [ -z "$PMA_GATEWAY_PUBLIC_BASE_PATH" ]; then
  ROOT_REDIRECT_BLOCK=$(cat <<EOF
        location = / {
            return 302 ${PMA_BASE};
        }
EOF
)
  PUBLIC_ENTRY_BLOCK=""
else
  ROOT_REDIRECT_BLOCK=$(cat <<EOF
        location = / {
            return 302 ${PMA_GATEWAY_PUBLIC_BASE_PATH}/;
        }
EOF
)
  PUBLIC_ENTRY_BLOCK=$(cat <<EOF
        location = ${PMA_GATEWAY_PUBLIC_BASE_PATH} {
            return 302 ${PMA_BASE};
        }

        location = ${PMA_GATEWAY_PUBLIC_BASE_PATH}/ {
            return 302 ${PMA_BASE};
        }
EOF
)
fi
export ROOT_REDIRECT_BLOCK
export PUBLIC_ENTRY_BLOCK

PROBE_LOCATION_BLOCK=$(cat <<EOF
$(write_proxy_location "/healthz" "/healthz")

$(write_proxy_location "/readyz" "/readyz")
EOF
)
if [ "$PUBLIC_HEALTH_PATH" != "/healthz" ]; then
  PROBE_LOCATION_BLOCK="${PROBE_LOCATION_BLOCK}

$(write_proxy_location "$PUBLIC_HEALTH_PATH" "$PUBLIC_HEALTH_PATH")"
fi
if [ "$PUBLIC_READY_PATH" != "/readyz" ]; then
  PROBE_LOCATION_BLOCK="${PROBE_LOCATION_BLOCK}

$(write_proxy_location "$PUBLIC_READY_PATH" "$PUBLIC_READY_PATH")"
fi
export PROBE_LOCATION_BLOCK

web_root=/tmp/pma-gateway-www
frontend_root="$(fs_join "$web_root" "${FRONTEND_BASE%/}")"
pma_root="$(fs_join "$web_root" "$PMA_BASE_NO_SLASH")"
signon_root="$(fs_join "$web_root" "$SIGNON_URL")"

rm -rf "$web_root"
mkdir -p \
  /var/lib/pma-gateway \
  /tmp/php-sessions \
  /tmp/phpmyadmin-tmp \
  /tmp/php-conf.d \
  /tmp/nginx-client-body \
  /tmp/nginx-proxy-temp \
  /tmp/nginx-fastcgi-temp \
  /tmp/nginx-uwsgi-temp \
  /tmp/nginx-scgi-temp \
  "$web_root" \
  "$(dirname "$signon_root")"
for log_file in \
  /tmp/pma-gateway-backend.log \
  /tmp/pma-gateway-backend.err \
  /tmp/php-fpm.log \
  /tmp/php-fpm.err \
  /tmp/nginx.log \
  /tmp/nginx.err
do
  : > "$log_file"
done
ln -sfn /opt/phpmyadmin "$pma_root"
ln -sfn /opt/pma-gateway/php/pma-signon.php "$signon_root"

if [ "$PMA_GATEWAY_ENABLED" = "true" ]; then
  mkdir -p "$frontend_root"
  cp -a /opt/pma-gateway/frontend/. "$frontend_root/"
  frontend_index="${frontend_root}/index.html"
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
  fi
fi
chown -R www-data:www-data \
  /var/lib/pma-gateway \
  /tmp/php-sessions \
  /tmp/phpmyadmin-tmp \
  /tmp/php-conf.d \
  /tmp/nginx-client-body \
  /tmp/nginx-proxy-temp \
  /tmp/nginx-fastcgi-temp \
  /tmp/nginx-uwsgi-temp \
  /tmp/nginx-scgi-temp \
  /tmp/pma-gateway-backend.log \
  /tmp/pma-gateway-backend.err \
  /tmp/php-fpm.log \
  /tmp/php-fpm.err \
  /tmp/nginx.log \
  /tmp/nginx.err \
  "$web_root" 2>/dev/null || true

if [ "$PMA_GATEWAY_ENABLED" = "true" ]; then
  GATEWAY_ROUTE_BLOCK=$(cat <<EOF
        $(write_proxy_location "$FRONTEND_CONFIG_PATH" "$FRONTEND_CONFIG_PATH")

        location = ${FRONTEND_BASE%/} {
            return 301 ${FRONTEND_BASE};
        }

        location ^~ ${FRONTEND_BASE} {
            try_files \$uri \$uri/ ${FRONTEND_BASE}index.html;
        }

        location ^~ ${API_BASE}/ {
            proxy_set_header Host \$http_host;
            proxy_set_header X-Real-IP \$remote_addr;
            proxy_set_header X-Forwarded-Host \$http_host;
            proxy_set_header X-Forwarded-Proto \$scheme;
            proxy_set_header X-Forwarded-Port \$server_port;
            proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
            proxy_connect_timeout ${NGINX_PROXY_CONNECT_TIMEOUT};
            proxy_send_timeout ${NGINX_PROXY_SEND_TIMEOUT};
            proxy_read_timeout ${NGINX_PROXY_READ_TIMEOUT};
            proxy_pass http://127.0.0.1:8081;
        }

        location = ${SIGNON_URL} {
            try_files \$uri =404;
            include /etc/nginx/fastcgi_params;
            fastcgi_param SCRIPT_FILENAME \$document_root\$fastcgi_script_name;
            fastcgi_param SCRIPT_NAME \$fastcgi_script_name;
            fastcgi_param DOCUMENT_ROOT \$document_root;
            fastcgi_param PATH_INFO "";
            fastcgi_param QUERY_STRING \$query_string;
            fastcgi_param REQUEST_METHOD \$request_method;
            fastcgi_param CONTENT_TYPE \$content_type;
            fastcgi_param CONTENT_LENGTH \$content_length;
            fastcgi_param HTTP_PROXY "";
            fastcgi_param HTTP_X_FORWARDED_PROTO \$http_x_forwarded_proto;
            fastcgi_param HTTPS \$https if_not_empty;
            fastcgi_read_timeout ${NGINX_FASTCGI_READ_TIMEOUT};
            fastcgi_send_timeout ${NGINX_FASTCGI_SEND_TIMEOUT};
            fastcgi_pass 127.0.0.1:9000;
        }
EOF
)
else
  GATEWAY_ROUTE_BLOCK=$(cat <<EOF
        location = ${FRONTEND_BASE%/} {
            return 302 ${PMA_BASE};
        }

        location ^~ ${FRONTEND_BASE} {
            return 302 ${PMA_BASE};
        }

        location = ${API_BASE} {
            return 302 ${PMA_BASE};
        }

        location ^~ ${API_BASE}/ {
            return 302 ${PMA_BASE};
        }

        location = ${SIGNON_URL} {
            return 302 ${PMA_BASE};
        }
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

{
  echo 'upload_tmp_dir=/tmp'
  echo 'sys_temp_dir=/tmp'
  echo 'expose_php=Off'
  if [ -n "${PMA_GATEWAY_PHP_UPLOAD_MAX_FILESIZE:-}" ]; then
    printf 'upload_max_filesize=%s\n' "${PMA_GATEWAY_PHP_UPLOAD_MAX_FILESIZE}"
  fi
  if [ -n "${PMA_GATEWAY_PHP_POST_MAX_SIZE:-}" ]; then
    printf 'post_max_size=%s\n' "${PMA_GATEWAY_PHP_POST_MAX_SIZE}"
  fi
  if [ -n "${PMA_GATEWAY_PHP_MEMORY_LIMIT:-}" ]; then
    printf 'memory_limit=%s\n' "${PMA_GATEWAY_PHP_MEMORY_LIMIT}"
  fi
  if [ -n "${PMA_GATEWAY_PHP_MAX_EXECUTION_TIME:-}" ]; then
    printf 'max_execution_time=%s\n' "${PMA_GATEWAY_PHP_MAX_EXECUTION_TIME}"
  fi
  if [ -n "${PMA_GATEWAY_PHP_MAX_INPUT_TIME:-}" ]; then
    printf 'max_input_time=%s\n' "${PMA_GATEWAY_PHP_MAX_INPUT_TIME}"
  fi
} > /tmp/php-conf.d/pma-gateway-runtime.ini
chown www-data:www-data /tmp/php-conf.d/pma-gateway-session.ini 2>/dev/null || true
chown www-data:www-data /tmp/php-conf.d/pma-gateway-runtime.ini 2>/dev/null || true

envsubst '${NGINX_CLIENT_MAX_BODY_SIZE} ${NGINX_PROXY_CONNECT_TIMEOUT} ${NGINX_PROXY_READ_TIMEOUT} ${NGINX_PROXY_SEND_TIMEOUT} ${NGINX_FASTCGI_READ_TIMEOUT} ${NGINX_FASTCGI_SEND_TIMEOUT} ${ROOT_REDIRECT_BLOCK} ${PUBLIC_ENTRY_BLOCK} ${PROBE_LOCATION_BLOCK} ${GATEWAY_ROUTE_BLOCK} ${PMA_BASE} ${PMA_BASE_NO_SLASH} ${PMA_BASE_REGEX}' \
  < /opt/pma-gateway/nginx.conf.template > /tmp/pma-gateway-nginx.conf
chown www-data:www-data /tmp/pma-gateway-nginx.conf 2>/dev/null || true

tail -q -n 0 -F \
  /tmp/pma-gateway-backend.log \
  /tmp/pma-gateway-backend.err \
  /tmp/php-fpm.log \
  /tmp/php-fpm.err \
  /tmp/nginx.log \
  /tmp/nginx.err &

if [ "$(id -u)" = "0" ]; then
  exec gosu www-data supervisord -c /etc/supervisor/conf.d/pma-gateway.conf
fi

exec supervisord -c /etc/supervisor/conf.d/pma-gateway.conf
