import { describe, expect, test } from "vitest";

import {
  infrastructureSteps,
  runInfrastructureSteps,
} from "./verify-infrastructure.mjs";

describe("production infrastructure verification", () => {
  test("runs every infrastructure boundary in a fixed order", () => {
    expect(infrastructureSteps.map((step) => step.name)).toEqual([
      "production topology validation",
      "bootstrap playbook syntax",
      "monitoring playbook syntax",
      "migration playbook syntax",
      "deployment playbook syntax",
      "release playbook syntax",
      "rollback playbook syntax",
      "backup playbook syntax",
      "restore playbook syntax",
      "Ansible lint",
      "infrastructure contract tests",
      "production OpenSpec strict validation",
      "confidentiality and credential scan",
    ]);
  });

  test("stops at the first infrastructure failure", () => {
    const calls: string[] = [];
    const steps = [
      { name: "one", command: "tool", args: ["one"] },
      { name: "two", command: "tool", args: ["two"] },
      { name: "three", command: "tool", args: ["three"] },
    ];

    expect(() =>
      runInfrastructureSteps(steps, (step) => {
        calls.push(step.name);
        if (step.name === "two") throw new Error("expected failure");
      }),
    ).toThrow("expected failure");
    expect(calls).toEqual(["one", "two"]);
  });
});
