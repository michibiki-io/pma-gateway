import test from "node:test";
import assert from "node:assert/strict";
import {
  ApiError,
  RecoverableAuthError,
  apiRequestWithAuthRecovery,
  isRecoverableAuthError,
} from "./api.js";

const config = {
  apiBase: "/_api/v1",
  appCheck: { enabled: false },
};

test.afterEach(() => {
  delete globalThis.fetch;
});

test("apiRequestWithAuthRecovery retries transient auth failures for /me", async () => {
  const calls = [];
  globalThis.fetch = async (url) => {
    calls.push(url);
    if (calls.length < 3) {
      return jsonResponse({ error: "authenticated identity required" }, 403);
    }
    return jsonResponse({
      user: "alice@example.com",
      groups: ["db-users"],
      isAdmin: false,
    });
  };

  const body = await apiRequestWithAuthRecovery(config, "/me", {
    authRetryDelaysMs: [0, 0],
  });

  assert.equal(calls.length, 3);
  assert.equal(body.user, "alice@example.com");
});

test("apiRequestWithAuthRecovery reports recoverable auth after retry exhaustion", async () => {
  let calls = 0;
  globalThis.fetch = async () => {
    calls += 1;
    return jsonResponse({ error: "authenticated identity required" }, 401);
  };

  await assert.rejects(
    apiRequestWithAuthRecovery(config, "/available-credentials", {
      authRetryDelaysMs: [0, 0],
    }),
    (err) => {
      assert.equal(calls, 3);
      assert.ok(err instanceof RecoverableAuthError);
      assert.ok(isRecoverableAuthError(err));
      assert.equal(err.status, 401);
      assert.equal(err.path, "/available-credentials");
      return true;
    },
  );
});

test("apiRequestWithAuthRecovery does not retry auth failures for other endpoints", async () => {
  let calls = 0;
  globalThis.fetch = async () => {
    calls += 1;
    return jsonResponse({ error: "admin authorization is required" }, 403);
  };

  await assert.rejects(
    apiRequestWithAuthRecovery(config, "/admin/credentials", {
      authRetryDelaysMs: [0, 0],
    }),
    (err) => {
      assert.equal(calls, 1);
      assert.ok(err instanceof ApiError);
      assert.equal(isRecoverableAuthError(err), false);
      assert.equal(err.status, 403);
      return true;
    },
  );
});

test("apiRequestWithAuthRecovery keeps server errors on the existing path", async () => {
  let calls = 0;
  globalThis.fetch = async () => {
    calls += 1;
    return jsonResponse({ error: "internal server error" }, 500);
  };

  await assert.rejects(
    apiRequestWithAuthRecovery(config, "/me", {
      authRetryDelaysMs: [0, 0],
    }),
    (err) => {
      assert.equal(calls, 1);
      assert.ok(err instanceof ApiError);
      assert.equal(isRecoverableAuthError(err), false);
      assert.equal(err.status, 500);
      return true;
    },
  );
});

function jsonResponse(body, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}
