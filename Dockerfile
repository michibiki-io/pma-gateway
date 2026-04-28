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

FROM php:8.4-fpm-trixie AS runtime
ARG PHPMYADMIN_VERSION
ARG BUILD_COMMIT
ARG BUILD_VERSION
ENV PMA_GATEWAY_LISTEN_ADDR=127.0.0.1:8081 \
    PMA_GATEWAY_INTERNAL_REDEEM_URL=http://127.0.0.1:8081/internal/v1/signon/redeem \
    PMA_GATEWAY_DATA_DIR=/var/lib/pma-gateway \
    PMA_GATEWAY_PHPMYADMIN_VERSION=${PHPMYADMIN_VERSION} \
    BUILD_COMMIT=${BUILD_COMMIT} \
    BUILD_VERSION=${BUILD_VERSION} \
    PHP_INI_SCAN_DIR=/usr/local/etc/php/conf.d:/tmp/php-conf.d \
    PHP_SESSION_SAVE_PATH=/tmp/php-sessions

RUN set -eux; \
    apt-get update; \
    apt-get install -y --no-install-recommends supervisor gosu gettext-base ca-certificates nginx-light ${PHPIZE_DEPS}; \
    docker-php-ext-install mysqli pdo_mysql; \
    pecl install redis; \
    docker-php-ext-enable redis; \
    apt-get purge -y --auto-remove ${PHPIZE_DEPS}; \
    rm -rf /var/lib/apt/lists/*

COPY --from=frontend-build /src/frontend/dist/ /opt/pma-gateway/frontend/
COPY --from=backend-build /out/pma-gateway /opt/pma-gateway/bin/pma-gateway
COPY --from=phpmyadmin-fetch /out/ /opt/phpmyadmin/
COPY phpmyadmin/config.user.inc.php.tmpl /opt/phpmyadmin/config.inc.php
COPY phpmyadmin/pma-signon.php /opt/pma-gateway/php/pma-signon.php
COPY docker/nginx.conf.template /opt/pma-gateway/nginx.conf.template
COPY docker/php-fpm.conf /opt/pma-gateway/php-fpm.conf
COPY docker/supervisord.conf /etc/supervisor/conf.d/pma-gateway.conf
COPY docker/entrypoint.sh /opt/pma-gateway/entrypoint.sh

RUN set -eux; \
    mkdir -p /var/lib/pma-gateway /tmp/php-sessions /tmp/php-conf.d; \
    chown -R www-data:www-data /opt/pma-gateway /opt/phpmyadmin /var/lib/pma-gateway /tmp/php-sessions /tmp/php-conf.d; \
    chmod +x /opt/pma-gateway/bin/pma-gateway /opt/pma-gateway/entrypoint.sh

EXPOSE 8080
VOLUME ["/var/lib/pma-gateway"]
ENTRYPOINT ["/opt/pma-gateway/entrypoint.sh"]
