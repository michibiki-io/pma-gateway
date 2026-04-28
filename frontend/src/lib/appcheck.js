let cachedToken = '';
let inFlight = null;

export async function appCheckHeader(config) {
  const appCheck = config?.appCheck;
  if (!appCheck?.enabled) {
    return {};
  }
  const token = await getToken(appCheck);
  if (!token) {
    return {};
  }
  return { [appCheck.headerName || 'X-Firebase-AppCheck']: token };
}

async function getToken(appCheck) {
  if (cachedToken) {
    return cachedToken;
  }
  if (window.__PMA_GATEWAY_APPCHECK_TOKEN__) {
    cachedToken = window.__PMA_GATEWAY_APPCHECK_TOKEN__;
    return cachedToken;
  }
  if (!appCheck.exchangeUrl || !appCheck.turnstileSite) {
    return '';
  }
  if (!inFlight) {
    inFlight = fetch(appCheck.exchangeUrl, { credentials: 'same-origin' })
      .then((response) => (response.ok ? response.json() : null))
      .then((body) => body?.token || '')
      .finally(() => {
        inFlight = null;
      });
  }
  cachedToken = await inFlight;
  return cachedToken;
}
