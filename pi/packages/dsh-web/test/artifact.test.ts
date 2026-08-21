import { execFileSync } from "node:child_process";
import { mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

import { parse } from "yaml";
import { afterAll, beforeAll, describe, expect, it } from "vitest";

const packageRoot = join(dirname(fileURLToPath(import.meta.url)), "..");

let artifactRoot = "";
let artifactTemp = "";

beforeAll(() => {
  artifactTemp = mkdtempSync(join(tmpdir(), "dsh-web-packed-contract-"));
  execFileSync("pnpm", ["pack", "--pack-destination", artifactTemp], {
    cwd: packageRoot,
    stdio: "ignore",
  });
  const tarballs = readdirSync(artifactTemp).filter((file) => file.endsWith(".tgz"));
  if (tarballs.length !== 1) throw new Error("expected one packed DSH Web artifact");

  const unpacked = join(artifactTemp, "unpacked");
  mkdirSync(unpacked);
  execFileSync("tar", ["-xzf", join(artifactTemp, tarballs[0]!), "-C", unpacked]);
  artifactRoot = join(unpacked, "package");
  writeHostFakes();
});

function writeHostFakes(): void {
  const dependencies = {
    "dsh-settings": "export function settingsNamespace(namespace) { return namespace; }\n",
    schemastery: [
      "const z = {",
      "  object: () => ({}),",
      "  union: () => ({ default: () => ({}) }),",
      "};",
      "export default z;",
      "",
    ].join("\n"),
    "dsh-credentials": "export function credentialRef(ref) { return ref; }\n",
    "dsh-tools": "export function defineTool(definition) { return definition; }\n",
  };
  const nativeDirectory = join(
    artifactRoot,
    "node_modules",
    "@tta-lab",
    `pi-web-${process.platform}-${process.arch}`,
  );
  mkdirSync(nativeDirectory, { recursive: true });
  writeFileSync(
    join(nativeDirectory, "package.json"),
    JSON.stringify({ name: `@tta-lab/pi-web-${process.platform}-${process.arch}` }) + "\n",
  );
  for (const [name, source] of Object.entries(dependencies)) {
    const directory = join(artifactRoot, "node_modules", "@deepseek-ai", name);
    mkdirSync(directory, { recursive: true });
    writeFileSync(
      join(directory, "package.json"),
      JSON.stringify({ type: "module", exports: "./index.js" }) + "\n",
    );
    writeFileSync(join(directory, "index.js"), source);
  }
}

afterAll(() => {
  if (artifactTemp !== "") rmSync(artifactTemp, { recursive: true, force: true });
});

function packedManifest(): any {
  return JSON.parse(readFileSync(join(artifactRoot, "package.json"), "utf8"));
}

describe("DSH rc.8 package contract", () => {
  it("packs the host/client artifact with the rc.8 peers, patch, and native declarations", async () => {
    const manifest = packedManifest();
    const packedHost = await import(
      `${pathToFileURL(join(artifactRoot, "dist", "index.js")).href}?host-contract=1`
    );
    const rc8Peers = [
      "@deepseek-ai/dsh-api-remotes",
      "@deepseek-ai/dsh-client-connection",
      "@deepseek-ai/dsh-client-locale",
      "@deepseek-ai/dsh-client-runtime",
      "@deepseek-ai/dsh-client-ui-settings",
      "@deepseek-ai/dsh-client-ui-settings-plugins",
      "@deepseek-ai/dsh-client-ui-slots",
      "@deepseek-ai/dsh-credentials",
      "@deepseek-ai/dsh-settings",
      "@deepseek-ai/dsh-tools",
      "@deepseek-ai/dsh-web",
    ];
    const nativePackages = [
      "@tta-lab/pi-web-darwin-arm64",
      "@tta-lab/pi-web-linux-arm64",
      "@tta-lab/pi-web-linux-x64",
      "@tta-lab/pi-web-win32-x64",
    ];

    expect(manifest.name).toBe("@tta-lab/dsh-web");
    expect(readdirSync(join(artifactRoot, "dist"))).toEqual(
      expect.arrayContaining(["index.js", "index.d.ts", "client.js", "client.d.cts"]),
    );
    expect(manifest.main).toBe("dist/index.js");
    expect(manifest.types).toBe("dist/index.d.ts");
    expect(manifest.exports["./client"]).toEqual({
      types: "./dist/client.d.cts",
      default: "./dist/client.js",
    });
    expect(packedHost.name).toBe("organon-dsh-web");
    expect(packedHost.inject).toEqual(["web", "credentials", "settings", "tools"]);
    expect(typeof packedHost.apply).toBe("function");

    let registeredProvider: any;
    const registeredTools: any[] = [];
    packedHost.apply({
      settings: {
        register: () => ({ get: () => ({ provider: "duckduckgo" }) }),
      },
      credentials: { resolve: async () => undefined },
      web: { registerSearchProvider: (provider: unknown) => (registeredProvider = provider) },
      tools: { register: (definition: unknown) => registeredTools.push(definition) },
    } as any);
    expect(registeredProvider.id).toBe("organon-web-search");
    expect(registeredTools.map((definition) => definition.name)).toEqual([
      "web_fetch",
      "web_docs",
      "web_sgraph",
    ]);
    expect(
      Object.fromEntries(
        Object.entries(manifest.peerDependencies).filter(([name]) =>
          name.startsWith("@deepseek-ai/dsh"),
        ),
      ),
    ).toEqual(Object.fromEntries(rc8Peers.map((name) => [name, "0.1.0-rc.8"])));
    expect(manifest.optionalDependencies).toEqual(
      Object.fromEntries(nativePackages.map((name) => [name, manifest.version])),
    );
    expect(manifest.dsh).toEqual({
      bundle: { patch: "./cordis.patch.yml" },
      client: {
        platform: "web",
        inject: [
          "@deepseek-ai/dsh-api-remotes",
          "@deepseek-ai/dsh-client-connection",
          "@deepseek-ai/dsh-client-locale",
          "@deepseek-ai/dsh-client-runtime",
          "@deepseek-ai/dsh-client-ui-settings",
          "@deepseek-ai/dsh-client-ui-settings-plugins",
        ],
      },
    });

    const patch = parse(readFileSync(join(artifactRoot, "cordis.patch.yml"), "utf8")) as any[];
    expect(patch.find((entry) => entry.id === "web")).toEqual({
      id: "web",
      name: "@deepseek-ai/dsh-web",
      config: { searchProvider: "organon-web-search" },
    });
    expect(patch.find((entry) => entry.id === "tool-web")).toEqual({
      id: "tool-web",
      disabled: true,
    });
    expect(patch.find((entry) => Array.isArray(entry.insert))?.insert).toContainEqual({
      id: "organon-dsh-web",
      name: "@tta-lab/dsh-web",
      inject: ["web", "credentials", "settings", "tools"],
    });
  });

  it("loads the packed browser entry through an rc.8 lazy-module fake", async () => {
    const previousWindow = (globalThis as any).window;
    const registrations: any[] = [];
    (globalThis as any).window = {
      __ModuleLoader__: {
        load: (registration: unknown) => registrations.push(registration),
      },
    };

    try {
      await import(`${pathToFileURL(join(artifactRoot, "dist", "client.js")).href}?contract=1`);
      expect(registrations).toHaveLength(1);
      expect(registrations[0].id).toBe("@tta-lab/dsh-web");
      expect(typeof registrations[0].factory).toBe("function");

      const fakeReact = {
        createElement: () => ({}),
        useEffect: () => undefined,
        useState: <T>(value: T) => [value, () => undefined] as const,
      };
      const loaded = registrations[0].factory((specifier: string) => {
        expect(specifier).toBe("react");
        return fakeReact;
      });
      expect(typeof loaded.apply).toBe("function");
      expect(loaded.inject).toEqual(["connection", "remote", "settingsScope", "slots"]);
    } finally {
      if (previousWindow === undefined) delete (globalThis as any).window;
      else (globalThis as any).window = previousWindow;
    }
  });
});
