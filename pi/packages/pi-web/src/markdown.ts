import { createHash } from "node:crypto";

const DEFAULT_TREE_THRESHOLD = 5000;
const MAX_CONTENT_CHARS = 30_000;
const BASE62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz";

type Heading = {
  level: number;
  text: string;
  start: number;
  id: string;
};

export type MarkdownResult = {
  content: string;
  mode: "full" | "tree" | "section";
};

export function renderMarkdown(
  source: string,
  showTree: boolean,
  sectionId: string | undefined,
  full: boolean,
  treeThreshold: number | undefined,
): MarkdownResult {
  const headings = assignIds(parseHeadings(source));
  const section = sectionId?.trim();
  if (section) {
    return { content: extractSection(source, headings, section), mode: "section" };
  }

  const threshold = treeThreshold && treeThreshold > 0 ? treeThreshold : DEFAULT_TREE_THRESHOLD;
  const charCount = Array.from(source).length;
  if (showTree || (!full && charCount > threshold)) {
    if (headings.length > 0) {
      return { content: renderTree(source, headings), mode: "tree" };
    }
  }
  return { content: truncateContent(source), mode: "full" };
}

export function truncateContent(content: string): string {
  const chars = Array.from(content);
  if (chars.length <= MAX_CONTENT_CHARS) {
    return content;
  }
  return (
    chars.slice(0, MAX_CONTENT_CHARS).join("") +
    `\n[content truncated at ${MAX_CONTENT_CHARS} characters]`
  );
}

function parseHeadings(source: string): Omit<Heading, "id">[] {
  const headings: Omit<Heading, "id">[] = [];
  const lines = source.split("\n");
  let offset = 0;
  let fenced = false;

  for (let index = 0; index < lines.length; index++) {
    const line = lines[index]!.replace(/\r$/, "");
    const trimmed = line.trimStart();
    if (/^(```|~~~)/.test(trimmed)) {
      fenced = !fenced;
      offset += lines[index]!.length + 1;
      continue;
    }
    if (fenced) {
      offset += lines[index]!.length + 1;
      continue;
    }

    const atx = line.match(/^[ \t]{0,3}(#{1,6})(?:[ \t]+(.*?)\s*|[ \t]*)$/);
    if (atx) {
      headings.push({
        level: atx[1]!.length,
        text: normalizeHeadingText(atx[2] ?? ""),
        start: offset,
      });
      offset += lines[index]!.length + 1;
      continue;
    }

    const next = lines[index + 1]?.replace(/\r$/, "");
    if (next && /^[ \t]{0,3}(=+|-+)[ \t]*$/.test(next) && line.trim() !== "") {
      headings.push({
        level: next.trimStart().startsWith("=") ? 1 : 2,
        text: normalizeHeadingText(line.trim()),
        start: offset,
      });
    }
    offset += lines[index]!.length + 1;
  }
  return headings;
}

function normalizeHeadingText(text: string): string {
  return text
    .replace(/[ \t]+#+[ \t]*$/, "")
    .replace(/`([^`]*)`/g, "$1")
    .trim();
}

function assignIds(headings: Omit<Heading, "id">[]): Heading[] {
  const labels = headings.map((heading) => `${"#".repeat(heading.level)} ${heading.text}`);
  const ids = labels.map((label) => hashBase62(label, 2));
  const counts = new Map<string, number>();
  for (const id of ids) counts.set(id, (counts.get(id) ?? 0) + 1);
  return headings.map((heading, index) => ({
    ...heading,
    id: counts.get(ids[index]!)! > 1 ? hashBase62(`${labels[index]}\0${index}`, 3) : ids[index]!,
  }));
}

function hashBase62(value: string, length: number): string {
  const digest = createHash("sha256").update(value).digest();
  let number = 0n;
  for (const byte of digest.subarray(0, 8)) number = (number << 8n) | BigInt(byte);
  let output = "";
  while (number > 0n) {
    output = BASE62[Number(number % 62n)] + output;
    number /= 62n;
  }
  return output.padStart(length, "0").slice(-length);
}

function extractSection(source: string, headings: Heading[], sectionId: string): string {
  const targetIndex = headings.findIndex((heading) => heading.id === sectionId);
  if (targetIndex < 0) {
    const available = headings
      .filter((heading) => heading.id !== "")
      .map((heading) => `${JSON.stringify(heading.id)} (${heading.text})`)
      .join(", ");
    throw new Error(`section ${JSON.stringify(sectionId)} not found; available: ${available}`);
  }
  const target = headings[targetIndex]!;
  let end = source.length;
  for (const heading of headings.slice(targetIndex + 1)) {
    if (heading.level <= target.level) {
      end = heading.start;
      break;
    }
  }
  return source.slice(target.start, end).replace(/\n+$/, "") + "\n";
}

function renderTree(source: string, headings: Heading[]): string {
  if (headings.length === 0) return "(no headings)\n";

  let header = "";
  if (headings[0]!.level === 1) {
    header = `# ${headings[0]!.text}\n\nTotal: ${formatNumber(sectionCharCount(source, headings, 0))} characters\n\n`;
  }

  const bodyHeadings = headings.filter((heading) => heading.level > 1);
  const minLevel = bodyHeadings.reduce((min, heading) => Math.min(min, heading.level), 99);
  if (bodyHeadings.length === 0) {
    return `${header}(empty)\n\nUse -s <id> to read a section, or --full to read everything.\n`;
  }

  const nodes = bodyHeadings.map((heading) => {
    const index = headings.indexOf(heading);
    return {
      heading,
      depth: heading.level - (minLevel === 99 ? 2 : minLevel),
      meta: `(${formatNumber(sectionCharCount(source, headings, index))} chars)`,
    };
  });

  const hasMore = new Map<number, boolean>();
  let tree = "";
  nodes.forEach((node, index) => {
    const isLast = !nodes.slice(index + 1).some((future) => future.depth <= node.depth)
      ? true
      : nodes.slice(index + 1).find((future) => future.depth <= node.depth)!.depth < node.depth;
    let indent = "";
    for (let depth = 0; depth < node.depth; depth++) {
      indent += hasMore.get(depth) ? "│   " : "    ";
    }
    tree += `${indent}${isLast ? "└── " : "├── "}[${node.heading.id}] ${"#".repeat(node.heading.level)} ${node.heading.text}  ${node.meta}\n`;
    hasMore.set(node.depth, !isLast);
    for (let depth = node.depth + 1; depth < node.depth + 10; depth++) hasMore.delete(depth);
  });

  return `${header}${tree}\nUse -s <id> to read a section, or --full to read everything.\n`;
}

function sectionCharCount(source: string, headings: Heading[], index: number): number {
  const heading = headings[index]!;
  let end = source.length;
  for (const next of headings.slice(index + 1)) {
    if (next.level <= heading.level) {
      end = next.start;
      break;
    }
  }
  return Array.from(source.slice(heading.start, end)).length;
}

function formatNumber(value: number): string {
  return value.toLocaleString("en-US");
}
