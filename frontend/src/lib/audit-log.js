import { toDisplayString } from "./text.js";

function asRecord(value) {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return {};
  }
  return value;
}

export function extractAuditSummaryItems(event) {
  const items = [];
  const targetId = toDisplayString(event?.targetId);
  const metadata = asRecord(event?.metadata);

  if (targetId) {
    items.push({ key: "targetId", value: targetId, kind: "code" });
  }

  const credentialId = toDisplayString(metadata.credentialId);
  if (credentialId && credentialId !== targetId) {
    items.push({ key: "credentialId", value: credentialId, kind: "code" });
  }

  const subject = toDisplayString(metadata.subject);
  if (subject) {
    items.push({ key: "subject", value: subject, kind: "text" });
  }

  const count = toDisplayString(metadata.count);
  if (count) {
    items.push({ key: "count", value: count, kind: "text" });
  }

  return items;
}

export function stringifyAuditMetadata(metadata) {
  const normalized = asRecord(metadata);
  if (!Object.keys(normalized).length) {
    return "";
  }
  return JSON.stringify(normalized, null, 2);
}
