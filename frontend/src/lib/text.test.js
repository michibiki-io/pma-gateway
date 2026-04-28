import test from "node:test";
import assert from "node:assert/strict";
import { toDisplayString, truncateMiddle } from "./text.js";

test("toDisplayString normalizes nullish values", () => {
  assert.equal(toDisplayString(null), "");
  assert.equal(toDisplayString(undefined), "");
  assert.equal(toDisplayString(3306), "3306");
});

test("truncateMiddle short strings unchanged", () => {
  assert.equal(truncateMiddle("dev-admin"), "dev-admin");
});

test("truncateMiddle preserves head and tail", () => {
  assert.equal(
    truncateMiddle("ticket_846c6f32abcdef9876543210", {
      start: 8,
      end: 6,
      minLength: 18,
    }),
    "ticket_8…543210",
  );
});
