import { defineConfig } from "tsup";

const dshExternals = [
  "@deepseek-ai/cordis",
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
  "@deepseek-ai/schemastery",
];

export default defineConfig([
  {
    entry: { index: "src/index.ts" },
    format: ["esm"],
    platform: "node",
    target: "node20",
    bundle: true,
    dts: true,
    sourcemap: false,
    clean: true,
    external: [...dshExternals, "react"],
  },
  {
    entry: { client: "src/client.ts" },
    format: ["cjs"],
    platform: "browser",
    target: "es2022",
    bundle: true,
    dts: true,
    sourcemap: false,
    clean: false,
    external: ["react", ...dshExternals],
    outExtension: () => ({ js: ".js" }),
    banner: {
      js: 'window.__ModuleLoader__.load({ id: "@tta-lab/dsh-web", factory: (require) => { var module = { exports: {} }; var exports = module.exports;',
    },
    footer: { js: "return module.exports; } });" },
  },
]);
