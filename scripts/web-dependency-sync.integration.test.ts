// @vitest-environment node

import { execFile } from "node:child_process";
import {
  copyFile,
  mkdir,
  mkdtemp,
  readFile,
  rm,
  writeFile,
} from "node:fs/promises";
import { resolve, join } from "node:path";
import { promisify } from "node:util";

import { expect, it } from "vitest";

const execFileAsync = promisify(execFile);
const root = process.cwd();

async function run(command: string, args: string[], cwd = root) {
  return execFileAsync(command, args, {
    cwd,
    timeout: 180_000,
    maxBuffer: 4 * 1024 * 1024,
  });
}

it.skipIf(process.env.RUN_COMPOSE_INTEGRATION !== "1")(
  "refreshes a retained web dependency volume without deleting PostgreSQL data",
  async () => {
    const fixture = await mkdtemp(resolve(".stack-deps-test-"));
    const composePath = join(fixture, "compose.json");
    try {
      await mkdir(join(fixture, "src", "app"), { recursive: true });
      await Promise.all([
        copyFile("package.json", join(fixture, "package.json")),
        copyFile("pnpm-lock.yaml", join(fixture, "pnpm-lock.yaml")),
        copyFile("Dockerfile.web.dev", join(fixture, "Dockerfile.web.dev")),
        writeFile(
          join(fixture, "src", "app", "layout.tsx"),
          "export default function Layout({ children }: { children: React.ReactNode }) { return <html><body>{children}</body></html>; }\n",
        ),
        writeFile(
          join(fixture, "src", "app", "page.tsx"),
          "export default function Page() { return <h1>Dependency sync test</h1>; }\n",
        ),
      ]);

      const { stdout } = await run("docker", [
        "compose",
        "config",
        "--format",
        "json",
      ]);
      const resolved = JSON.parse(stdout);
      const postgres = resolved.services.postgres;
      const web = resolved.services.web;
      delete postgres.ports;
      delete postgres.networks;
      postgres.volumes[0].source = "postgres-data";
      delete web.ports;
      delete web.networks;
      delete web.depends_on;
      web.build.context = fixture;
      web.volumes[0].source = fixture;
      const compose = {
        name: `scheduler-deps-${Date.now()}`,
        services: { postgres, web },
        volumes: { "postgres-data": {}, "web-modules": {}, "web-next": {} },
      };
      await writeFile(composePath, JSON.stringify(compose));

      const docker = (...args: string[]) =>
        run("docker", ["compose", "-f", composePath, ...args], fixture);

      await docker("up", "--build", "-d", "--wait");
      await docker(
        "exec",
        "-T",
        "postgres",
        "psql",
        "-U",
        "scheduler",
        "-d",
        "scheduler",
        "-c",
        "create table volume_marker (value text not null); insert into volume_marker values ('retained');",
      );

      await run(
        "pnpm",
        ["add", "-D", "is-number@7.0.0", "--lockfile-only", "--ignore-scripts"],
        fixture,
      );
      const packageJson = JSON.parse(
        await readFile(join(fixture, "package.json"), "utf8"),
      );
      expect(packageJson.devDependencies["is-number"]).toBe("7.0.0");

      await docker("up", "--build", "-d", "--wait");
      const marker = await docker(
        "exec",
        "-T",
        "postgres",
        "psql",
        "-U",
        "scheduler",
        "-d",
        "scheduler",
        "-Atqc",
        "select value from volume_marker",
      );
      expect(marker.stdout.trim()).toBe("retained");

      const installed = await docker(
        "exec",
        "-T",
        "web",
        "node",
        "-e",
        'process.stdout.write(require("is-number")(42) ? "installed" : "missing")',
      );
      expect(installed.stdout).toBe("installed");
    } finally {
      if (await readFile(composePath, "utf8").catch(() => null)) {
        await run("docker", [
          "compose",
          "-f",
          composePath,
          "down",
          "--volumes",
          "--remove-orphans",
        ]);
      }
      await rm(fixture, { recursive: true, force: true });
    }
  },
  240_000,
);
