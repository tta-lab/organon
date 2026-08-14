import { describe, expect, it } from "vitest";

import {
  assertNativePackagesFirst,
  packagePublishPlan,
} from "../../../scripts/publish-packages.mjs";

describe("release publish plan", () => {
  it("uses the actual release plan to publish every native package before any main package", () => {
    const plan = packagePublishPlan();

    expect(plan).toHaveLength(16);
    expect(plan.slice(0, 12).every((entry) => entry.kind === "native")).toBe(true);
    expect(plan.slice(12).every((entry) => entry.kind === "main")).toBe(true);
    expect(() => assertNativePackagesFirst([...plan].reverse())).toThrow(
      "native packages must be published before main packages",
    );
  });
});
