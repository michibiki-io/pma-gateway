import test from "node:test";
import assert from "node:assert/strict";
import {
  extractAuditSummaryItems,
  stringifyAuditMetadata,
} from "./audit-log.js";

test("extractAuditSummaryItems returns prioritized summary values", () => {
  assert.deepEqual(
    extractAuditSummaryItems({
      targetId: "ticket_1234567890",
      metadata: {
        credentialId: "dev-admin",
        subject: "alice@example.com",
        count: 3,
      },
    }),
    [
      { key: "targetId", value: "ticket_1234567890", kind: "code" },
      { key: "credentialId", value: "dev-admin", kind: "code" },
      { key: "subject", value: "alice@example.com", kind: "text" },
      { key: "count", value: "3", kind: "text" },
    ],
  );
});

test("extractAuditSummaryItems skips duplicate credential ids", () => {
  assert.deepEqual(
    extractAuditSummaryItems({
      targetId: "dev-admin",
      metadata: {
        credentialId: "dev-admin",
      },
    }),
    [{ key: "targetId", value: "dev-admin", kind: "code" }],
  );
});

test("stringifyAuditMetadata formats non-empty records", () => {
  assert.equal(
    stringifyAuditMetadata({ credentialId: "dev-admin", ttlSeconds: 90 }),
    '{\n  "credentialId": "dev-admin",\n  "ttlSeconds": 90\n}',
  );
  assert.equal(stringifyAuditMetadata({}), "");
});
