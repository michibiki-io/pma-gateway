# pma-gateway

## Table of Contents
- English
  - [Overview](#overview)
  - [Architecture](#architecture)
  - [Local Development](#local-development)
  - [Configuration](#configuration)
  - [External MySQL and Redis](#external-mysql-and-redis)
  - [Security Model](#security-model)
  - [Bootstrap](#bootstrap)
  - [API](#api)
  - [Audit Logs](#audit-logs)
  - [Health Probes](#health-probes)
  - [Version Metadata](#version-metadata)
  - [Docker Build](#docker-build)
  - [Kubernetes](#kubernetes)
  - [Verification](#verification)
- 日本語
  - [概要](#概要)
  - [アーキテクチャ](#アーキテクチャ)
  - [ローカル開発](#ローカル開発)
  - [設定](#設定)
  - [外部 MySQL / Redis](#外部-mysql--redis)
  - [セキュリティモデル](#セキュリティモデル)
  - [ブートストラップ](#ブートストラップ)
  - [API](#api-1)
  - [監査ログ](#監査ログ)
  - [ヘルスプローブ](#ヘルスプローブ)
  - [バージョン情報](#バージョン情報)
  - [Docker ビルド](#docker-ビルド)
  - [Kubernetes](#kubernetes-1)
  - [検証](#検証)

## English

### Overview

`pma-gateway` is a header-authenticated access gateway for phpMyAdmin. It lets an upstream authentication proxy identify a user, maps that user or their groups to managed database credentials, and signs the browser into phpMyAdmin without ever returning database passwords to the browser.

### Architecture

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

### Local Development

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

The development proxy injects fake auth headers. The default user is `alice@example.com` with `db-users,db-admins`. To switch the development proxy to a non-admin user:

```text
http://localhost:8080/?as=bob
```

The selected development user is stored by the development proxy in a cookie. Switch back with:

```text
http://localhost:8080/?as=alice
```

The compose stack starts:

- `pma-gateway`
- `mariadb`
- `dev-auth-proxy`

MariaDB is initialized with `sampledb`, a readonly user, and an admin-like user. Bootstrap credentials and mappings are loaded from [dev/bootstrap.json](dev/bootstrap.json).

### Configuration

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

Key variables:

| Variable | Purpose | Notes |
| --- | --- | --- |
| `PMA_GATEWAY` | Enables gateway mode for phpMyAdmin signon flow | Set `false` for direct phpMyAdmin login mode |
| `PMA_GATEWAY_PUBLIC_BASE_PATH` | Sets the public base path | Default is `/`; use `/dbadmin` for subpath deployment |
| `PMA_GATEWAY_PMA_PATH` | Sets the phpMyAdmin path under the public base path | Default `/_pma` |
| `PMA_GATEWAY_FRONTEND_PATH` | Sets the frontend path under the public base path | Default `/_gateway` |
| `PMA_GATEWAY_API_PATH` | Sets the backend API path under the public base path | Default `/_api` |
| `PMA_GATEWAY_SIGNON_PATH` | Sets the PHP signon bridge path | Default `/_signon.php` |
| `PMA_GATEWAY_ALLOWED_ORIGINS` | Enables CORS for allowed browser origins | Leave empty unless cross-origin access is required |
| `PMA_GATEWAY_MASTER_KEY_BASE64` / `_FILE` | Supplies the 32-byte encryption key for stored credentials | Required in production |
| `PMA_GATEWAY_INTERNAL_SHARED_SECRET` / `_FILE` | Supplies the shared secret between the backend and PHP signon bridge | Required for signon redemption |
| `PMA_GATEWAY_PHP_SESSION_GC_MAXLIFETIME` | Controls PHP session lifetime | Should be equal to or greater than `PMA_GATEWAY_PHPMYADMIN_LOGIN_COOKIE_VALIDITY` |
| `PMA_GATEWAY_PHP_UPLOAD_MAX_FILESIZE` / `PMA_GATEWAY_PHP_POST_MAX_SIZE` | Controls phpMyAdmin upload limits | Set both when enabling large imports |
| `PMA_GATEWAY_DATABASE_DRIVER` | Selects backend metadata storage | Default runtime uses SQLite; set `mysql` for multi-replica deployments |
| `PMA_GATEWAY_PHP_SESSION_STORE` | Selects PHP session storage | Use `redis` for multi-replica deployments |

`PMA_GATEWAY_PUBLIC_BASE_PATH` defaults to `/`. The backend and generated nginx routing avoid double slashes for root-base deployments.

The application supports both root deployment and subpath deployment. For example, `PMA_GATEWAY_PUBLIC_BASE_PATH=/dbadmin` keeps the public URLs under `/dbadmin/_gateway/`, `/dbadmin/_api/v1/`, `/dbadmin/_pma/`, and `/dbadmin/_signon.php`.

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

### External MySQL and Redis

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

### Security Model

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

### Bootstrap

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

### API

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

### Audit Logs

The backend records security and administration events such as credential access, session starts, ticket creation/redeem attempts, unauthorized access, admin mutations, bootstrap, and audit reset.

The admin UI includes a paginated audit viewer with server-side filters:

- actor
- action
- target type
- result
- from timestamp
- to timestamp

Audit reset requires typing `RESET` and can include a reason. Reset deletes existing audit events and inserts a visible `audit.reset` marker.

### Health Probes

Probe endpoints:

```http
GET /healthz
GET /readyz
```

`readyz` checks storage, migrations, and master-key readiness.

Both probe endpoints also return build metadata, including the application version, commit hash, and phpMyAdmin version.

### Version Metadata

Release automation computes the next version, creates the release tag from the merge commit, and injects the resolved version into the image build with the generic build args `BUILD_VERSION` and `BUILD_COMMIT`. This keeps the workflow reusable across repositories and avoids conflicts with branch protection rules.

At runtime:

- the backend reads `BUILD_VERSION` and `BUILD_COMMIT`
- the frontend receives the same version through `config.js`
- the footer shows both the `pma-gateway` version and the phpMyAdmin version

You can optionally pass the version and commit hash into a local image build.

### Docker Build

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

### Kubernetes

Example manifests are in [deploy/kubernetes](deploy/kubernetes).

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

### Verification

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

## 日本語

### 概要

`pma-gateway` は、ヘッダー認証されたユーザーを phpMyAdmin に安全に中継するアクセスゲートウェイです。上流の認証プロキシがユーザーを識別し、そのユーザーまたは所属グループを管理対象のデータベース認証情報にマッピングし、DB パスワードをブラウザーへ返さずに phpMyAdmin へサインインさせます。

### アーキテクチャ

runtime image には次が含まれます。

- Go backend API
- Svelte 製の静的 frontend
- `signon` 認証で構成された phpMyAdmin
- 公開 HTTP サーバーとしての nginx
- phpMyAdmin と signon 実行用の PHP-FPM
- プロセス監視用の supervisord
- メタデータ保存用の SQLite

デフォルトの公開パス構成:

```text
/                     統合された公開エントリ URL
/_pma/                phpMyAdmin
/_gateway/            pma-gateway frontend
/_api/v1/             backend API
/_signon.php          phpMyAdmin signon bridge
```

`/` へアクセスすると phpMyAdmin にリダイレクトされます。phpMyAdmin に有効な session がない場合、`SignonURL` を通じて gateway frontend へ戻されます。ユーザーが利用可能な credential を選択すると、backend が短命の one-time ticket を発行し、PHP の signon bridge が internal shared secret を使って localhost 経由でその ticket を redeem します。

### ローカル開発

stack を起動する前に `dev/.env.example` を `dev/.env` にコピーし、実際の値を書き込んでください。compose file は example file ではなく `dev/.env` から runtime 環境変数を読みます。default 以外の origin から gateway にアクセスする場合は、`dev/.env` の `PMA_GATEWAY_ALLOWED_ORIGINS` をブラウザーの origin と完全一致するように更新してください。port も含めて一致させる必要があります。default compose 構成では次の値です。

```text
PMA_GATEWAY_ALLOWED_ORIGINS=http://localhost:8080
```

```bash
cp dev/.env.example dev/.env
docker compose up --build
```

アクセス先:

```text
http://localhost:8080/
```

development proxy は疑似 auth header を注入します。default user は `alice@example.com` で、所属グループは `db-users,db-admins` です。development proxy を非 admin user に切り替えるには次を使います。

```text
http://localhost:8080/?as=bob
```

選択した development user は development proxy が cookie に保持します。`alice` に戻すには次を使います。

```text
http://localhost:8080/?as=alice
```

compose stack で起動するもの:

- `pma-gateway`
- `mariadb`
- `dev-auth-proxy`

MariaDB には `sampledb`、readonly user、admin 相当 user が初期化されます。bootstrap 用の credentials と mappings は [dev/bootstrap.json](dev/bootstrap.json) から読み込まれます。

### 設定

主な環境変数:

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

主要な環境変数:

| 変数 | 用途 | 補足 |
| --- | --- | --- |
| `PMA_GATEWAY` | phpMyAdmin の signon を使う gateway mode を有効化する | `false` にすると phpMyAdmin の direct login mode になる |
| `PMA_GATEWAY_PUBLIC_BASE_PATH` | 公開 base path を設定する | default は `/`。subpath 配置なら `/dbadmin` を使う |
| `PMA_GATEWAY_PMA_PATH` | public base path 配下の phpMyAdmin path を設定する | default は `/_pma` |
| `PMA_GATEWAY_FRONTEND_PATH` | public base path 配下の frontend path を設定する | default は `/_gateway` |
| `PMA_GATEWAY_API_PATH` | public base path 配下の backend API path を設定する | default は `/_api` |
| `PMA_GATEWAY_SIGNON_PATH` | PHP signon bridge の path を設定する | default は `/_signon.php` |
| `PMA_GATEWAY_ALLOWED_ORIGINS` | 許可するブラウザー origin に対して CORS を有効化する | cross-origin access が不要なら空のままにする |
| `PMA_GATEWAY_MASTER_KEY_BASE64` / `_FILE` | 保存 credential の暗号化に使う 32 byte key を渡す | production では必須 |
| `PMA_GATEWAY_INTERNAL_SHARED_SECRET` / `_FILE` | backend と PHP signon bridge 間の shared secret を渡す | signon redeem に必須 |
| `PMA_GATEWAY_PHP_SESSION_GC_MAXLIFETIME` | PHP session の有効期間を制御する | `PMA_GATEWAY_PHPMYADMIN_LOGIN_COOKIE_VALIDITY` 以上を推奨 |
| `PMA_GATEWAY_PHP_UPLOAD_MAX_FILESIZE` / `PMA_GATEWAY_PHP_POST_MAX_SIZE` | phpMyAdmin の upload 上限を制御する | 大きい import を許可する場合は両方設定する |
| `PMA_GATEWAY_DATABASE_DRIVER` | backend metadata storage を選ぶ | default runtime は SQLite。multi-replica では `mysql` を使う |
| `PMA_GATEWAY_PHP_SESSION_STORE` | PHP session storage を選ぶ | multi-replica では `redis` を使う |

`PMA_GATEWAY_PUBLIC_BASE_PATH` の default は `/` です。backend と生成される nginx routing は root base の deployment でも二重スラッシュが出ないようにしています。

このアプリケーションは root 配置と subpath 配置の両方に対応しています。たとえば `PMA_GATEWAY_PUBLIC_BASE_PATH=/dbadmin` を指定すると、公開 URL は `/dbadmin/_gateway/`、`/dbadmin/_api/v1/`、`/dbadmin/_pma/`、`/dbadmin/_signon.php` の形になります。

`PMA_GATEWAY=false` を指定すると phpMyAdmin の direct login mode に切り替わります。この mode では次の動作になります。

- `/` は引き続き phpMyAdmin へ redirect されるが、gateway UI ではなく phpMyAdmin の login screen が表示される
- `/_gateway`、`/_api`、`/_signon.php` は phpMyAdmin 側へ redirect される
- phpMyAdmin は `auth_type=cookie` を使う
- `PMA_GATEWAY_PMA_HOST` が設定されていれば、その host/port を固定の login target として使う
- `PMA_GATEWAY_PMA_HOST` が空なら、`PMA_GATEWAY_PMA_ALLOW_ARBITRARY_SERVER=true` により login screen 上で接続先 server を選べる

`PMA_GATEWAY_PHPMYADMIN_ALLOW_THIRD_PARTY_FRAMING` は phpMyAdmin の `X-Frame-Options` 挙動を制御します。対応値は `false`、`sameorigin`、`true` です。default は `sameorigin` で、同一 origin からの iframe 埋め込みのみを許可します。

gateway UI/API が返す timestamp は、レスポンス生成時に `PMA_GATEWAY_TIMESTAMP_FORMAT` と `PMA_GATEWAY_TIMESTAMP_TIMEZONE` で整形されます。内部保存値は ordering/filtering のため RFC3339 のままです。default 表示は `2001-01-01 10:00:00 JST` です。

admin UI からの credential connection test は `PMA_GATEWAY_CREDENTIAL_TEST_TIMEOUT_SECONDS` を使用します。default timeout は `10` 秒です。

`PMA_GATEWAY_PHP_SESSION_GC_MAXLIFETIME` は `PMA_GATEWAY_PHPMYADMIN_LOGIN_COOKIE_VALIDITY` 以上にしてください。短いと、login cookie より先に PHP session が失効するという phpMyAdmin の warning が出ます。

secret 相当の値は直接指定することも、`_FILE` variant から読むこともできます。Kubernetes Secret では `_FILE` の利用を推奨します。

```text
PMA_GATEWAY_MASTER_KEY_FILE=/var/run/secrets/pma-gateway/master-key
PMA_GATEWAY_INTERNAL_SHARED_SECRET_FILE=/var/run/secrets/pma-gateway/internal-shared-secret
PMA_GATEWAY_BOOTSTRAP_CONFIG_FILE=/config/bootstrap.json
```

master key は base64 decode 後に 32 byte である必要があります。production では key 未設定時に backend が起動失敗します。development 専用の ephemeral key を使うには次を指定します。

```text
PMA_GATEWAY_DEV_INSECURE_EPHEMERAL_KEY=true
```

### 外部 MySQL / Redis

SQLite と local PHP session は single-replica deployment 向けです。複数 pod で動かすには、共有 state を container 外へ出してください。

- gateway metadata: MySQL
- PHP/phpMyAdmin session: Redis

backend storage に MySQL を使う場合:

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

`PMA_GATEWAY_DATABASE_DSN` が優先されます。未設定なら component 変数から MySQL DSN を組み立てます。Kubernetes Secret では `PMA_GATEWAY_DATABASE_DSN_FILE` または `PMA_GATEWAY_MYSQL_PASSWORD_FILE` を使ってください。

PHP session に Redis を使う場合:

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

`PMA_GATEWAY_REDIS_SESSION_URL` が優先されます。Redis auth や接続 parameter を厳密に制御したい場合はこちらを使ってください。

phpMyAdmin 初回表示時に `Failed to acquire session lock` / `Failed to read session data: redis` が出る場合、redis session lock の待機窓が短すぎることが多いです。上記 default は retry budget と lock wait time を増やしてそのケースを避けるためのものです。

multi-pod 相当の local development:

```bash
docker compose -f docker-compose.yaml -f docker-compose.mysql-redis.yaml up --build
```

この構成では、phpMyAdmin 用の sample MariaDB を維持しつつ、pma-gateway state 用に別の MySQL/Redis service を追加します。

### セキュリティモデル

backend は、設定済み trusted proxy CIDR からの identity header だけを信頼します。上流 proxy は spoof 可能な incoming header を必ず除去し、認証成功後にのみ `Remote-User` / `Remote-Groups` を設定してください。

database password は保存時に AES-256-GCM で暗号化され、API からは write-only です。login ticket は暗号学的に十分ランダムで、hash 化して保存され、短命かつ single-use です。

state-changing API は same-origin の `Origin` / `Referer` header を検証します。`PMA_GATEWAY_ALLOWED_ORIGINS` を設定しない限り CORS は無効です。

App Check 統合用の設定点:

```text
PMA_GATEWAY_APPCHECK_MODE=disabled|trusted-header|required
PMA_GATEWAY_APPCHECK_VERIFIED_HEADER=X-AppCheck-Verified
VITE_APPCHECK_ENABLED=false
VITE_APPCHECK_HEADER_NAME=X-Firebase-AppCheck
VITE_APPCHECK_EXCHANGE_URL=/appcheck/api/v1/exchange
```

`trusted-header` と `required` mode では、`turnstile-appcheck-gateway` のような upstream verifier が verified header を付与する前提です。

### ブートストラップ

bootstrap では credentials と mappings を seed できます。

```text
PMA_GATEWAY_BOOTSTRAP_ENABLED=true
PMA_GATEWAY_BOOTSTRAP_CONFIG_JSON=
PMA_GATEWAY_BOOTSTRAP_CONFIG_FILE=/config/bootstrap.json
PMA_GATEWAY_BOOTSTRAP_MODE=first-run|reconcile
```

`first-run` は credentials と mappings が空のときだけ適用されます。`reconcile` は credentials と mappings を upsert します。bootstrap password は log に出しません。

bootstrap credential の `dbUser` と `dbPassword` は、`bootstrap.json` に literal で書かず、runtime 設定から注入できます。

- `env:NAME`
  - 環境変数 `NAME` の値を使う
- `secret:NAME`
  - main gateway 設定と同じ secret 読み込み規約に従い、`NAME` または `NAME_FILE` を使う

例:

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

Kubernetes では、`dbUser` は ConfigMap または Secret-backed env var から、`dbPassword` は `PMA_GATEWAY_BOOTSTRAP_PROD_ADMIN_DB_PASSWORD` または `PMA_GATEWAY_BOOTSTRAP_PROD_ADMIN_DB_PASSWORD_FILE` のどちらからでも渡せます。

### API

認証済み API:

```http
GET  /_api/v1/me
GET  /_api/v1/available-credentials
POST /_api/v1/pma/sessions
```

admin API:

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

PHP bridge 専用の internal API:

```http
POST /internal/v1/signon/redeem
```

### 監査ログ

backend は credential access、session start、ticket の create/redeem 試行、unauthorized access、admin mutation、bootstrap、audit reset などの security / administration event を記録します。

admin UI には、server-side filter 付きのページネーション対応 audit viewer があります。

- actor
- action
- target type
- result
- from timestamp
- to timestamp

audit reset では `RESET` の入力が必要で、任意で reason も付けられます。reset を行うと既存の audit event を削除し、可視な `audit.reset` marker を挿入します。

### ヘルスプローブ

probe endpoint:

```http
GET /healthz
GET /readyz
```

`readyz` は storage、migration、master-key readiness を確認します。

両 endpoint とも、application version、commit hash、phpMyAdmin version を含む build metadata を返します。

### バージョン情報

release automation は次の version を計算し、merge commit から release tag を作成し、解決済み version を汎用 build arg `BUILD_VERSION` と `BUILD_COMMIT` として image build に注入します。これにより workflow を repo 間で再利用しやすくし、branch protection rule とも衝突しにくくしています。

runtime では次のように使われます。

- backend が `BUILD_VERSION` と `BUILD_COMMIT` を読む
- frontend は同じ version を `config.js` 経由で受け取る
- footer に `pma-gateway` version と phpMyAdmin version の両方を表示する

local image build でも、version と commit hash を任意で渡せます。

### Docker ビルド

```bash
docker buildx build \
  --platform=linux/amd64,linux/arm64 \
  --build-arg PHPMYADMIN_VERSION=5.2.3 \
  --build-arg BUILD_VERSION="0.1.0" \
  --build-arg BUILD_COMMIT="$(git rev-parse HEAD)" \
  -t pma-gateway:local \
  .
```

書き込みが必要な path:

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

### Kubernetes

example manifest は [deploy/kubernetes](deploy/kubernetes) にあります。

SQLite deployment の制約:

- `replicas: 1` を使う
- `/var/lib/pma-gateway` に PersistentVolumeClaim を mount する
- SQLite と local PHP session は水平分散できない
- multi-replica 対応には external database storage と external session storage が必要

水平分散向けには次の example file を使ってください。

```text
deploy/kubernetes/configmap.external-mysql-redis.yaml
deploy/kubernetes/secret.external-mysql-redis.example.yaml
deploy/kubernetes/deployment.external-mysql-redis-example.yaml
```

この構成では gateway metadata を MySQL、PHP session を Redis に保存するため複数 replica を使えます。ただし全 pod で同じ `PMA_GATEWAY_MASTER_KEY_BASE64` と `PMA_GATEWAY_INTERNAL_SHARED_SECRET` を使う前提です。

probe 設定例:

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

example deployment では `readOnlyRootFilesystem: true` を使い、`/tmp` と `/var/lib/pma-gateway` を writable mount にしています。

### 検証

backend test:

```bash
go test ./...
```

frontend build/check:

```bash
cd frontend
npm install
npm run build
npm run check
```

container 全体の確認:

```bash
docker compose up --build
```
