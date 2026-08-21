import { describe, expect, it } from "vitest";

import {
  BRAVE_CREDENTIAL_REF,
  CONTEXT7_CREDENTIAL_REF,
  EXA_CREDENTIAL_REF,
  SETTINGS_NAMESPACE,
} from "../src/contract.js";
import {
  decodeSettings,
  describeCredentialStatus,
  persistProviderSelection,
  removeCredential,
  writeCredential,
} from "../src/client.js";

function fakeApi(overrides: Record<string, unknown> = {}) {
  const calls: Array<{ method: string; payload: unknown }> = [];
  return {
    calls,
    credentials: {
      describe: async ({ refs }: { refs: string[] }) => ({
        result: {
          ok: true,
          value: {
            credentials: Object.fromEntries(
              refs.map((ref) => [
                ref,
                {
                  configured: ref === EXA_CREDENTIAL_REF,
                  source: "file",
                  writable: true,
                },
              ]),
            ),
          },
        },
      }),
      set: async (payload: unknown) => {
        calls.push({ method: "set", payload });
        return overrides.set ?? { result: { ok: true, value: {} } };
      },
      unset: async (payload: unknown) => {
        calls.push({ method: "unset", payload });
        return overrides.unset ?? { result: { ok: true, value: {} } };
      },
    },
  } as any;
}

describe("DSH settings client credential surface", () => {
  it("persists only the selected provider and drops unknown/secret settings fields", async () => {
    const calls: Array<{ field: string; value: unknown }> = [];
    await persistProviderSelection(
      { set: async (field, value) => void calls.push({ field, value }) },
      "exa",
    );
    expect(calls).toEqual([{ field: "provider", value: "exa" }]);
    expect(decodeSettings({ provider: "brave", apiKey: "secret-value" })).toEqual({
      provider: "brave",
    });
    expect(decodeSettings({ apiKey: "secret-value" })).toEqual({});
    expect(SETTINGS_NAMESPACE).toBe("organon-web");
  });

  it("returns only credential metadata and keeps refs package-namespaced", async () => {
    const api = fakeApi();
    const status = await describeCredentialStatus(api, [
      EXA_CREDENTIAL_REF,
      BRAVE_CREDENTIAL_REF,
      CONTEXT7_CREDENTIAL_REF,
    ]);
    expect(status).toEqual({
      [EXA_CREDENTIAL_REF]: { configured: true, source: "file", writable: true },
      [BRAVE_CREDENTIAL_REF]: { configured: false, source: "file", writable: true },
      [CONTEXT7_CREDENTIAL_REF]: { configured: false, source: "file", writable: true },
    });
    expect(JSON.stringify(status)).not.toContain("secret-value");
  });

  it("writes and removes secrets through credentials only, with blank drafts as no-ops", async () => {
    const api = fakeApi();
    expect(await writeCredential(api, EXA_CREDENTIAL_REF, "")).toBe(false);
    expect(api.calls).toEqual([]);
    expect(await writeCredential(api, EXA_CREDENTIAL_REF, "secret-value")).toBe(true);
    await removeCredential(api, EXA_CREDENTIAL_REF);
    expect(api.calls).toEqual([
      { method: "set", payload: { ref: EXA_CREDENTIAL_REF, value: "secret-value" } },
      { method: "unset", payload: { ref: EXA_CREDENTIAL_REF } },
    ]);
    expect(JSON.stringify(api.calls)).toContain("secret-value");
  });

  it("maps rejected credential writes to safe UI errors", async () => {
    const api = fakeApi({
      set: { result: { ok: false, error: { message: "secret-value rejected" } } },
    });
    await expect(writeCredential(api, EXA_CREDENTIAL_REF, "secret-value")).rejects.toThrow(
      "credential write rejected",
    );
    await expect(writeCredential(api, EXA_CREDENTIAL_REF, "secret-value")).rejects.not.toThrow(
      "secret-value",
    );
  });
});
