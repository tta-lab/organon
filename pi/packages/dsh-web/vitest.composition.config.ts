import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    include: ["test/composition-smoke.ts"],
    environment: "node",
  },
});
