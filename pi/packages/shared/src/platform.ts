// Supported native platforms; Windows x64 is not supported.

export interface PlatformTriple {
  os: "darwin" | "linux";
  arch: "arm64" | "x64";
}

const SUPPORTED: Record<PlatformTriple["os"], PlatformTriple["arch"][]> = {
  darwin: ["arm64"],
  linux: ["arm64", "x64"],
};

export function detectPlatform(): PlatformTriple {
  const os = process.platform as NodeJS.Platform;
  const arch = process.arch as NodeJS.Architecture;
  const supportedArches = SUPPORTED[os as PlatformTriple["os"]];
  if (!supportedArches || !supportedArches.includes(arch as PlatformTriple["arch"])) {
    throw new Error(
      `Organon Pi extensions ship native binaries for darwin-arm64, linux-x64, and linux-arm64; ` +
        `this host is ${os}-${arch} and is not supported. Install a supported pi runtime instead of ` +
        `downloading a GitHub release or falling back to PATH.`,
    );
  }
  return { os: os as PlatformTriple["os"], arch: arch as PlatformTriple["arch"] };
}
