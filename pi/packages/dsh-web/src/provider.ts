import { parseSingleJsonDoc } from "@tta-lab/pi-shared/parse";
import { runCli, type CliRunOptions, type CliRunResult } from "@tta-lab/pi-shared/process";
import {
  credentialRef,
  type CredentialRef,
  type ResolvedCredential,
} from "@deepseek-ai/dsh-credentials";
import type { WebSearchProvider, WebSearchRequest, WebSearchResult } from "@deepseek-ai/dsh-web";

import {
  BRAVE_CREDENTIAL_REF,
  EXA_CREDENTIAL_REF,
  SEARCH_PROVIDER_ID,
  type SearchProviderName,
} from "./contract.js";

export interface SearchProviderDependencies {
  binaryPath: string;
  getProvider: () => SearchProviderName;
  credentials: {
    resolve(ref: CredentialRef): Promise<ResolvedCredential | undefined>;
  };
  run?: (binaryPath: string, options: CliRunOptions) => Promise<CliRunResult>;
}

function credentialFor(
  provider: SearchProviderName,
): { ref: CredentialRef; env: string } | undefined {
  switch (provider) {
    case "exa":
      return { ref: credentialRef(EXA_CREDENTIAL_REF), env: "EXA_API_KEY" };
    case "brave":
      return { ref: credentialRef(BRAVE_CREDENTIAL_REF), env: "BRAVE_API_KEY" };
    case "duckduckgo":
      return undefined;
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function validateSearchResult(value: unknown, provider: SearchProviderName): WebSearchResult {
  if (!isRecord(value) || !Array.isArray(value.results)) {
    throw new Error(`${provider} search returned an invalid results array`);
  }

  const sources = value.results.map((raw, index) => {
    if (!isRecord(raw)) {
      throw new Error(
        `${provider} search returned an invalid result at index ${index}: source must be an object`,
      );
    }
    const link = raw.link;
    const title = raw.title;
    const snippet = raw.snippet;
    if (typeof link !== "string") {
      throw new Error(
        `${provider} search returned an invalid result at index ${index}: link must be a string`,
      );
    }
    if (link.length === 0) {
      throw new Error(
        `${provider} search returned an invalid result at index ${index}: link must be non-empty`,
      );
    }
    if (title !== undefined && typeof title !== "string") {
      throw new Error(
        `${provider} search returned an invalid result at index ${index}: title must be a string`,
      );
    }
    if (snippet !== undefined && typeof snippet !== "string") {
      throw new Error(
        `${provider} search returned an invalid result at index ${index}: snippet must be a string`,
      );
    }
    return {
      url: link,
      ...(title === undefined ? {} : { title }),
      ...(snippet === undefined ? {} : { snippet }),
    };
  });

  return { sources, truncated: false };
}

function redactedMessage(error: unknown, secret?: string): string {
  const message = error instanceof Error ? error.message : String(error);
  return secret === undefined || secret === "" ? message : message.split(secret).join("[redacted]");
}

export function createOrganonSearchProvider(
  dependencies: SearchProviderDependencies,
): WebSearchProvider {
  const runner = dependencies.run ?? runCli;
  return {
    id: SEARCH_PROVIDER_ID,
    // Selection is explicit in DSH settings. Credential absence is not an
    // availability failure because the child preserves env and ttal dotenv.
    available: () => true,
    async search(request: WebSearchRequest, signal?: AbortSignal): Promise<WebSearchResult> {
      const provider = dependencies.getProvider();
      const credential = credentialFor(provider);
      let resolved: ResolvedCredential | undefined;
      if (credential !== undefined) {
        try {
          resolved = await dependencies.credentials.resolve(credential.ref);
        } catch {
          throw new Error(`${provider} credential resolution failed`);
        }
      }

      const options: CliRunOptions = {
        // Put all flags before `--`; a query beginning with `-` is positional.
        args: ["search", "--provider", provider, "--json", "--", request.query],
        signal,
        ...(resolved !== undefined && credential !== undefined
          ? { env: { [credential.env]: resolved.value } }
          : {}),
      };

      let result: CliRunResult;
      try {
        result = await runner(dependencies.binaryPath, options);
      } catch (error) {
        throw new Error(`${provider} search failed: ${redactedMessage(error, resolved?.value)}`);
      }
      if (result.exitCode !== 0) {
        const detail = result.stderr
          .trim()
          .replace(/^Error:\s*/m, "")
          .replace(/\s+/g, " ");
        const error =
          detail === "" ? `command exited with code ${result.exitCode}` : detail.slice(0, 8192);
        throw new Error(`${provider} search failed: ${redactedMessage(error, resolved?.value)}`);
      }

      let data: unknown;
      try {
        data = parseSingleJsonDoc<unknown>(result.stdout);
      } catch (error) {
        throw new Error(
          `${provider} search returned invalid JSON: ${redactedMessage(error, resolved?.value)}`,
        );
      }
      try {
        return validateSearchResult(data, provider);
      } catch (error) {
        throw new Error(
          `${provider} search result validation failed: ${redactedMessage(error, resolved?.value)}`,
        );
      }
    },
  };
}
