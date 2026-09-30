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

    def test_migration_failure_stops_before_application_start(self):
        tasks = self.read("roles/application/tasks/main.yml")
        migrate = tasks.index("Run database migrations")
        start = tasks.index("Start production application services")
        self.assertLess(migrate, start)
        self.assertIn("service_completed_successfully", self.read("roles/application/templates/compose.production.yaml.j2"))

    def test_release_state_is_recorded_only_after_https_checks(self):
        tasks = self.read("roles/application/tasks/main.yml")
        self.assertLess(tasks.index("Verify public HTTPS web"), tasks.index("Record verified active release"))
        self.assertLess(tasks.index("Verify public HTTPS API readiness"), tasks.index("Record verified active release"))

    def test_rollback_requires_explicit_immutable_tag(self):
        rollback = self.read("playbooks/rollback.yml")
        self.assertIn("rollback_tag", rollback)
        self.assertIn("^[a-f0-9]{40}$", rollback)
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
        self.assertRegex(workflow, r"(?s)push:.*?branches:.*?main")
        self.assertIn("linux/amd64", workflow)
        self.assertGreaterEqual(workflow.count("${{ github.sha }}"), 3)
        self.assertIn("packages: write", workflow)
        self.assertIn("contents: read", workflow)
        self.assertIn("VPS_SSH_PRIVATE_KEY", workflow)
        self.assertIn("ANSIBLE_VAULT_PASSWORD", workflow)
        self.assertIn("GHCR_DEPLOY_TOKEN: ${{ secrets.GITHUB_TOKEN }}", workflow)
        self.assertIn('application_ghcr_token:$token', workflow)
        self.assertIn('ghcr.json', workflow)
        self.assertIn("printf '%s\\n' \"$VPS_SSH_PRIVATE_KEY\"", workflow)
        self.assertIn("--skip-tags backup", workflow)
        self.assertNotRegex(workflow, r"(?i)(?:password|private_key):\s*[^$\s]")


if __name__ == "__main__":
    unittest.main()
