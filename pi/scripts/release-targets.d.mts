export interface NativeTarget {
  packageOS: "darwin" | "linux";
  packageCPU: "arm64" | "x64";
  packageSuffix: string;
  goos: "darwin" | "linux";
  goarch: "arm64" | "amd64";
  fileMarker: RegExp;
  tools: string[];
}

export const NATIVE_TOOLS: string[];
export const NATIVE_TARGETS: NativeTarget[];
export function nativeTargetsForTool(tool: string): NativeTarget[];
export function artifactMatchesTool(name: string, tool: string): boolean;
