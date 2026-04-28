import { svelte } from '@sveltejs/vite-plugin-svelte';
import { defineConfig } from 'vite';

const appVersion = normalizeVersion(process.env.BUILD_VERSION || process.env.PMA_GATEWAY_BUILD_VERSION || '');
const appCommit = normalizeCommit(process.env.BUILD_COMMIT || process.env.PMA_GATEWAY_BUILD_COMMIT || process.env.GITHUB_SHA || '');
const appShortCommit = shortCommit(appCommit);
const appDisplayVersion = formatDisplayVersion(appVersion, appShortCommit);
const phpMyAdminVersion = normalizeVersion(process.env.PHPMYADMIN_VERSION || '');

export default defineConfig({
  plugins: [svelte()],
  base: './',
  define: {
    'import.meta.env.VITE_PMA_GATEWAY_APP_VERSION': JSON.stringify(appVersion),
    'import.meta.env.VITE_PMA_GATEWAY_APP_DISPLAY_VERSION': JSON.stringify(appDisplayVersion),
    'import.meta.env.VITE_PMA_GATEWAY_APP_COMMIT': JSON.stringify(appCommit),
    'import.meta.env.VITE_PMA_GATEWAY_APP_SHORT_COMMIT': JSON.stringify(appShortCommit),
    'import.meta.env.VITE_PMA_GATEWAY_PHPMYADMIN_VERSION': JSON.stringify(phpMyAdminVersion)
  },
  build: {
    sourcemap: true
  }
});

function normalizeVersion(value) {
  const normalized = String(value || '').trim().replace(/^v/, '');
  return normalized || 'unknown';
}

function normalizeCommit(value) {
  const normalized = String(value || '').trim();
  return normalized || 'unknown';
}

function shortCommit(value) {
  if (!value || value === 'unknown') {
    return '';
  }
  return value.slice(0, 12);
}

function formatDisplayVersion(version, shortHash) {
  if (!version || version === 'unknown') {
    return 'unknown';
  }
  return shortHash ? `v${version}+${shortHash}` : `v${version}`;
}
