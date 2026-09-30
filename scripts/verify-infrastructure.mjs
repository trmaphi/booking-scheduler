import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { resolve } from "node:path";

const ansibleEnvironment = {
  ANSIBLE_CONFIG: "infra/ansible/ansible.cfg",
};
const syntaxStep = (name, playbook) => ({
  name,
  command: "ansible-playbook",
  args: [
    "--syntax-check",
    "-i",
    "infra/ansible/inventory/production.yml",
    `infra/ansible/playbooks/${playbook}.yml`,
  ],
  env: ansibleEnvironment,
});

export const infrastructureSteps = [
  {
    name: "production topology validation",
    command: "pnpm",
    args: ["production:check"],
  },
  syntaxStep("bootstrap playbook syntax", "bootstrap"),
  syntaxStep("monitoring playbook syntax", "monitoring"),
  syntaxStep("migration playbook syntax", "migrate"),
  syntaxStep("deployment playbook syntax", "deploy"),
  syntaxStep("release playbook syntax", "release"),
  syntaxStep("rollback playbook syntax", "rollback"),
  syntaxStep("backup playbook syntax", "backup"),
  syntaxStep("restore playbook syntax", "restore"),
  {
    name: "Ansible lint",
    command: "ansible-lint",
    args: ["infra/ansible"],
    env: ansibleEnvironment,
  },
  {
    name: "infrastructure contract tests",
    command: "python3",
    args: [
      "-m",
      "unittest",
      "discover",
      "-s",
      "infra/ansible/tests",
      "-p",
      "*_test.py",
    ],
  },
  {
    name: "production OpenSpec strict validation",
    command: "openspec",
    args: ["validate", "deploy-production-vps", "--strict"],
  },
  {
    name: "confidentiality and credential scan",
    command: "pnpm",
    args: ["scan:confidentiality"],
  },
];

function execute(step) {
  const result = spawnSync(step.command, step.args, {
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

export function runInfrastructureSteps(
  steps = infrastructureSteps,
  runner = execute,
) {
  for (const [index, step] of steps.entries()) {
    console.log(`\n[${index + 1}/${steps.length}] ${step.name}`);
    runner(step);
  }
}

const invokedPath = process.argv[1] ? resolve(process.argv[1]) : "";
if (invokedPath === fileURLToPath(import.meta.url)) {
  try {
    runInfrastructureSteps();
    console.log("\nInfrastructure verification passed.");
  } catch (error) {
    console.error(
      `\nInfrastructure verification failed: ${error instanceof Error ? error.message : "unknown error"}`,
    );
    process.exitCode = 1;
  }
}
