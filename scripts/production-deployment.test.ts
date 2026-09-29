import { readFileSync } from "node:fs";
import { resolve } from "node:path";

import { describe, expect, test } from "vitest";
import { parse } from "yaml";

import {
  validateProductionEnvironment,
  validateProductionTopology,
} from "./check-production-deployment.mjs";

const root = resolve(import.meta.dirname, "..");
const validEnvironment = {
  IMAGE_TAG: "0123456789abcdef0123456789abcdef01234567",
  WEB_IMAGE: "ghcr.io/example/booking-scheduler-web",
  API_IMAGE: "ghcr.io/example/booking-scheduler-api",
  APP_DOMAIN: "booking.example.invalid",
  POSTGRES_DB: "scheduler",
  POSTGRES_USER: "scheduler",
  POSTGRES_PASSWORD: "fake-test-password",
};

describe("production deployment inputs", () => {
  test.each(["", "latest", "main", "0123456"])(
    "rejects non-immutable image tag %j",
    (IMAGE_TAG) => {
      expect(() =>
        validateProductionEnvironment({ ...validEnvironment, IMAGE_TAG }),
      ).toThrow(/IMAGE_TAG/);
    },
  );

  test.each(["", "https://example.com", "bad_domain", "localhost:3000"])(
    "rejects unsafe application domain %j",
    (APP_DOMAIN) => {
      expect(() =>
        validateProductionEnvironment({ ...validEnvironment, APP_DOMAIN }),
      ).toThrow(/APP_DOMAIN/);
    },
  );

  test("accepts a complete non-secret test environment", () => {
    expect(() => validateProductionEnvironment(validEnvironment)).not.toThrow();
  });
});

describe("production Compose topology", () => {
  const topology = () =>
    parse(readFileSync(resolve(root, "compose.production.yaml"), "utf8"));

  test("publishes only proxy ports 80 and 443", () => {
    const compose = topology();
    expect(validateProductionTopology(compose)).toBeUndefined();
    expect(compose.services.proxy.ports).toEqual(["80:80", "443:443"]);
    for (const service of ["postgres", "migrate", "api", "web"]) {
      expect(compose.services[service].ports).toBeUndefined();
    }
  });

  test("gates application startup on migration and health", () => {
    const compose = topology();
    expect(compose.services.migrate.depends_on.postgres.condition).toBe(
      "service_healthy",
    );
    expect(compose.services.api.depends_on.migrate.condition).toBe(
      "service_completed_successfully",
    );
    expect(compose.services.web.depends_on.api.condition).toBe(
      "service_healthy",
    );
  });

  test("persists database and Caddy state with bounded logs", () => {
    const compose = topology();
    expect(Object.keys(compose.volumes)).toEqual(
      expect.arrayContaining(["postgres-data", "caddy-data", "caddy-config"]),
    );
    for (const service of Object.values(compose.services) as Array<{
      healthcheck?: unknown;
      restart?: string;
      logging: { options: Record<string, string> };
    }>) {
      expect(service.healthcheck).toBeDefined();
      expect(service.restart).toMatch(/^(?:unless-stopped|no)$/);
      expect(service.logging.options).toMatchObject({
        "max-size": "10m",
        "max-file": "5",
      });
    }
  });

  test("keeps committed environment examples free of values", () => {
    const example = readFileSync(
      resolve(root, "deploy/production.env.example"),
      "utf8",
    );
    expect(example).not.toMatch(/=(?!\s*(?:#|$)).+/m);
  });
});
