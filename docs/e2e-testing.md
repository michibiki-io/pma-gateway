# E2E Testing

This project uses Playwright for Web GUI E2E smoke testing. The Playwright project lives in `frontend/`.

## Install

```bash
cd frontend
npm ci
```

## Run all E2E tests

```bash
cd frontend
npm run test:e2e
```

By default, Playwright starts the Vite dev server on `http://127.0.0.1:3000` and uses mocked gateway API responses for frontend smoke tests. To run against a real gateway stack, set `E2E_REAL_APP=true`, `E2E_BASE_URL`, `E2E_FRONTEND_PATH`, and disable the built-in web server:

```bash
cd frontend
E2E_REAL_APP=true E2E_BASE_URL=http://127.0.0.1:8080 E2E_FRONTEND_PATH=/pma/_gateway/ E2E_SKIP_WEBSERVER=true npm run test:e2e
```

## Run Desktop Chrome-compatible tests

```bash
cd frontend
npm run test:e2e:chrome
```

The `desktop-chrome` project uses Playwright Chromium with the Desktop Chrome device profile. The official Microsoft Playwright Linux Docker image includes Playwright browser binaries, but not the branded Google Chrome channel.

## Run WebKit / Safari-compatible tests

```bash
cd frontend
npm run test:e2e:webkit
```

## Run mobile tests

```bash
cd frontend
npm run test:e2e:mobile
```

The mobile projects use the Pixel 5 and iPhone 13 Playwright device profiles.

## View report

```bash
cd frontend
npm run test:e2e:report
```

The HTML report is written to `frontend/playwright-report/` and does not open automatically in CI or container execution.

## View traces

```bash
cd frontend
npx playwright show-trace test-results/**/trace.zip
```

Screenshots, videos, and traces are written under `frontend/test-results/` and are retained on failure.

Successful smoke-test screen screenshots are written to `frontend/test-results/screenshots/` by default. In the real-stack run, screenshots include gateway navigation screens and the phpMyAdmin screen reached through the signon flow.

## Run in Docker

The Docker E2E environment uses the official Microsoft Playwright image matching `@playwright/test`:

```text
mcr.microsoft.com/playwright:v1.59.1-noble
```

Run it from the repository root with the application compose files:

```bash
cp dev/.env.example dev/.env
docker compose \
  -f docker-compose.yaml \
  -f docker-compose.mysql-redis.yaml \
  -f docker-compose.e2e.yaml \
  up --build --abort-on-container-exit --exit-code-from e2e e2e
```

The container runs:

```bash
npm ci
npm run test:e2e
```

The E2E service waits for `dev-auth-proxy` to report `/pma/readyz` as healthy, then runs Playwright against `http://dev-auth-proxy:8080/pma/_gateway/`. This exercises the gateway frontend, backend API, MySQL metadata storage, Redis-backed PHP sessions, the signon bridge, and phpMyAdmin.

The E2E compose override forces gateway mode on and uses `/pma` as the public base path so the test URL is stable even if `dev/.env` differs.

This works well from VS Code Remote Development and Dev Containers because the browsers and Linux system dependencies come from the Playwright image. In a custom dev container, use the same image or install dependencies with the matching Playwright version before running `npm ci` and `npm run test:e2e`.

## Notes about Safari

The Safari-compatible test projects use Playwright WebKit.

This does not run the real macOS Safari browser inside Linux Docker.

## Authentication

The initial smoke tests mock the gateway API responses and do not require credentials. Future authenticated E2E tests should use `E2E_USER` and `E2E_PASSWORD` or a clearly separated test-only mode such as `E2E_TEST_MODE=true`; credentials must not be hard-coded.
