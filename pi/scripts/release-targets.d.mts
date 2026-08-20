export interface NativeTarget {
  packageOS: "darwin" | "linux" | "win32";
  packageCPU: "arm64" | "x64";
  packageSuffix: string;
  goos: "darwin" | "linux" | "windows";
  goarch: "arm64" | "amd64";
  fileMarker: RegExp;
  tools: string[];
}

export const NATIVE_TOOLS: string[];
export const NATIVE_TARGETS: NativeTarget[];
export function nativeTargetsForTool(tool: string): NativeTarget[];
