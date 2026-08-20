// Supported public native package targets. Keep platform/build metadata here so
// staging, artifact invariants, and package inventory tests cannot drift.
export const NATIVE_TOOLS = ["src", "web", "project", "og"];

const ALL_TOOLS = NATIVE_TOOLS;

export const NATIVE_TARGETS = [
  {
    packageOS: "darwin",
    packageCPU: "arm64",
    packageSuffix: "darwin-arm64",
    goos: "darwin",
    goarch: "arm64",
    fileMarker: /Mach-O.*arm64/,
    tools: ALL_TOOLS,
  },
  {
    packageOS: "linux",
    packageCPU: "x64",
    packageSuffix: "linux-x64",
    goos: "linux",
    goarch: "amd64",
    fileMarker: /ELF.*x86-64/,
    tools: ALL_TOOLS,
  },
  {
    packageOS: "linux",
    packageCPU: "arm64",
    packageSuffix: "linux-arm64",
    goos: "linux",
    goarch: "arm64",
    fileMarker: /ELF.*aarch64/,
    tools: ALL_TOOLS,
  },
  {
    packageOS: "win32",
    packageCPU: "x64",
    packageSuffix: "win32-x64",
    goos: "windows",
    goarch: "amd64",
    fileMarker: /PE32\+.*x86-64/,
    tools: ["web"],
  },
];

export function nativeTargetsForTool(tool) {
  return NATIVE_TARGETS.filter((target) => target.tools.includes(tool));
}

export function artifactMatchesTool(name, tool) {
  return String(name).replace(/[.]exe$/i, "") === tool;
}
