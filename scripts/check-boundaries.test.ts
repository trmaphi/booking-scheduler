// @vitest-environment node

import { mkdtemp, mkdir, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";

import { describe, expect, it } from "vitest";

import { findBoundaryViolations } from "./check-boundaries.mjs";

describe("booking module boundaries", () => {
  it("rejects framework and database imports from domain modules", async () => {
    const root = await mkdtemp(path.join(tmpdir(), "booking-boundaries-"));
    const domainDir = path.join(root, "src/features/booking/domain");
    await mkdir(domainDir, { recursive: true });
    await writeFile(
      path.join(domainDir, "invalid.ts"),
      'import { NextRequest } from "next/server";\nimport postgres from "postgres";\n',
    );

    const violations = await findBoundaryViolations(root);

    expect(violations).toEqual([
      expect.stringContaining(
        'invalid.ts imports forbidden module "next/server"',
      ),
      expect.stringContaining('invalid.ts imports forbidden module "postgres"'),
    ]);
  });

  it("finds the required booking module roots and accepts their dependencies", async () => {
    const violations = await findBoundaryViolations(process.cwd(), {
      requireModuleRoots: true,
    });

    expect(violations).toEqual([]);
  });
});
