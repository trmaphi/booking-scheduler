import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { readdirSync, readFileSync } from "node:fs";
import { join, relative } from "node:path";

const generatedDirectory = "src/features/booking/api/generated";

function files(directory) {
  return readdirSync(directory, { withFileTypes: true })
    .flatMap((entry) => {
      const path = join(directory, entry.name);
      return entry.isDirectory() ? files(path) : [path];
    })
    .sort();
}

function snapshot() {
  const hash = createHash("sha256");
  for (const path of files(generatedDirectory)) {
    hash.update(relative(generatedDirectory, path));
    hash.update("\0");
    hash.update(readFileSync(path));
    hash.update("\0");
  }
  return hash.digest("hex");
}

const before = snapshot();

execFileSync("pnpm", ["api:generate"], { stdio: "inherit" });

if (snapshot() !== before) {
  console.error(
    "Generated API client is out of date. Run `pnpm api:generate` and commit the result.",
  );
  process.exit(1);
}
