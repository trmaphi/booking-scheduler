import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";

const webUrl = `http://localhost:${process.env.WEB_PORT || "3000"}`;
const apiUrl = `http://localhost:${process.env.API_PORT || "8080"}`;

async function get(path, baseUrl) {
  const response = await fetch(new URL(path, baseUrl), {
    signal: AbortSignal.timeout(10_000),
  });
  assert.equal(
    response.status,
    200,
    `${path} returned HTTP ${response.status}`,
  );
  return response;
}

async function checkHealth(path) {
  const response = await get(path, apiUrl);
  const body = await response.json();
  assert.equal(body.status, "ok", `${path} did not report status ok`);
  console.log(`PASS ${path}: HTTP 200, status ok`);
}

async function main() {
  const response = await get("/", webUrl);
  const html = await response.text();
  assert.match(html, /<h1[^>]*>Book your next service with confidence\.<\/h1>/);
  console.log("PASS /: HTTP 200, booking heading present");

  await checkHealth("/api/v1/health/live");
  await checkHealth("/api/v1/health/ready");

  const schema = execFileSync(
    "docker",
    [
      "compose",
      "exec",
      "-T",
      "postgres",
      "psql",
      "-U",
      "scheduler",
      "-d",
      "scheduler",
      "-At",
      "-v",
      "ON_ERROR_STOP=1",
      "-c",
      "select exists (select 1 from schema_migrations where name = '001_initial_schema.sql') and to_regclass('public.appointments') is not null",
    ],
    { encoding: "utf8" },
  ).trim();
  assert.equal(
    schema,
    "t",
    "initial migration or appointments table is missing",
  );
  console.log(
    "PASS PostgreSQL: initial migration and appointments table present",
  );
}

main().catch((error) => {
  console.error(`Local stack smoke failed: ${error.message}`);
  process.exitCode = 1;
});
