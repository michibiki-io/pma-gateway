ARG GO_VERSION=1.26.2
ARG PHPMYADMIN_VERSION=5.2.3
ARG BUILD_COMMIT=unknown
ARG BUILD_VERSION=unknown

FROM node:24-alpine AS frontend-build
ARG PHPMYADMIN_VERSION
ARG BUILD_COMMIT
ARG BUILD_VERSION
WORKDIR /src/frontend
ENV PHPMYADMIN_VERSION=${PHPMYADMIN_VERSION}
ENV BUILD_COMMIT=${BUILD_COMMIT}
ENV BUILD_VERSION=${BUILD_VERSION}
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS backend-build
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum* ./
RUN go mod download
COPY backend/ ./
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /out/pma-gateway ./cmd/pma-gateway

FROM alpine:3.22 AS phpmyadmin-fetch
ARG PHPMYADMIN_VERSION
RUN apk add --no-cache ca-certificates tar wget \
    && wget -O /tmp/phpmyadmin.tar.gz "https://files.phpmyadmin.net/phpMyAdmin/${PHPMYADMIN_VERSION}/phpMyAdmin-${PHPMYADMIN_VERSION}-all-languages.tar.gz" \
    && mkdir -p /out \
    && tar -xzf /tmp/phpmyadmin.tar.gz -C /out --strip-components=1

FROM php:8.3-apache-bookworm AS runtime
ARG PHPMYADMIN_VERSION
ARG BUILD_COMMIT
ARG BUILD_VERSION
ENV PMA_GATEWAY_LISTEN_ADDR=127.0.0.1:8081 \
    PMA_GATEWAY_INTERNAL_REDEEM_URL=http://127.0.0.1:8081/internal/v1/signon/redeem \
    PMA_GATEWAY_DATA_DIR=/var/lib/pma-gateway \
    PMA_GATEWAY_PHPMYADMIN_VERSION=${PHPMYADMIN_VERSION} \
    BUILD_COMMIT=${BUILD_COMMIT} \
    BUILD_VERSION=${BUILD_VERSION} \
    APACHE_RUN_DIR=/tmp/apache2-run \
    APACHE_LOCK_DIR=/tmp/apache2-lock \
    APACHE_PID_FILE=/tmp/apache2-run/apache2.pid \
    APACHE_LOG_DIR=/tmp \
    PHP_INI_SCAN_DIR=/usr/local/etc/php/conf.d:/tmp/php-conf.d \
    PHP_SESSION_SAVE_PATH=/tmp/php-sessions

RUN set -eux; \
    apt-get update; \
    apt-get install -y --no-install-recommends supervisor gosu gettext-base ca-certificates ${PHPIZE_DEPS}; \
    docker-php-ext-install mysqli pdo_mysql; \
    pecl install redis; \
    docker-php-ext-enable redis; \
    a2enmod rewrite proxy proxy_http headers expires; \
    a2dissite 000-default; \
    sed -i 's/^Listen 80$/Listen 8080/' /etc/apache2/ports.conf; \
    printf '\nIncludeOptional /tmp/pma-gateway-apache.conf\n' >> /etc/apache2/apache2.conf; \
    { \
      echo 'session.save_path=/tmp/php-sessions'; \
      echo 'upload_tmp_dir=/tmp'; \
      echo 'expose_php=Off'; \
    } > /usr/local/etc/php/conf.d/pma-gateway.ini; \
    apt-get purge -y --auto-remove ${PHPIZE_DEPS}; \
    rm -rf /var/lib/apt/lists/*

COPY --from=frontend-build /src/frontend/dist/ /opt/pma-gateway/frontend/
COPY --from=backend-build /out/pma-gateway /opt/pma-gateway/bin/pma-gateway
COPY --from=phpmyadmin-fetch /out/ /opt/phpmyadmin/
COPY phpmyadmin/config.user.inc.php.tmpl /opt/phpmyadmin/config.inc.php
COPY phpmyadmin/pma-signon.php /opt/pma-gateway/php/pma-signon.php
COPY docker/apache-site.conf.template /opt/pma-gateway/apache-site.conf.template
COPY docker/supervisord.conf /etc/supervisor/conf.d/pma-gateway.conf
COPY docker/entrypoint.sh /opt/pma-gateway/entrypoint.sh

RUN set -eux; \
    mkdir -p /opt/pma-gateway/empty /var/lib/pma-gateway /tmp/php-sessions /tmp/php-conf.d /tmp/apache2-run /tmp/apache2-lock; \
    chown -R www-data:www-data /opt/pma-gateway /opt/phpmyadmin /var/lib/pma-gateway /tmp/php-sessions /tmp/php-conf.d /tmp/apache2-run /tmp/apache2-lock; \
    chmod +x /opt/pma-gateway/bin/pma-gateway /opt/pma-gateway/entrypoint.sh

EXPOSE 8080
VOLUME ["/var/lib/pma-gateway"]
ENTRYPOINT ["/opt/pma-gateway/entrypoint.sh"]
