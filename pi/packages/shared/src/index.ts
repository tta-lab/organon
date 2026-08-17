export { detectPlatform, type PlatformTriple } from "./platform.js";
export { nativePackageName, resolveBinaryPath, type BinaryResolver } from "./binary.js";
export { runCli, type CliRunOptions, type CliRunResult } from "./process.js";
export { parseSingleJsonDoc, cliError } from "./result.js";
export {
  modelTextResult,
  type ModelTextDetails,
  type ModelTextToolResult,
  type TextContentBlock,
} from "./model-text.js";
export { truncateForModel, type ModelText, type TruncateForModelOptions } from "./truncate.js";
