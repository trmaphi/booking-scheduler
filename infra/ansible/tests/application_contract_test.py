from pathlib import Path
import re
import unittest
import yaml


ROOT = Path(__file__).resolve().parents[1]
REPOSITORY = ROOT.parents[1]


class ApplicationContractTest(unittest.TestCase):
    def read(self, relative: str) -> str:
        return (ROOT / relative).read_text()

    def yaml(self, relative: str):
        return yaml.safe_load(self.read(relative))

    def test_validation_precedes_remote_mutation(self):
        tasks = self.read("roles/application/tasks/main.yml")
        self.assertLess(tasks.index("Validate immutable release inputs"), tasks.index("Create release directory"))
        self.assertIn("IMAGE_TAG must be a full Git commit SHA", tasks)
        self.assertIn("APP_DOMAIN must be a safe DNS hostname", tasks)

    def test_secret_files_are_suppressed_and_group_readable_only(self):
        tasks = self.read("roles/application/tasks/main.yml")
        self.assertRegex(tasks, r"(?s)Render protected production environment.*?mode: \"0640\".*?no_log: true")
        self.assertRegex(tasks, r"(?s)Authenticate to GHCR.*?no_log: true")

    def test_postgres_password_escapes_compose_interpolation(self):
        environment = self.read("roles/application/templates/production.env.j2")
        self.assertIn("POSTGRES_PASSWORD='{{ postgres_password", environment)
        self.assertIn('replace("\'", "\\\\\'")', environment)

    def test_migration_failure_stops_before_application_start(self):
        tasks = self.read("roles/application/tasks/main.yml")
        self.assertLess(tasks.index("migrate.yml"), tasks.index("services.yml"))
        self.assertIn("application_phase in ['full', 'migrate']", tasks)
        self.assertIn("application_phase in ['full', 'deploy']", tasks)
        compose = self.read("roles/application/templates/compose.production.yaml.j2")
        self.assertNotIn("service_completed_successfully", compose)
        self.assertRegex(compose, r"(?s)api:.*?depends_on:.*?postgres:.*?service_healthy")

    def test_release_state_is_recorded_only_after_https_checks(self):
        tasks = self.read("roles/application/tasks/services.yml")
        self.assertLess(tasks.index("Verify public HTTPS web"), tasks.index("Record verified active release"))
        self.assertLess(tasks.index("Verify public HTTPS API readiness"), tasks.index("Record verified active release"))

    def test_rollback_requires_explicit_immutable_tag(self):
        rollback = self.read("playbooks/rollback.yml")
        self.assertIn("rollback_tag", rollback)
        self.assertIn("^[a-f0-9]{40}$", rollback)
        self.assertIn("application_phase: deploy", rollback)
        self.assertNotIn("latest", rollback.lower().replace("cannot be latest", ""))

    def test_deploy_and_rollback_never_remove_volumes(self):
        content = "\n".join(
            self.read(path)
            for path in (
                "roles/application/tasks/main.yml",
                "playbooks/deploy.yml",
                "playbooks/rollback.yml",
            )
        )
        self.assertNotRegex(content, r"(?:down|rm)[^\n]*(?:--volumes|-v\b)")
        self.assertNotIn("postgres-data", "\n".join(line for line in content.splitlines() if "remove" in line.lower()))

    def test_ci_publishes_and_deploys_the_same_sha_on_main(self):
        workflow = (REPOSITORY / ".github/workflows/publish-images.yml").read_text()
        self.assertFalse((REPOSITORY / ".github/workflows/migrate-production.yml").exists())
        self.assertFalse((REPOSITORY / ".github/workflows/deploy-production.yml").exists())
        self.assertRegex(workflow, r"(?s)push:.*?branches:.*?main")
        self.assertIn("linux/amd64", workflow)
        self.assertIn("packages: write", workflow)
        self.assertIn("contents: read", workflow)
        self.assertIn("pnpm verify:infrastructure", workflow)
        self.assertIn("@fission-ai/openspec@1.13.1", workflow)
        self.assertLess(workflow.index("pnpm verify:infrastructure"), workflow.index("Build and publish web image"))
        self.assertRegex(workflow, r"(?s)migrate:.*?needs: verify-and-publish.*?runs-on:")
        self.assertRegex(workflow, r"(?s)deploy:.*?needs: migrate.*?runs-on:")
        self.assertGreaterEqual(workflow.count("RELEASE_TAG: ${{ github.sha }}"), 4)
        self.assertGreaterEqual(workflow.count("VPS_SSH_PRIVATE_KEY"), 4)
        self.assertGreaterEqual(workflow.count("ANSIBLE_VAULT_PASSWORD"), 4)
        self.assertGreaterEqual(workflow.count("GHCR_DEPLOY_TOKEN: ${{ secrets.GITHUB_TOKEN }}"), 2)
        self.assertGreaterEqual(workflow.count('application_ghcr_token:$token'), 2)
        self.assertGreaterEqual(workflow.count('ghcr.json'), 4)
        self.assertGreaterEqual(workflow.count("printf '%s\\n' \"$VPS_SSH_PRIVATE_KEY\""), 2)
        self.assertGreaterEqual(workflow.count("--skip-tags backup"), 2)
        self.assertIn("playbooks/migrate.yml", workflow)
        self.assertIn("playbooks/deploy.yml", workflow)
        self.assertNotIn("workflow_call:", workflow)
        self.assertNotRegex(workflow, r"(?i)(?:password|private_key):\s*[^$\s]")


if __name__ == "__main__":
    unittest.main()
