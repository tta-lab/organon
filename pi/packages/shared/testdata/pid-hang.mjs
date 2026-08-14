import { writeFileSync } from "node:fs";

const pidPath = process.argv[2];
if (!pidPath) {
  throw new Error("pid path is required");
}
writeFileSync(pidPath, String(process.pid));
setInterval(() => {}, 1000);
