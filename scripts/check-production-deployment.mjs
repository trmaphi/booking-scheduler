import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { parse } from "yaml";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const required = [
  "IMAGE_TAG",
  "WEB_IMAGE",
  "API_IMAGE",
  "APP_DOMAIN",
  "POSTGRES_DB",
  "POSTGRES_USER",
  "POSTGRES_PASSWORD",
];

export function validateProductionEnvironment(environment) {
  for (const name of required) {
    if (!String(environment[name] ?? "").trim())
      throw new Error(`${name} is required`);
  }
  if (
    !/^[a-f0-9]{40}$/.test(environment.IMAGE_TAG) ||
    environment.IMAGE_TAG === "latest"
  ) {
    throw new Error(
      "IMAGE_TAG must be a full Git commit SHA and cannot be latest",
    );
  }
  if (
    !/^(?=.{1,253}$)(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$/.test(
      environment.APP_DOMAIN,
    )
  ) {
    throw new Error("APP_DOMAIN must be a safe DNS hostname");
  }
}

export function validateProductionTopology(compose) {
  const services = compose?.services ?? {};
  for (const name of ["postgres", "migrate", "api", "web", "proxy"]) {
    if (!services[name]) throw new Error(`missing production service ${name}`);
  }
  for (const name of ["postgres", "migrate", "api", "web"]) {
    if (services[name].ports?.length)
      throw new Error(`${name} must not publish host ports`);
  }
  const ports = services.proxy.ports ?? [];
  if (JSON.stringify(ports) !== JSON.stringify(["80:80", "443:443"])) {
    throw new Error("proxy must publish only ports 80 and 443");
  }
}

const testEnvironment = {
  IMAGE_TAG: "0123456789abcdef0123456789abcdef01234567",
  WEB_IMAGE: "ghcr.io/example/booking-scheduler-web",
  API_IMAGE: "ghcr.io/example/booking-scheduler-api",
  APP_DOMAIN: "booking.example.invalid",
  POSTGRES_DB: "scheduler",
  POSTGRES_USER: "scheduler",
  POSTGRES_PASSWORD: "fake-test-password",
};

function main() {
  if (process.argv.includes("--smoke")) {
    const origin = process.env.PRODUCTION_ORIGIN;
    if (!origin)
      throw new Error("PRODUCTION_ORIGIN is required for smoke checks");
    execFileSync("curl", ["--fail", "--silent", "--show-error", `${origin}/`], {
      stdio: "inherit",
    });
    execFileSync(
      "curl",
      ["--fail", "--silent", "--show-error", `${origin}/api/v1/health/ready`],
      { stdio: "inherit" },
    );
    return;
  }
  const environment = process.argv.includes("--test")
    ? testEnvironment
    : process.env;
  validateProductionEnvironment(environment);
  const compose = parse(
    readFileSync(resolve(root, "compose.production.yaml"), "utf8"),
  );
  validateProductionTopology(compose);
  console.log("Production deployment configuration is valid.");
}

if (
  process.argv[1] &&
  resolve(process.argv[1]) === fileURLToPath(import.meta.url)
)
  main();
