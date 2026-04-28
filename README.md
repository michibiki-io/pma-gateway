# pma-gateway

`pma-gateway` is a header-authenticated access gateway for phpMyAdmin. It lets an upstream authentication proxy identify a user, maps that user or their groups to managed database credentials, and signs the browser into phpMyAdmin without ever returning database passwords to the browser.

## Architecture

The runtime image contains:

- Go backend API
- Svelte static frontend
- phpMyAdmin configured with `signon` auth
- nginx public HTTP server
- PHP-FPM for phpMyAdmin and signon execution
- supervisord process supervision
- SQLite metadata storage

Default public layout:

```text
/                     Unified public entry URL
/_pma/                phpMyAdmin
/_gateway/            pma-gateway frontend
/_api/v1/             backend API
/_signon.php          phpMyAdmin signon bridge
```

The `/` entry redirects into phpMyAdmin. If phpMyAdmin has no valid session, phpMyAdmin redirects to the gateway frontend through `SignonURL`. The user chooses an allowed credential, the backend issues a short-lived one-time ticket, and the PHP signon bridge redeems that ticket over localhost using an internal shared secret.

## Local Development

Copy `dev/.env.example` to `dev/.env` before starting the stack, then write the actual values there. The compose files read runtime environment values from `dev/.env`, not from the example file. If you access the gateway through a non-default origin, update `PMA_GATEWAY_ALLOWED_ORIGINS` in `dev/.env` to match the browser origin exactly, including the port. For the default compose setup:

```text
PMA_GATEWAY_ALLOWED_ORIGINS=http://localhost:8080
```

```bash
cp dev/.env.example dev/.env
docker compose up --build
```

Open:

```text
http://localhost:8080/
```

The development proxy injects fake auth headers. The default user is `alice@example.com` with `db-users,db-admins`. To view as a non-admin user for the initial request:

```text
http://localhost:8080/?as=bob
```

The compose stack starts:

- `pma-gateway`
- `mariadb`
- `dev-auth-proxy`

MariaDB is initialized with `sampledb`, a readonly user, and an admin-like user. Bootstrap credentials and mappings are loaded from [dev/bootstrap.json](/home/staratlas@ad.michibiki.io/workspace/pma-gateway/dev/bootstrap.json).

## Configuration

Important environment variables:

```text
PMA_GATEWAY=true
PMA_GATEWAY_PUBLIC_BASE_PATH=/
PMA_GATEWAY_PMA_PATH=/_pma
PMA_GATEWAY_FRONTEND_PATH=/_gateway
PMA_GATEWAY_API_PATH=/_api
PMA_GATEWAY_SIGNON_PATH=/_signon.php
PMA_GATEWAY_PMA_HOST=
PMA_GATEWAY_PMA_PORT=
PMA_GATEWAY_PMA_ALLOW_ARBITRARY_SERVER=true
PMA_GATEWAY_PHPMYADMIN_ALLOW_THIRD_PARTY_FRAMING=sameorigin
PMA_GATEWAY_MASTER_KEY_BASE64=
PMA_GATEWAY_MASTER_KEY_FILE=
PMA_GATEWAY_INTERNAL_SHARED_SECRET=
PMA_GATEWAY_INTERNAL_SHARED_SECRET_FILE=
PMA_GATEWAY_USER_HEADER=Remote-User
PMA_GATEWAY_GROUPS_HEADER=Remote-Groups
PMA_GATEWAY_GROUPS_SEPARATOR=,
PMA_GATEWAY_ADMIN_USERS=
PMA_GATEWAY_ADMIN_GROUPS=
PMA_GATEWAY_TRUSTED_PROXY_CIDRS=127.0.0.1/32,::1/128
PMA_GATEWAY_TRUST_PROXY_HEADERS=true
PMA_GATEWAY_ALLOWED_ORIGINS=
PMA_GATEWAY_APPCHECK_MODE=disabled
PMA_GATEWAY_APPCHECK_VERIFIED_HEADER=X-AppCheck-Verified
PMA_GATEWAY_SIGNON_TICKET_TTL_SECONDS=60
PMA_GATEWAY_CREDENTIAL_TEST_TIMEOUT_SECONDS=10
PMA_GATEWAY_TIMESTAMP_FORMAT=2006-01-02 15:04:05 MST
PMA_GATEWAY_TIMESTAMP_TIMEZONE=Asia/Tokyo
PMA_GATEWAY_PHPMYADMIN_LOGIN_COOKIE_VALIDITY=3600
PMA_GATEWAY_PHP_SESSION_GC_MAXLIFETIME=3600
PMA_GATEWAY_PHP_UPLOAD_MAX_FILESIZE=
PMA_GATEWAY_PHP_POST_MAX_SIZE=
PMA_GATEWAY_PHP_MEMORY_LIMIT=
PMA_GATEWAY_PHP_MAX_EXECUTION_TIME=
PMA_GATEWAY_PHP_MAX_INPUT_TIME=
```

`PMA_GATEWAY_PUBLIC_BASE_PATH` defaults to `/`. The backend and generated nginx routing avoid double slashes for root-base deployments.

`PMA_GATEWAY=false` switches phpMyAdmin to direct login mode. In that mode:

- `/` still redirects to phpMyAdmin, but the phpMyAdmin login screen is shown instead of the gateway UI
- `/_gateway`, `/_api`, and `/_signon.php` are redirected back to phpMyAdmin
- phpMyAdmin uses `auth_type=cookie`
- if `PMA_GATEWAY_PMA_HOST` is set, that host/port is used as the fixed login target
- if `PMA_GATEWAY_PMA_HOST` is empty, `PMA_GATEWAY_PMA_ALLOW_ARBITRARY_SERVER=true` lets the user choose the server on the login screen

`PMA_GATEWAY_PHPMYADMIN_ALLOW_THIRD_PARTY_FRAMING` controls the phpMyAdmin `X-Frame-Options` behavior. Supported values are `false`, `sameorigin`, and `true`. The default is `sameorigin`, which allows iframe embedding only from the same origin.

Timestamp values returned by the gateway UI/API are formatted at response time with `PMA_GATEWAY_TIMESTAMP_FORMAT` and `PMA_GATEWAY_TIMESTAMP_TIMEZONE`. Stored values remain RFC3339 internally for ordering and filtering. The default display is `2001-01-01 10:00:00 JST`.

Credential connection tests from the admin UI use `PMA_GATEWAY_CREDENTIAL_TEST_TIMEOUT_SECONDS`. The default timeout is `10` seconds.

`PMA_GATEWAY_PHP_SESSION_GC_MAXLIFETIME` should be equal to or greater than `PMA_GATEWAY_PHPMYADMIN_LOGIN_COOKIE_VALIDITY`, otherwise phpMyAdmin warns that the PHP session may expire before the login cookie.

Secret-like values can be loaded directly or through `_FILE` variants, which is preferred for Kubernetes Secrets:

```text
PMA_GATEWAY_MASTER_KEY_FILE=/var/run/secrets/pma-gateway/master-key
PMA_GATEWAY_INTERNAL_SHARED_SECRET_FILE=/var/run/secrets/pma-gateway/internal-shared-secret
PMA_GATEWAY_BOOTSTRAP_CONFIG_FILE=/config/bootstrap.json
```

The master key must base64-decode to 32 bytes. In production the backend fails startup if no key is provided. Development-only ephemeral keys require:

```text
PMA_GATEWAY_DEV_INSECURE_EPHEMERAL_KEY=true
```

## External MySQL and Redis

SQLite plus local PHP sessions are intended for single-replica deployments. To run multiple pods, move both shared state stores out of the container:

- gateway metadata: MySQL
- PHP/phpMyAdmin sessions: Redis

Use MySQL for backend storage:

```text
PMA_GATEWAY_DATABASE_DRIVER=mysql
PMA_GATEWAY_DATABASE_DSN=
PMA_GATEWAY_DATABASE_DSN_FILE=
PMA_GATEWAY_MYSQL_HOST=mysql.example.internal
PMA_GATEWAY_MYSQL_PORT=3306
PMA_GATEWAY_MYSQL_DATABASE=pma_gateway
PMA_GATEWAY_MYSQL_USER=pma_gateway
PMA_GATEWAY_MYSQL_PASSWORD=
PMA_GATEWAY_MYSQL_PASSWORD_FILE=
```

`PMA_GATEWAY_DATABASE_DSN` takes precedence. If it is unset, the component variables are used to build a MySQL DSN. Use `PMA_GATEWAY_DATABASE_DSN_FILE` or `PMA_GATEWAY_MYSQL_PASSWORD_FILE` for Kubernetes Secrets.

Use Redis for PHP sessions:

```text
PMA_GATEWAY_PHP_SESSION_STORE=redis
PMA_GATEWAY_REDIS_SESSION_URL=
PMA_GATEWAY_REDIS_SESSION_URL_FILE=
PMA_GATEWAY_REDIS_HOST=redis.example.internal
PMA_GATEWAY_REDIS_PORT=6379
PMA_GATEWAY_REDIS_DATABASE=0
PMA_GATEWAY_REDIS_PREFIX=pma-gateway:
PMA_GATEWAY_REDIS_PASSWORD=
PMA_GATEWAY_REDIS_PASSWORD_FILE=
PMA_GATEWAY_REDIS_SESSION_LOCKING_ENABLED=1
PMA_GATEWAY_REDIS_SESSION_LOCK_RETRIES=100
PMA_GATEWAY_REDIS_SESSION_LOCK_WAIT_TIME=50000
PMA_GATEWAY_REDIS_SESSION_LOCK_EXPIRE=30
```

`PMA_GATEWAY_REDIS_SESSION_URL` takes precedence. Use it when Redis auth or connection parameters need exact control.

`Failed to acquire session lock` / `Failed to read session data: redis` on the first phpMyAdmin load usually means the redis session lock window is too short for concurrent requests. The defaults above increase the retry budget and lock wait time to avoid that case.

Local multi-pod-style development:

```bash
docker compose -f docker-compose.yaml -f docker-compose.mysql-redis.yaml up --build
```

This keeps the sample MariaDB used by phpMyAdmin and adds separate MySQL/Redis services for pma-gateway state.

## Security Model

The backend trusts identity headers only from configured trusted proxy CIDRs. The upstream proxy must strip spoofable incoming identity headers and set `Remote-User` / `Remote-Groups` only after successful authentication.

Database passwords are encrypted at rest with AES-256-GCM and are write-only in the API. Login tickets are cryptographically random, stored hashed, short-lived, and single-use.

State-changing APIs validate same-origin `Origin` / `Referer` headers. CORS is disabled unless `PMA_GATEWAY_ALLOWED_ORIGINS` is configured.

App Check integration points are present:

```text
PMA_GATEWAY_APPCHECK_MODE=disabled|trusted-header|required
PMA_GATEWAY_APPCHECK_VERIFIED_HEADER=X-AppCheck-Verified
VITE_APPCHECK_ENABLED=false
VITE_APPCHECK_HEADER_NAME=X-Firebase-AppCheck
VITE_APPCHECK_EXCHANGE_URL=/appcheck/api/v1/exchange
```

`trusted-header` and `required` modes expect an upstream verifier such as `turnstile-appcheck-gateway` to set the verified header.

## Bootstrap

Bootstrap can seed credentials and mappings:

```text
PMA_GATEWAY_BOOTSTRAP_ENABLED=true
PMA_GATEWAY_BOOTSTRAP_CONFIG_JSON=
PMA_GATEWAY_BOOTSTRAP_CONFIG_FILE=/config/bootstrap.json
PMA_GATEWAY_BOOTSTRAP_MODE=first-run|reconcile
```

`first-run` applies only when credentials and mappings are empty. `reconcile` upserts credentials and mappings. Bootstrap passwords are never logged.

Bootstrap credential `dbUser` and `dbPassword` can be injected from runtime configuration instead of being written literally in `bootstrap.json`.

- `env:NAME`
  - use the value of environment variable `NAME`
- `secret:NAME`
  - use `NAME` or `NAME_FILE`, following the same secret-loading pattern as the main gateway settings

Example:

```json
{
  "credentials": [
    {
      "id": "prod-admin",
      "name": "Production Admin",
      "dbHost": "mysql.example.internal",
      "dbPort": 3306,
      "dbUser": "env:PMA_GATEWAY_BOOTSTRAP_PROD_ADMIN_DB_USER",
      "dbPassword": "secret:PMA_GATEWAY_BOOTSTRAP_PROD_ADMIN_DB_PASSWORD",
      "enabled": true
    }
  ]
}
```

In Kubernetes, `dbUser` can come from a ConfigMap or Secret-backed env var, and `dbPassword` can come from either `PMA_GATEWAY_BOOTSTRAP_PROD_ADMIN_DB_PASSWORD` or `PMA_GATEWAY_BOOTSTRAP_PROD_ADMIN_DB_PASSWORD_FILE`.

## API

Authenticated APIs:

```http
GET  /_api/v1/me
GET  /_api/v1/available-credentials
POST /_api/v1/pma/sessions
```

Admin APIs:

```http
GET    /_api/v1/admin/credentials
POST   /_api/v1/admin/credentials
POST   /_api/v1/admin/credentials/test
GET    /_api/v1/admin/credentials/:id
PUT    /_api/v1/admin/credentials/:id
DELETE /_api/v1/admin/credentials/:id
GET    /_api/v1/admin/mappings
POST   /_api/v1/admin/mappings
DELETE /_api/v1/admin/mappings/:id
GET    /_api/v1/admin/audit-events
POST   /_api/v1/admin/audit-events/reset
```

Internal API, intended only for the PHP bridge:

```http
POST /internal/v1/signon/redeem
```

## Audit Logs

The backend records security and administration events such as credential access, session starts, ticket creation/redeem attempts, unauthorized access, admin mutations, bootstrap, and audit reset.

The admin UI includes a paginated audit viewer with server-side filters:

- actor
- action
- target type
- result
- from timestamp
- to timestamp

Audit reset requires typing `RESET` and can include a reason. Reset deletes existing audit events and inserts a visible `audit.reset` marker.

## Health Probes

Probe endpoints:

```http
GET /healthz
GET /readyz
GET /healthz
GET /readyz
```

`readyz` checks storage, migrations, and master-key readiness.

Both probe endpoints also return build metadata, including the application version, commit hash, and phpMyAdmin version.

## Version Metadata

Release automation computes the next version, creates the release tag from the merge commit, and injects the resolved version into the image build with the generic build args `BUILD_VERSION` and `BUILD_COMMIT`. This keeps the workflow reusable across repositories and avoids conflicts with branch protection rules.

At runtime:

- the backend reads `BUILD_VERSION` and `BUILD_COMMIT`
- the frontend receives the same version through `config.js`
- the sidebar footer shows both the `pma-gateway` version and the phpMyAdmin version

You can optionally pass the version and commit hash into a local image build:

## Docker Build

```bash
docker buildx build \
  --platform=linux/amd64,linux/arm64 \
  --build-arg PHPMYADMIN_VERSION=5.2.3 \
  --build-arg BUILD_VERSION="0.1.0" \
  --build-arg BUILD_COMMIT="$(git rev-parse HEAD)" \
  -t pma-gateway:local \
  .
```

Writable paths:

```text
/var/lib/pma-gateway
/tmp
/tmp/php-sessions
/tmp/php-conf.d
/tmp/pma-gateway-www
/tmp/nginx-client-body
/tmp/nginx-proxy-temp
/tmp/nginx-fastcgi-temp
/tmp/nginx-uwsgi-temp
/tmp/nginx-scgi-temp
```

## Kubernetes

Example manifests are in [deploy/kubernetes](/home/staratlas@ad.michibiki.io/workspace/pma-gateway/deploy/kubernetes).

SQLite deployment limits:

- Use `replicas: 1`.
- Mount a PersistentVolumeClaim at `/var/lib/pma-gateway`.
- SQLite and local PHP sessions are not horizontally scalable.
- Multi-replica support would require external database storage and external session storage.

For a horizontally scalable pattern, use the example files:

```text
deploy/kubernetes/configmap.external-mysql-redis.yaml
deploy/kubernetes/secret.external-mysql-redis.example.yaml
deploy/kubernetes/deployment.external-mysql-redis-example.yaml
```

That pattern can use multiple replicas because the gateway metadata is stored in MySQL and PHP sessions are stored in Redis. It still assumes all pods use the same `PMA_GATEWAY_MASTER_KEY_BASE64` and `PMA_GATEWAY_INTERNAL_SHARED_SECRET`.

Example probes:

```yaml
livenessProbe:
  httpGet:
    path: /healthz
    port: http

readinessProbe:
  httpGet:
    path: /readyz
    port: http
```

The example deployment uses `readOnlyRootFilesystem: true` with `/tmp` and `/var/lib/pma-gateway` mounted writable.

## Verification

Backend tests:

```bash
go test ./...
```

Frontend build/check:

```bash
cd frontend
npm install
npm run build
npm run check
```

Full container path:

```bash
docker compose up --build
```
