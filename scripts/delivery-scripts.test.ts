import { execFileSync, spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { mkdtempSync, mkdirSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { pathToFileURL } from "node:url";

import { describe, expect, test } from "vitest";

const repositoryRoot = resolve(import.meta.dirname, "..");
const scanner = join(repositoryRoot, "scripts/confidentiality-scan.mjs");
const decodedWords = (values: string[]) =>
  values.map((value) => Buffer.from(value, "base64").toString("utf8"));
const prohibitedSentinel = ["Restricted", "Brand", "42"].join("");
const credentialSentinel = ["ghp", "abcdefghijklmnopqrstuvwxyz123456"].join(
  "_",
);
const sourceFingerprint = decodedWords([
  "Y29uZmlkZW50aWFs",
  "Y29kaW5n",
  "Y2hhbGxlbmdl",
]).join(" ");
const secretFilename = ["token", credentialSentinel, "txt"].join(".");
const fingerprintFilename =
  decodedWords(["Y29uZmlkZW50aWFs", "Y29kaW5n", "Y2hhbGxlbmdl"]).join("-") +
  ".txt";

function git(cwd: string, ...args: string[]) {
  return execFileSync("git", args, { cwd, encoding: "utf8" }).trim();
}

function gitWithEnv(
  cwd: string,
  env: Record<string, string>,
  ...args: string[]
) {
  return execFileSync("git", args, {
    cwd,
    encoding: "utf8",
    env: { ...process.env, ...env },
  }).trim();
}

function repository() {
  const cwd = mkdtempSync(join(tmpdir(), "scheduler-scan-"));
  git(cwd, "init", "-q");
  git(cwd, "config", "user.name", "Delivery Test");
  git(cwd, "config", "user.email", "delivery@example.invalid");
  writeFileSync(join(cwd, "README.md"), "safe\n");
  git(cwd, "add", ".");
  git(cwd, "commit", "-qm", "chore: initial safe content");
  return cwd;
}

function scan(cwd: string, prohibitedTerms = "") {
  return spawnSync(process.execPath, [scanner], {
    cwd,
    encoding: "utf8",
    env: { ...process.env, CONFIDENTIALITY_PROHIBITED_TERMS: prohibitedTerms },
  });
}

function scanWithDigest(cwd: string, value: string) {
  const digest = createHash("sha256").update(value.toLowerCase()).digest("hex");
  return spawnSync(process.execPath, [scanner], {
    cwd,
    encoding: "utf8",
    env: {
      ...process.env,
      CONFIDENTIALITY_PROHIBITED_DIGESTS: `${value.length}:${digest}`,
    },
  });
}

describe("delivery verification contract", () => {
  test("covers every release command class in order and checks cleanliness last", async () => {
    const { deliverySteps } = await import(
      pathToFileURL(join(repositoryRoot, "scripts/verify-delivery.mjs")).href
    );
    const names = deliverySteps.map((step: { name: string }) => step.name);

    expect(names[0]).toBe("reset empty volumes");
    expect(names).toEqual(
      expect.arrayContaining([
        "generated client consistency",
        "frontend format",
        "frontend lint",
        "frontend unit tests",
        "frontend type check",
        "frontend production build",
        "Go formatting",
        "Go vet",
        "Go tests with PostgreSQL and race detector",
        "Go repeated concurrency tests",
        "Go production build",
        "start migrated and seeded stack",
        "Compose smoke checks",
        "Playwright booking journey",
        "OpenSpec strict validation",
        "production infrastructure",
        "confidentiality scan",
        "clean Git tree",
      ]),
    );
    expect(names.at(-1)).toBe("clean Git tree");
    expect(names.indexOf("generated client consistency")).toBeLessThan(
      names.indexOf("clean Git tree"),
    );
    expect(names.indexOf("frontend production build")).toBeLessThan(
      names.indexOf("clean Git tree"),
    );
  });

  test("runs sequentially and stops at the first failure", async () => {
    const { runSteps } = await import(
      pathToFileURL(join(repositoryRoot, "scripts/verify-delivery.mjs")).href
    );
    const calls: string[] = [];
    const steps = [
      { name: "one", command: "tool", args: ["one"] },
      { name: "two", command: "tool", args: ["two"] },
      { name: "three", command: "tool", args: ["three"] },
    ];

    expect(() =>
      runSteps(steps, (step: { name: string }) => {
        calls.push(step.name);
        if (step.name === "two") throw new Error("expected failure");
      }),
    ).toThrow("expected failure");
    expect(calls).toEqual(["one", "two"]);
  });
});

describe("confidentiality scanner", () => {
  test("accepts a safe repository", () => {
    const result = scan(repository());
    expect(result.status, result.stderr).toBe(0);
    expect(result.stdout).toContain("Confidentiality scan passed");
  });

  test("accepts approved production deployment services", () => {
    const cwd = repository();
    const approvedProvider = ["cloud", "flare"].join("");
    mkdirSync(join(cwd, ".github/workflows"), { recursive: true });
    mkdirSync(join(cwd, "infra/ansible/group_vars"), { recursive: true });
    writeFileSync(
      join(cwd, ".github/workflows/publish-images.yml"),
      [
        "name: publish images",
        "jobs:",
        "  publish:",
        "    steps:",
        "      - uses: docker/login-action@v3",
        "        with:",
        "          registry: ghcr.io",
      ].join("\n"),
    );
    writeFileSync(
      join(cwd, "infra/ansible/group_vars/production.example.yml"),
      [
        `provider: ${approvedProvider}`,
        "backup_endpoint: https://account.example.invalid",
        "backup_backend: r2",
        "certificate_proxy: caddy",
      ].join("\n"),
    );
    git(cwd, "add", ".");

    const result = scan(cwd);
    expect(result.status, result.stdout).toBe(0);
  });

  test("accepts approved production monitoring service", () => {
    const cwd = repository();
    const approvedProvider = ["gra", "fana"].join("");
    mkdirSync(join(cwd, "infra/ansible/group_vars"), { recursive: true });
    writeFileSync(
      join(cwd, "infra/ansible/group_vars/monitoring.example.yml"),
      `provider: ${approvedProvider}\n`,
    );
    git(cwd, "add", ".");

    const result = scan(cwd);
    expect(result.status, result.stdout).toBe(0);
  });

  test("rejects unapproved monitoring provider", () => {
    const cwd = repository();
    mkdirSync(join(cwd, "infra/ansible/group_vars"), { recursive: true });
    writeFileSync(
      join(cwd, "infra/ansible/group_vars/monitoring.example.yml"),
      ["provider", ["data", "dog"].join("")].join(": ") + "\n",
    );
    git(cwd, "add", ".");

    const result = scan(cwd);
    expect(result.status).toBe(1);
    expect(result.stdout).toContain("category=provider_commitment");
  });

  test("rejects unapproved provider commitment", () => {
    const cwd = repository();
    writeFileSync(
      join(cwd, "deploy.yaml"),
      ["provider", ["acme", "cloud"].join("")].join(": ") + "\n",
    );
    git(cwd, "add", ".");

    const result = scan(cwd);
    expect(result.status).toBe(1);
    expect(result.stdout).toContain("category=provider_commitment");
  });

  test("still rejects provider credentials", () => {
    const cwd = repository();
    const fakeSecret = ["FAKE", "SECRET", "MARKER", "1234567890"].join("_");
    writeFileSync(join(cwd, "deploy.env"), `token=${fakeSecret}\n`);
    git(cwd, "add", ".");

    const result = scan(cwd);
    expect(result.status).toBe(1);
    expect(result.stdout).toContain("category=credential");
    expect(result.stdout).not.toContain(fakeSecret);
  });

  test("rejects a configured non-reversible prohibited-term digest", () => {
    const cwd = repository();
    writeFileSync(join(cwd, "notes.txt"), `${prohibitedSentinel}\n`);
    git(cwd, "add", ".");
    git(cwd, "commit", "-qm", "chore: add digest sentinel");

    const result = scanWithDigest(cwd, prohibitedSentinel);
    expect(result.status).toBe(1);
    expect(result.stdout).toContain("category=prohibited_term");
    expect(result.stdout).not.toContain(prohibitedSentinel);
  });

  test.each([
    [
      "tracked prohibited text",
      "notes.txt",
      prohibitedSentinel,
      "prohibited_term",
    ],
    ["PDF path and content", "brief.PDF", "%PDF-1.7", "pdf_artifact"],
    [
      "credential pattern",
      "settings.txt",
      `token=${credentialSentinel}`,
      "credential",
    ],
    [
      "non-fictional fixture",
      "fixtures/customers.sql",
      "customer@example.com",
      "fixture_policy",
    ],
    [
      "source-document fingerprint",
      "notes.txt",
      sourceFingerprint,
      "source_fingerprint",
    ],
    [
      "provider commitment",
      "vercel.json",
      '{"project":"demo"}',
      "provider_commitment",
    ],
  ])("rejects %s", (_name, path, content, category) => {
    const cwd = repository();
    mkdirSync(join(cwd, path, "..").replace(/\/\.\.$/, ""), {
      recursive: true,
    });
    writeFileSync(join(cwd, path), `${content}\n`);
    git(cwd, "add", ".");
    const result = scan(cwd, prohibitedSentinel);
    expect(result.status).toBe(1);
    expect(result.stdout).toContain(`category=${category}`);
    expect(result.stdout).not.toContain(prohibitedSentinel);
    expect(result.stdout).not.toContain(credentialSentinel);
  });

  test.each([
    [
      "real-looking name",
      "fixtures/customers.yaml",
      "name: Priya Nair",
      "fixture_policy",
    ],
    [
      "phone number",
      "fixtures/customers.yaml",
      "phone: +44 20 7946 0958",
      "fixture_policy",
    ],
    [
      "postal address",
      "fixtures/customers.yaml",
      "address: 221B Baker Street",
      "fixture_policy",
    ],
    [
      "registration",
      "fixtures/vehicles.yaml",
      "registration: AB12 CDE",
      "fixture_policy",
    ],
    [
      "email",
      "fixtures/customers.yaml",
      "email: person@commercial.example.com",
      "fixture_policy",
    ],
  ])(
    "rejects a non-allowlisted fixture %s",
    (_name, path, content, category) => {
      const cwd = repository();
      mkdirSync(join(cwd, "fixtures"), { recursive: true });
      writeFileSync(join(cwd, path), `${content}\n`);
      git(cwd, "add", ".");
      const result = scan(cwd);
      expect(result.status).toBe(1);
      expect(result.stdout).toContain(`category=${category}`);
      expect(result.stdout).not.toContain(content);
    },
  );

  test.each([
    [
      "compact JSON object",
      "fixtures/customer.json",
      JSON.stringify({ name: "Priya Nair", email: "safe@example.invalid" }),
    ],
    [
      "JSON array with quoted keys",
      "fixtures/customers.json",
      JSON.stringify([{ phone: "+44 20 7946 0958" }]),
    ],
    ["quoted YAML key", "fixtures/vehicle.yaml", '"registration": "AB12 CDE"'],
    [
      "CSV header and row",
      "fixtures/customers.csv",
      'name,email\n"Priya Nair",safe@example.invalid',
    ],
    [
      "SQL column/value mapping",
      "seed/people.sql",
      "insert into people (id, full_name, phone) values (1, 'Priya Nair', '+44 20 7946 0958');",
    ],
  ])("rejects format-aware fixture evasion in %s", (_name, path, content) => {
    const cwd = repository();
    mkdirSync(join(cwd, path, "..").replace(/\/\.\.$/, ""), {
      recursive: true,
    });
    writeFileSync(join(cwd, path), `${content}\n`);
    git(cwd, "add", ".");
    const result = scan(cwd);
    expect(result.status).toBe(1);
    expect(result.stdout).toContain("category=fixture_policy");
    expect(result.stdout).not.toContain("Priya Nair");
  });

  test.each([
    ["block sequence", "- name: Priya Nair"],
    ["flow mapping", "customer: {name: Priya Nair}"],
    ["nested flow arrays", "groups: [{customers: [{name: Priya Nair}]}]"],
  ])("rejects recursive YAML fixture evasion in %s", (_name, content) => {
    const cwd = repository();
    mkdirSync(join(cwd, "fixtures"), { recursive: true });
    writeFileSync(join(cwd, "fixtures/data.yaml"), `${content}\n`);
    git(cwd, "add", ".");
    const result = scan(cwd);
    expect(result.status).toBe(1);
    expect(result.stdout).toContain("category=fixture_policy");
  });

  test("does not classify unrelated SQL status and description fields as names", () => {
    const cwd = repository();
    mkdirSync(join(cwd, "seed"), { recursive: true });
    writeFileSync(
      join(cwd, "seed/work.sql"),
      "insert into work_items (status, description) values ('Needs Review', 'Awaiting Parts');\n",
    );
    git(cwd, "add", ".");
    const result = scan(cwd);
    expect(result.status, result.stdout).toBe(0);
  });

  test("accepts allowlisted values across compact JSON and CSV fixtures", () => {
    const cwd = repository();
    mkdirSync(join(cwd, "fixtures"), { recursive: true });
    writeFileSync(
      join(cwd, "fixtures/demo.json"),
      JSON.stringify([
        {
          name: "Jordan Lee",
          email: "jordan.lee@example.invalid",
          address: "100 Riverside Way",
          registration: "DEMO-001",
          phone: "+1 202-555-0100",
        },
      ]),
    );
    writeFileSync(
      join(cwd, "fixtures/demo.csv"),
      "name,email,address,registration,phone\nJordan Lee,jordan.lee@example.invalid,100 Riverside Way,DEMO-001,+1 202-555-0100\n",
    );
    git(cwd, "add", ".");
    const result = scan(cwd);
    expect(result.status, result.stdout).toBe(0);
  });

  test("accepts the reviewed neutral fixture allowlist", () => {
    const cwd = repository();
    mkdirSync(join(cwd, "fixtures"), { recursive: true });
    writeFileSync(
      join(cwd, "fixtures/demo.yaml"),
      [
        "name: Jordan Lee",
        "email: jordan.lee@example.invalid",
        "address: 100 Riverside Way",
        "registration: DEMO-001",
        "phone: +1 202-555-0100",
      ].join("\n"),
    );
    git(cwd, "add", ".");
    const result = scan(cwd);
    expect(result.status, result.stdout).toBe(0);
  });

  test.each([
    [
      "alternate deployment YAML",
      "deploy.yaml",
      ["provider", "aws"].join(": "),
    ],
    ["Terraform provider", "main.tf", ["provider", '"google" {}'].join(" ")],
    [
      "alternate Terraform provider",
      "preview.tf",
      ["provider", `"${["ne", "on"].join("")}" {}`].join(" "),
    ],
    [
      "deployment workflow",
      ".github/workflows/deploy.yml",
      ["uses", "cloudflare/pages-action@v1"].join(": "),
    ],
    [
      "text commitment",
      "notes.txt",
      ["Production hosting", "Vercel"].join(": "),
    ],
  ])("rejects provider commitment in %s", (_name, path, content) => {
    const cwd = repository();
    mkdirSync(join(cwd, path, "..").replace(/\/\.\.$/, ""), {
      recursive: true,
    });
    writeFileSync(join(cwd, path), `${content}\n`);
    git(cwd, "add", ".");
    const result = scan(cwd);
    expect(result.status).toBe(1);
    expect(result.stdout).toContain("category=provider_commitment");
  });

  test.each([
    ["unlisted Terraform provider", "main.tf", 'provider "digitalocean" {}'],
    ["another Terraform provider", "main.tf", 'provider "heroku" {}'],
    ["unknown Terraform provider", "main.tf", 'provider "acmecloud" {}'],
    [
      "remote Terraform backend",
      "state.tf",
      'terraform { backend "remote" {} }',
    ],
    [
      "remote Terraform module",
      "module.tf",
      'module "compute" { source = "acmecloud/compute/service" }',
    ],
    ["unlisted YAML provider", "deploy.yaml", "provider: digitalocean"],
    ["unknown JSON provider", "deploy.json", '{"provider":"acmecloud"}'],
  ])("rejects generic provider configuration in %s", (_name, path, content) => {
    const cwd = repository();
    writeFileSync(join(cwd, path), `${content}\n`);
    git(cwd, "add", ".");
    const result = scan(cwd);
    expect(result.status).toBe(1);
    expect(result.stdout).toContain("category=provider_commitment");
  });

  test.each([
    ["local Terraform provider", 'provider "postgresql" {}'],
    ["local Terraform backend", 'terraform { backend "local" {} }'],
    ["local Terraform module", 'module "local" { source = "./modules/local" }'],
  ])("accepts %s", (_name, content) => {
    const cwd = repository();
    writeFileSync(join(cwd, "main.tf"), `${content}\n`);
    git(cwd, "add", ".");
    const result = scan(cwd);
    expect(result.status, result.stdout).toBe(0);
  });

  test("accepts a provider client as a library dependency", () => {
    const cwd = repository();
    const packageName = ["@neon", "database/serverless"].join("");
    writeFileSync(
      join(cwd, "package.json"),
      JSON.stringify({ dependencies: { [packageName]: "1.0.0" } }),
    );
    git(cwd, "add", ".");
    const result = scan(cwd);
    expect(result.status, result.stdout).toBe(0);
  });

  test("rejects a provider-specific branch action in a workflow", () => {
    const cwd = repository();
    const owner = ["neon", "database"].join("");
    const action = [owner, "create-branch-action@v6"].join("/");
    mkdirSync(join(cwd, ".github/workflows"), { recursive: true });
    writeFileSync(
      join(cwd, ".github/workflows/preview.yml"),
      ["steps:", `  - uses: ${action}`].join("\n"),
    );
    git(cwd, "add", ".");
    const result = scan(cwd);
    expect(result.status).toBe(1);
    expect(result.stdout).toContain("category=provider_commitment");
    expect(result.stdout).not.toContain(owner);
  });

  test("accepts documentation that explicitly leaves hosting undecided", () => {
    const cwd = repository();
    writeFileSync(
      join(cwd, "README.md"),
      "Production hosting and managed database providers remain undecided.\n",
    );
    git(cwd, "add", ".");
    const result = scan(cwd);
    expect(result.status, result.stdout).toBe(0);
  });

  test("rejects expanded assessment source phrasing", () => {
    const cwd = repository();
    const phrase = decodedWords([
      "Q29uZmlkZW50aWFs",
      "dGVjaG5pY2Fs",
      "YXNzZXNzbWVudA==",
    ]).join(" ");
    writeFileSync(join(cwd, "notes.txt"), `${phrase}\n`);
    git(cwd, "add", ".");
    const result = scan(cwd);
    expect(result.status).toBe(1);
    expect(result.stdout).toContain("category=source_fingerprint");
    expect(result.stdout).not.toContain(phrase);
  });

  test.each([
    [
      decodedWords([
        "VGVjaG5pY2Fs",
        "YXNzZXNzbWVudA==",
        "c291cmNl",
        "YnJpZWY=",
      ]).join(" "),
    ],
    [decodedWords(["Q29kaW5n", "Y2hhbGxlbmdl", "ZG9jdW1lbnQ="]).join(" ")],
    [decodedWords(["Q29uZmlkZW50aWFs", "ZXhlcmNpc2U=", "YnJpZWY="]).join(" ")],
  ])("rejects tokenized source fingerprint combinations", (phrase) => {
    const cwd = repository();
    writeFileSync(join(cwd, "notes.txt"), `${phrase}\n`);
    git(cwd, "add", ".");
    const result = scan(cwd);
    expect(result.status).toBe(1);
    expect(result.stdout).toContain("category=source_fingerprint");
    expect(result.stdout).not.toContain(phrase);
  });

  test("does not treat local Docker and PostgreSQL configuration as a provider commitment", () => {
    const cwd = repository();
    writeFileSync(
      join(cwd, "compose.yaml"),
      "services:\n  database:\n    image: postgres:16-alpine\n",
    );
    git(cwd, "add", ".");
    const result = scan(cwd);
    expect(result.status, result.stdout).toBe(0);
  });

  test.each([
    ["credential", secretFilename, "safe"],
    ["source fingerprint", fingerprintFilename, "safe"],
  ])("never emits a sensitive %s filename", (_name, path, content) => {
    const cwd = repository();
    writeFileSync(join(cwd, path), `${content}\n`);
    git(cwd, "add", ".");
    const result = scan(cwd);
    expect(result.status).toBe(1);
    expect(result.stdout).toMatch(/location=[a-f0-9]{16}/);
    expect(result.stdout).not.toContain(path);
    expect(result.stdout).not.toContain(credentialSentinel);
    expect(result.stdout).not.toContain(fingerprintFilename.slice(0, -4));
  });

  test("detects a prohibited term in a newline-containing historical path", () => {
    const cwd = repository();
    const path = `archive\nrecord.txt`;
    writeFileSync(join(cwd, path), `${prohibitedSentinel}\n`);
    git(cwd, "add", ".");
    git(cwd, "commit", "-qm", "chore: unusual path fixture");
    const result = scan(cwd, prohibitedSentinel);
    expect(result.status).toBe(1);
    expect(result.stdout).toContain("scope=history_blob");
    expect(result.stdout).not.toContain("archive");
    expect(result.stdout).not.toContain(prohibitedSentinel);
  });

  test("rejects prohibited text retained only in a reachable historical blob", () => {
    const cwd = repository();
    writeFileSync(join(cwd, "old.txt"), `${prohibitedSentinel}\n`);
    git(cwd, "add", ".");
    git(cwd, "commit", "-qm", "chore: add temporary note");
    git(cwd, "rm", "-q", "old.txt");
    git(cwd, "commit", "-qm", "chore: remove temporary note");

    const result = scan(cwd, prohibitedSentinel);
    expect(result.status).toBe(1);
    expect(result.stdout).toContain("scope=history_blob");
    expect(result.stdout).not.toContain(prohibitedSentinel);
  });

  test("rejects prohibited text retained only in a commit message", () => {
    const cwd = repository();
    writeFileSync(join(cwd, "change.txt"), "safe\n");
    git(cwd, "add", ".");
    git(cwd, "commit", "-qm", `docs: ${prohibitedSentinel} note`);

    const result = scan(cwd, prohibitedSentinel);
    expect(result.status).toBe(1);
    expect(result.stdout).toContain("scope=commit_metadata");
    expect(result.stdout).not.toContain(prohibitedSentinel);
  });

  test.each([
    ["author", { GIT_AUTHOR_NAME: prohibitedSentinel }],
    ["committer", { GIT_COMMITTER_NAME: prohibitedSentinel }],
  ])("rejects prohibited text in commit %s metadata", (_field, env) => {
    const cwd = repository();
    writeFileSync(join(cwd, "metadata.txt"), "safe\n");
    git(cwd, "add", ".");
    gitWithEnv(cwd, env, "commit", "-qm", "chore: metadata test");
    const result = scan(cwd, prohibitedSentinel);
    expect(result.status).toBe(1);
    expect(result.stdout).toContain("scope=commit_metadata");
    expect(result.stdout).not.toContain(prohibitedSentinel);
  });

  test("rejects prohibited text in annotated tag metadata", () => {
    const cwd = repository();
    git(cwd, "tag", "-a", "review-v1", "-m", `${prohibitedSentinel} release`);
    const result = scan(cwd, prohibitedSentinel);
    expect(result.status).toBe(1);
    expect(result.stdout).toContain("scope=tag_metadata");
    expect(result.stdout).not.toContain(prohibitedSentinel);
  });

  test("rejects prohibited text in a lightweight tag ref name", () => {
    const cwd = repository();
    const tagName = `release-${prohibitedSentinel}`;
    git(cwd, "tag", tagName);
    const result = scan(cwd, prohibitedSentinel);
    expect(result.status).toBe(1);
    expect(result.stdout).toContain("scope=tag_ref");
    expect(result.stdout).not.toContain(tagName);
    expect(result.stdout).not.toContain(prohibitedSentinel);
  });

  test("rejects prohibited text in an annotated tag ref name", () => {
    const cwd = repository();
    const tagName = `review-${prohibitedSentinel}`;
    git(cwd, "tag", "-a", tagName, "-m", "safe release metadata");
    const result = scan(cwd, prohibitedSentinel);
    expect(result.status).toBe(1);
    expect(result.stdout).toContain("scope=tag_ref");
    expect(result.stdout).not.toContain(tagName);
    expect(result.stdout).not.toContain(prohibitedSentinel);
  });
});
