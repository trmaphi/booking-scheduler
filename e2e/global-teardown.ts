import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import path from "node:path";

export default async function globalTeardown() {
  const outputDir = process.env.E2E_OUTPUT_DIR ?? "./test-results";
  const expected = JSON.parse(
    await readFile(path.join(outputDir, "expected-appointment.json"), "utf8"),
  ) as { id: string };
  const apiBaseURL = process.env.E2E_API_BASE_URL ?? "http://localhost:8080";
  const response = await fetch(
    `${apiBaseURL}/api/v1/appointments/${expected.id}`,
  );
  if (!response.ok) {
    throw new Error(
      `Persisted appointment was not retrievable after the browser closed: HTTP ${response.status}`,
    );
  }
  const appointment = await response.json();
  assert.deepStrictEqual(
    appointment,
    expected,
    "Persisted appointment changed after the browser closed.",
  );
}
