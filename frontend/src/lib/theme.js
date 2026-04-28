import { get, writable } from "svelte/store";

const THEME_STORAGE_KEY = "pma-gateway-theme";

export const supportedThemeModes = Object.freeze(["system", "light", "dark"]);
export const themePreference = writable("system");
export const resolvedTheme = writable("light");

let mediaQuery;
let cleanupMediaQuery = () => {};

function normalizeThemePreference(value) {
  return supportedThemeModes.includes(value) ? value : "system";
}

function resolveTheme(preference) {
  if (preference === "system") {
    if (
      typeof window !== "undefined" &&
      window.matchMedia("(prefers-color-scheme: dark)").matches
    ) {
      return "dark";
    }
    return "light";
  }
  return preference;
}

function applyResolvedTheme(theme) {
  if (typeof document === "undefined") {
    return;
  }
  const root = document.documentElement;
  root.classList.toggle("dark", theme === "dark");
  root.style.colorScheme = theme;
  resolvedTheme.set(theme);
}

function syncTheme() {
  applyResolvedTheme(resolveTheme(get(themePreference)));
}

function readStoredThemePreference() {
  if (typeof window === "undefined") {
    return "system";
  }
  return normalizeThemePreference(
    window.localStorage.getItem(THEME_STORAGE_KEY) || "system",
  );
}

function attachMediaQueryListener() {
  if (typeof window === "undefined") {
    return () => {};
  }
  mediaQuery = window.matchMedia("(prefers-color-scheme: dark)");
  const handleChange = () => {
    if (get(themePreference) === "system") {
      syncTheme();
    }
  };

  if (typeof mediaQuery.addEventListener === "function") {
    mediaQuery.addEventListener("change", handleChange);
    return () => mediaQuery.removeEventListener("change", handleChange);
  }

  mediaQuery.addListener(handleChange);
  return () => mediaQuery.removeListener(handleChange);
}

export function setupTheme() {
  themePreference.set(readStoredThemePreference());
  cleanupMediaQuery();
  cleanupMediaQuery = attachMediaQueryListener();
  syncTheme();
}

export function setThemeMode(nextTheme) {
  const normalized = normalizeThemePreference(nextTheme);
  themePreference.set(normalized);
  if (typeof window !== "undefined") {
    window.localStorage.setItem(THEME_STORAGE_KEY, normalized);
  }
  syncTheme();
}
