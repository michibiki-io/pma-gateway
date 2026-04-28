import { appCheckHeader } from './appcheck.js';

export class ApiError extends Error {
  constructor(message, status, body) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.body = body;
  }
}

export function runtimeConfig() {
  const fallback = {
    publicBasePath: import.meta.env.VITE_PMA_GATEWAY_PUBLIC_BASE_PATH || '/dbadmin',
    frontendBase: import.meta.env.VITE_PMA_GATEWAY_FRONTEND_BASE || '/dbadmin/_gateway/',
    apiBase: import.meta.env.VITE_API_BASE || '/dbadmin/_api/v1',
    pmaBase: '/dbadmin/_pma/',
    signonUrl: '/dbadmin/_signon.php',
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

function withTrailingSlash(value) {
  return value.endsWith('/') ? value : `${value}/`;
}

function withoutTrailingSlash(value) {
  return value.endsWith('/') ? value.slice(0, -1) : value;
}
