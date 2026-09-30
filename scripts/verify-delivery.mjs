import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { resolve } from "node:path";

const databaseURL =
  process.env.TEST_DATABASE_URL ??
  `postgresql://scheduler:scheduler@localhost:${process.env.POSTGRES_PORT ?? "55432"}/scheduler`;

export const deliverySteps = [
  { name: "reset empty volumes", command: "pnpm", args: ["stack:reset"] },
  {
    name: "generated client consistency",
    command: "pnpm",
    args: ["api:check"],
  },
  { name: "frontend format", command: "pnpm", args: ["format:check"] },
  { name: "frontend lint", command: "pnpm", args: ["lint"] },
  { name: "frontend unit tests", command: "pnpm", args: ["test"] },
  { name: "frontend type check", command: "pnpm", args: ["typecheck"] },
  { name: "frontend production build", command: "pnpm", args: ["build:check"] },
  { name: "Go formatting", command: "pnpm", args: ["go:format:check"] },
  { name: "Go vet", command: "pnpm", args: ["go:vet"] },
  {
    name: "start migrated and seeded stack",
    command: "pnpm",
    args: ["stack:start"],
  },
  {
    name: "Go tests with PostgreSQL and race detector",
    command: "go",
    args: ["test", "-race", "./...", "-count=1"],
    cwd: "api",
    env: { TEST_DATABASE_URL: databaseURL },
  },
  {
    name: "Go repeated concurrency tests",
    command: "go",
    args: [
      "test",
      "-race",
      "./internal/postgres",
      "-run",
      "TestConcurrentConfirmationAllowsAtMostOneWinner|TestIdempotency",
      "-count=20",
    ],
    cwd: "api",
    env: { TEST_DATABASE_URL: databaseURL },
  },
  {
    name: "Go production build",
    command: "go",
    args: ["build", "./cmd/server", "./cmd/migrate"],
    cwd: "api",
  },
  { name: "Compose smoke checks", command: "pnpm", args: ["stack:smoke"] },
  {
    name: "Playwright booking journey",
    command: "pnpm",
    args: ["test:e2e"],
    env: { E2E_REQUIRE_CLEAN_ARTIFACT: "1" },
  },
  {
    name: "OpenSpec strict validation",
    command: "openspec",
    args: ["validate", "bootstrap-service-scheduler", "--strict"],
  },
  {
    name: "production infrastructure",
    command: "pnpm",
    args: ["verify:infrastructure"],
  },
  {
    name: "confidentiality scan",
    command: "pnpm",
    args: ["scan:confidentiality"],
  },
  {
    name: "clean Git tree",
    command: "git",
    args: ["diff", "--exit-code", "--check", "HEAD"],
    cleanTree: true,
  },
];

function execute(step) {
  if (step.cleanTree) {
    const status = spawnSync(
      "git",
      ["status", "--porcelain=v1", "--untracked-files=all"],
      {
        encoding: "utf8",
      },
    );
    if (status.status !== 0)
      throw new Error("Unable to inspect the Git worktree.");
    if (status.stdout.trim())
      throw new Error("The delivery worktree is not clean.");
  }

  const result = spawnSync(step.command, step.args, {
    cwd: step.cwd,
    env: { ...process.env, ...step.env },
    stdio: "inherit",
  });
  if (result.error) throw result.error;
  if (result.status !== 0) {
    throw new Error(
      `${step.name} failed with exit code ${result.status ?? "unknown"}.`,
    );
  }
}

export function runSteps(steps = deliverySteps, runner = execute) {
  for (const [index, step] of steps.entries()) {
    console.log(`\n[${index + 1}/${steps.length}] ${step.name}`);
    runner(step);
  }
}

const invokedPath = process.argv[1] ? resolve(process.argv[1]) : "";
if (invokedPath === fileURLToPath(import.meta.url)) {
  try {
    runSteps();
    console.log("\nDelivery verification passed.");
  } catch (error) {
    console.error(
      `\nDelivery verification failed: ${error instanceof Error ? error.message : "unknown error"}`,
    );
    process.exitCode = 1;
  }
}
