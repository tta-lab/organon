import { spawn } from "node:child_process";

export interface CliRunOptions {
  /** Fixed argv passed to the binary; never a shell string. */
  args: string[];
  /** Optional multiline payload written to the child's stdin. */
  stdin?: string;
  /** Tool abort signal; propagated to the child process. */
  signal?: AbortSignal;
}

export interface CliRunResult {
  stdout: string;
  stderr: string;
  exitCode: number;
  killed: boolean;
}

/**
 * Runs the package-local binary with fixed argv, optional stdin content, and
 * abort propagation. Resolves with the captured output; rejects with
 * "Operation aborted" when the caller signal fires, mirroring pi's built-in
 * tools. Does not use a shell and does not start a persistent process.
 */
export function runCli(binaryPath: string, options: CliRunOptions): Promise<CliRunResult> {
  return new Promise((resolve, reject) => {
    if (options.signal?.aborted) {
      reject(new Error("Operation aborted"));
      return;
    }
    const child = spawn(binaryPath, options.args, { stdio: ["pipe", "pipe", "pipe"] });
    let stdout = "";
    let stderr = "";
    let spawnError: Error | undefined;

    const onAbort = () => {
      child.kill("SIGTERM");
    };
    options.signal?.addEventListener("abort", onAbort, { once: true });

    child.stdout.setEncoding("utf8");
    child.stderr.setEncoding("utf8");
    child.stdout.on("data", (chunk: string) => {
      stdout += chunk;
    });
    child.stderr.on("data", (chunk: string) => {
      stderr += chunk;
    });
    child.on("error", (error: Error) => {
      spawnError = error;
    });
    child.on("close", (code, signal) => {
      options.signal?.removeEventListener("abort", onAbort);
      if (options.signal?.aborted) {
        reject(new Error("Operation aborted"));
        return;
      }
      if (spawnError) {
        reject(
          new Error(
            `failed to start ${binaryPath}: ${spawnError.message} ` + `(exit ${code ?? "?"})`,
          ),
        );
        return;
      }
      resolve({ stdout, stderr, exitCode: code ?? -1, killed: signal !== null });
    });

    if (options.stdin !== undefined) {
      child.stdin.write(options.stdin);
    }
    child.stdin.end();
  });
}
