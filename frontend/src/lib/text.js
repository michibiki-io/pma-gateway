export function toDisplayString(value) {
  if (value === null || value === undefined) {
    return "";
  }
  return typeof value === "string" ? value : String(value);
}

export function truncateMiddle(
  value,
  { start = 12, end = 10, minLength = 28 } = {},
) {
  const normalized = toDisplayString(value);
  if (!normalized) {
    return "";
  }
  if (normalized.length <= minLength) {
    return normalized;
  }

  const safeStart = Math.max(1, start);
  const safeEnd = Math.max(1, end);
  if (normalized.length <= safeStart + safeEnd + 1) {
    return normalized;
  }

  return `${normalized.slice(0, safeStart)}…${normalized.slice(-safeEnd)}`;
}

export async function copyText(value) {
  const text = toDisplayString(value);
  if (!text) {
    return false;
  }

  if (
    typeof navigator !== "undefined" &&
    navigator.clipboard?.writeText &&
    (typeof window === "undefined" || window.isSecureContext)
  ) {
    await navigator.clipboard.writeText(text);
    return true;
  }

  if (typeof document === "undefined") {
    return false;
  }

  const textarea = document.createElement("textarea");
  textarea.value = text;
  textarea.setAttribute("readonly", "");
  textarea.style.position = "fixed";
  textarea.style.opacity = "0";
  textarea.style.pointerEvents = "none";
  document.body.appendChild(textarea);
  textarea.select();
  textarea.setSelectionRange(0, textarea.value.length);

  try {
    return document.execCommand("copy");
  } finally {
    document.body.removeChild(textarea);
  }
}
