import { appCheckHeader } from './appcheck.js';

export class ApiError extends Error {
  constructor(message, status, body) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.body = body;
  }
}

export class RecoverableAuthError extends ApiError {
  constructor(message, status, body, path) {
    super(message, status, body);
    this.name = 'RecoverableAuthError';
    this.path = path;
    this.recoverableAuth = true;
  }
}

const RECOVERABLE_AUTH_PATHS = new Set(['/me', '/available-credentials']);
const RECOVERABLE_AUTH_STATUSES = new Set([401, 403]);
const DEFAULT_AUTH_RETRY_DELAYS_MS = [500, 1500];

export function runtimeConfig() {
  const fallback = {
    publicBasePath: import.meta.env.VITE_PMA_GATEWAY_PUBLIC_BASE_PATH || '/',
    frontendBase: import.meta.env.VITE_PMA_GATEWAY_FRONTEND_BASE || '/_gateway/',
    apiBase: import.meta.env.VITE_API_BASE || '/_api/v1',
    pmaBase: '/_pma/',
    signonUrl: '/_signon.php',
    version: {
      appVersion: import.meta.env.VITE_PMA_GATEWAY_APP_VERSION || 'unknown',
      appDisplayVersion: import.meta.env.VITE_PMA_GATEWAY_APP_DISPLAY_VERSION || 'unknown',
      appCommit: import.meta.env.VITE_PMA_GATEWAY_APP_COMMIT || 'unknown',
      appShortCommit: import.meta.env.VITE_PMA_GATEWAY_APP_SHORT_COMMIT || '',
      phpMyAdminVersion: import.meta.env.VITE_PMA_GATEWAY_PHPMYADMIN_VERSION || 'unknown'
    },
    appCheck: {
      enabled: import.meta.env.VITE_APPCHECK_ENABLED === 'true',
      headerName: import.meta.env.VITE_APPCHECK_HEADER_NAME || 'X-Firebase-AppCheck',
      exchangeUrl: import.meta.env.VITE_APPCHECK_EXCHANGE_URL || '/appcheck/api/v1/exchange',
      turnstileSite: import.meta.env.VITE_TURNSTILE_SITE_KEY || '',
      firebaseAPIKey: import.meta.env.VITE_FIREBASE_API_KEY || '',
      firebaseAppID: import.meta.env.VITE_FIREBASE_APP_ID || '',
      firebaseProjID: import.meta.env.VITE_FIREBASE_PROJECT_ID || ''
    }
  };
  const config = window.__PMA_GATEWAY_CONFIG__ || {};
  return {
    ...fallback,
    ...config,
    version: {
      ...fallback.version,
      ...(config.version || {})
    },
    appCheck: {
      ...fallback.appCheck,
      ...(config.appCheck || {})
    },
    frontendBase: withTrailingSlash(config.frontendBase || fallback.frontendBase),
    apiBase: withoutTrailingSlash(config.apiBase || fallback.apiBase)
  };
}

export async function apiRequest(config, path, options = {}) {
  const headers = {
    Accept: 'application/json',
    ...(options.body ? { 'Content-Type': 'application/json' } : {}),
    ...(await appCheckHeader(config)),
    ...(options.headers || {})
  };
  const response = await fetch(`${withoutTrailingSlash(config.apiBase)}${path}`, {
    credentials: 'same-origin',
    ...options,
    headers
  });
  const text = await response.text();
  let body = null;
  if (text) {
    try {
      body = JSON.parse(text);
    } catch {
      body = { error: text };
    }
  }
  if (!response.ok) {
    throw new ApiError(body?.error || '', response.status, body);
  }
  return body;
}

export async function apiRequestWithAuthRecovery(config, path, options = {}) {
  const delays = options.authRetryDelaysMs || DEFAULT_AUTH_RETRY_DELAYS_MS;
  const requestOptions = { ...options };
  delete requestOptions.authRetryDelaysMs;

  for (let attempt = 0; ; attempt += 1) {
    try {
      return await apiRequest(config, path, requestOptions);
    } catch (err) {
      if (!isRetryableAuthFailure(path, err)) {
        throw err;
      }
      if (attempt >= delays.length) {
        throw new RecoverableAuthError(err.message, err.status, err.body, path);
      }
      await delay(delays[attempt]);
    }
  }
}

export function isRecoverableAuthError(err) {
  return Boolean(err?.recoverableAuth);
}

function isRetryableAuthFailure(path, err) {
  return (
    RECOVERABLE_AUTH_PATHS.has(pathWithoutQuery(path)) &&
    RECOVERABLE_AUTH_STATUSES.has(err?.status)
  );
}

function pathWithoutQuery(path) {
  return String(path || '').split('?')[0];
}

function delay(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function withTrailingSlash(value) {
  return value.endsWith('/') ? value : `${value}/`;
}

function withoutTrailingSlash(value) {
  return value.endsWith('/') ? value.slice(0, -1) : value;
}
