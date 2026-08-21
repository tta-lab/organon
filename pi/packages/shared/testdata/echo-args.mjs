// Reads stdin, echoes argv + stdin as JSON to stdout, writes one stderr line.
let input = "";
process.stdin.setEncoding("utf8");
process.stdin.on("data", (chunk) => {
  input += chunk;
});
process.stdin.on("end", () => {
  process.stdout.write(
    JSON.stringify({
      argv: process.argv.slice(2),
      stdin: input,
      ...(process.env.PI_TEST_CHILD_VALUE !== undefined
        ? { env: process.env.PI_TEST_CHILD_VALUE }
        : {}),
    }) + "\n",
  );
  process.stderr.write("diag\n");
});
