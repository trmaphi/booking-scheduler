// @vitest-environment node

import { readFile } from "node:fs/promises";
import { execFileSync } from "node:child_process";

import { describe, expect, it } from "vitest";

describe("local PostgreSQL development contract", () => {
  it("defines a health-checked PostgreSQL service without hosted credentials", async () => {
    const compose = await readFile("compose.yaml", "utf8");

    expect(compose).toContain("postgres:");
    expect(compose).toContain("pg_isready");
    expect(compose).toContain("${POSTGRES_PORT:-55432}:5432");
    expect(compose).not.toMatch(/neon\.tech/i);
  });

  it("starts migrations, API, and web in dependency order with mounted source and caches", () => {
    const config = JSON.parse(
      execFileSync("docker", ["compose", "config", "--format", "json"], {
        encoding: "utf8",
        env: {
          ...process.env,
          POSTGRES_PORT: "55432",
          API_PORT: "8080",
          WEB_PORT: "3000",
        },
      }),
    );
    const { postgres, migrate, api, web } = config.services;

    expect(Object.keys(config.services).sort()).toEqual([
      "api",
      "migrate",
      "postgres",
      "web",
    ]);
    expect(migrate.depends_on.postgres.condition).toBe("service_healthy");
    expect(api.depends_on.migrate.condition).toBe(
      "service_completed_successfully",
    );
    expect(web.depends_on.api.condition).toBe("service_healthy");
    expect(postgres.ports[0].published).toBe("55432");
    expect(api.environment.DATABASE_URL).toBe(
      "postgresql://scheduler:scheduler@postgres:5432/scheduler",
    );
    expect(web.environment.NEXT_PUBLIC_API_BASE_URL).toBe(
      "http://localhost:8080",
    );
    expect(api.volumes).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ target: "/app", type: "bind" }),
        expect.objectContaining({ target: "/go/pkg/mod", type: "volume" }),
        expect.objectContaining({
          target: "/root/.cache/go-build",
          type: "volume",
        }),
      ]),
    );
    expect(web.volumes).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ target: "/app", type: "bind" }),
        expect.objectContaining({
          target: "/app/node_modules",
          type: "volume",
        }),
        expect.objectContaining({ target: "/app/.next", type: "volume" }),
      ]),
    );
    expect(JSON.stringify(config)).not.toMatch(/neon\.tech|docker\.sock/i);
  });
});
