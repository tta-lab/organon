import { createHash } from "node:crypto";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { homedir } from "node:os";
import { join } from "node:path";

import { Defuddle } from "defuddle/node";
import { parseHTML } from "linkedom";

import { renderMarkdown, truncateContent } from "./markdown.js";

const REQUEST_TIMEOUT_MS = 30_000;
const MAX_DOWNLOAD_BYTES = 10 * 1024 * 1024;
const MAX_BINARY_SCAN_BYTES = 8192;
const WEB_FETCH_AGENT = "organon/1.0";

export type FetchInput = {
  url: string;
  tree?: boolean;
  section_id?: string;
  full?: boolean;
  tree_threshold?: number;
};

export type FetchResult = {
  url: string;
  mode: "full" | "tree" | "section";
  content: string;
};

export async function fetchWebPage(
  input: FetchInput,
  callerSignal?: AbortSignal,
): Promise<FetchResult> {
  const url = validateURL(input.url);
  const content = await fetchCached(url, callerSignal);
  const rendered = renderMarkdown(
    content,
    input.tree ?? false,
    input.section_id,
    input.full ?? false,
    input.tree_threshold,
  );
  return { url, mode: rendered.mode, content: rendered.content };
}

async function fetchCached(url: string, callerSignal?: AbortSignal): Promise<string> {
  const cache = new DailyCache(defaultCacheDir());
  await cache.prepare();
  const cached = await cache.read(url);
  if (cached !== undefined) return cached;

  const content = await fetchLocal(url, callerSignal);
  await cache.write(url, content);
  return content;
}

async function fetchLocal(url: string, callerSignal?: AbortSignal): Promise<string> {
  const request = createRequestSignal(callerSignal, REQUEST_TIMEOUT_MS);
  try {
    const response = await fetch(url, {
      headers: { "User-Agent": WEB_FETCH_AGENT },
      redirect: "follow",
      signal: request.signal,
    });
    if (!response.ok) {
      throw new Error(`HTTP ${response.status}`);
    }

    const body = await readBody(response);
    const contentType = mediaType(response.headers.get("content-type"));
    if (isBinaryContentType(contentType) || isBinaryBody(body)) {
      throw binaryFetchError(url, response.headers.get("content-type") ?? "");
    }

    if (contentType !== "" && contentType !== "text/html") {
      return truncateContent(new TextDecoder().decode(body));
    }

    try {
      const html = new TextDecoder().decode(body);
      const { document } = parseHTML(html);
      const parsed = await Defuddle(document, url, { markdown: true, useAsync: false });
      const content = parsed.content;
      if (!content || content.trim() === "") {
        throw new Error("no content could be extracted");
      }
      return truncateContent(content);
    } catch (error) {
      throw new Error(`defuddle parse failed: ${errorMessage(error)}`);
    }
  } catch (error) {
    if (callerSignal?.aborted) throw new Error("Operation aborted");
    if (request.timedOut)
      throw new Error(`fetch timed out after ${REQUEST_TIMEOUT_MS / 1000} seconds`);
    if (error instanceof Error && error.message.startsWith("binary content at ")) throw error;
    throw new Error(`fetch ${url}: ${errorMessage(error)}`);
  } finally {
    request.cleanup();
  }
}

function validateURL(rawURL: string): string {
  let url: URL;
  try {
    url = new URL(rawURL);
  } catch {
    throw new Error(`fetch ${rawURL}: invalid URL`);
  }
  if (url.protocol !== "http:" && url.protocol !== "https:") {
    throw new Error(`fetch ${rawURL}: URL must use http or https`);
  }
  return rawURL;
}

async function readBody(response: Response): Promise<Uint8Array> {
  if (!response.body) {
    const body = new Uint8Array(await response.arrayBuffer());
    if (body.byteLength > MAX_DOWNLOAD_BYTES) {
      throw new Error(`response exceeds ${MAX_DOWNLOAD_BYTES} byte limit`);
    }
    return body;
  }
  const reader = response.body.getReader();
  const chunks: Uint8Array[] = [];
  let total = 0;
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      if (!value) continue;
      total += value.byteLength;
      if (total > MAX_DOWNLOAD_BYTES) {
        throw new Error(`response exceeds ${MAX_DOWNLOAD_BYTES} byte limit`);
      }
      chunks.push(value);
    }
  } finally {
    reader.releaseLock();
  }
  const body = new Uint8Array(total);
  let offset = 0;
  for (const chunk of chunks) {
    body.set(chunk, offset);
    offset += chunk.byteLength;
  }
  return body;
}

function mediaType(contentType: string | null): string {
  return contentType?.split(";", 1)[0]?.trim().toLowerCase() ?? "";
}

function isBinaryContentType(contentType: string): boolean {
  return [
    "application/octet-stream",
    "application/zip",
    "application/gzip",
    "application/x-gzip",
    "application/x-tar",
    "application/pdf",
    "application/msword",
    "application/vnd.ms-",
    "application/vnd.openxmlformats",
    "image/",
    "audio/",
    "video/",
    "font/",
    "application/x-msdownload",
    "application/x-executable",
    "application/x-mach-binary",
  ].some((prefix) => contentType.startsWith(prefix));
}

function isBinaryBody(body: Uint8Array): boolean {
  const limit = Math.min(body.byteLength, MAX_BINARY_SCAN_BYTES);
  for (let index = 0; index < limit; index++) {
    if (body[index] === 0) return true;
  }
  return false;
}

function binaryFetchError(url: string, contentType: string): Error {
  const displayType = contentType || "(none)";
  return new Error(
    `binary content at ${url} (Content-Type: ${displayType})\n\n` +
      "web fetch only handles text. Use curl to download:\n  curl -L -O " +
      url,
  );
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function defaultCacheDir(): string {
  const home = process.env.HOME?.trim() || homedir();
  return join(home, ".cache", "organon", "scrapes");
}

class DailyCache {
  private enabled = false;

  constructor(private readonly directory: string) {}

  async prepare(): Promise<void> {
    try {
      await mkdir(this.directory, { recursive: true, mode: 0o755 });
      this.enabled = true;
    } catch {
      this.enabled = false;
    }
  }

  async read(url: string): Promise<string | undefined> {
    if (!this.enabled) return undefined;
    try {
      return await readFile(this.path(url), "utf8");
    } catch {
      return undefined;
    }
  }

  async write(url: string, content: string): Promise<void> {
    if (!this.enabled) return;
    try {
      await writeFile(this.path(url), content, { encoding: "utf8", mode: 0o644 });
    } catch {
      // Cache failures must not make a successful fetch fail.
    }
  }

  private path(url: string): string {
    const now = new Date();
    const date = [now.getFullYear(), now.getMonth() + 1, now.getDate()]
      .map((part, index) => (index === 0 ? String(part) : String(part).padStart(2, "0")))
      .join("-");
    return join(this.directory, `${sanitizeURL(url)}__${date}.md`);
  }
}

function sanitizeURL(rawURL: string): string {
  let parsed: URL | undefined;
  try {
    parsed = new URL(rawURL);
  } catch {
    return rawURL
      .replaceAll("://", "___")
      .replaceAll("/", "_")
      .replaceAll("?", "_")
      .replaceAll("=", "_")
      .replaceAll("&", "_")
      .replaceAll("..", "__");
  }

  let base = rawURL.replaceAll("://", "___");
  if (parsed.search) {
    base = base.split("?", 1)[0]!;
  }
  base = base.replaceAll("/", "_").replace(/_$/, "").replaceAll("..", "__");
  if (parsed.search) {
    const queryHash = createHash("sha256").update(parsed.search.slice(1)).digest("hex").slice(0, 8);
    base += `_q${queryHash}`;
  }
  if (base.length > 200) {
    const hash = createHash("sha256").update(base).digest("hex").slice(0, 8);
    base = base.slice(0, 191) + `_${hash}`;
  }
  return base;
}

function createRequestSignal(callerSignal: AbortSignal | undefined, timeoutMs: number) {
  const controller = new AbortController();
  let timedOut = false;
  const onAbort = () => controller.abort(callerSignal?.reason);
  if (callerSignal?.aborted) onAbort();
  callerSignal?.addEventListener("abort", onAbort, { once: true });
  const timer = setTimeout(() => {
    timedOut = true;
    controller.abort(new Error("request timeout"));
  }, timeoutMs);
  return {
    signal: controller.signal,
    get timedOut() {
      return timedOut;
    },
    cleanup() {
      clearTimeout(timer);
      callerSignal?.removeEventListener("abort", onAbort);
    },
  };
}
