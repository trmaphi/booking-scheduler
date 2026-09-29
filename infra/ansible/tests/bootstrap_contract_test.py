from pathlib import Path
import unittest
import yaml


ROOT = Path(__file__).resolve().parents[1]


class BootstrapContractTest(unittest.TestCase):
    def read(self, relative: str) -> str:
        return (ROOT / relative).read_text()

    def yaml(self, relative: str):
        return yaml.safe_load(self.read(relative))

    def test_bootstrap_rejects_unsupported_platform_before_roles(self):
        play = self.yaml("playbooks/bootstrap.yml")[0]
        assertions = play["pre_tasks"][0]["ansible.builtin.assert"]["that"]
        self.assertTrue(any("Ubuntu" in item for item in assertions))
        self.assertTrue(any("24.04" in item for item in assertions))
        self.assertTrue(any("x86_64" in item for item in assertions))

    def test_key_installation_precedes_validated_ssh_hardening(self):
        tasks = self.read("roles/security/tasks/main.yml")
        self.assertLess(tasks.index("ansible.posix.authorized_key"), tasks.index("99-booking-scheduler.conf.j2"))
        self.assertIn("validate: /usr/sbin/sshd -t -f %s", tasks)
        template = self.read("roles/security/templates/99-booking-scheduler.conf.j2")
        self.assertIn("PermitRootLogin no", template)
        self.assertIn("PasswordAuthentication no", template)
        self.assertNotRegex(tasks, r"(?i)password\s*:")

    def test_firewall_denies_inbound_and_allows_only_public_ports(self):
        tasks = self.read("roles/security/tasks/main.yml")
        variables = self.read("group_vars/all.yml")
        self.assertIn("default: deny", tasks)
        self.assertEqual(tasks.count("rule: allow"), 1)
        for port in (22, 80, 443):
            self.assertIn(str(port), variables)
        self.assertNotIn("5432", variables)
        self.assertNotIn("8080", variables)

    def test_docker_uses_signed_repository_and_bounded_logs(self):
        tasks = self.read("roles/docker/tasks/main.yml")
        self.assertIn("download.docker.com/linux/ubuntu", tasks)
        self.assertIn("signed-by=", tasks)
        daemon = self.read("roles/docker/templates/daemon.json.j2")
        self.assertIn('"max-size": "10m"', daemon)
        self.assertIn('"max-file": "5"', daemon)

    def test_protected_directories_have_restrictive_modes(self):
        tasks = self.read("roles/directories/tasks/main.yml")
        for path in ("/opt/booking-scheduler", "/etc/booking-scheduler", "/var/lib/booking-scheduler"):
            self.assertIn(path, tasks)
        self.assertIn('mode: "0750"', tasks)

    def test_deploy_account_receives_validated_automation_sudo_policy(self):
        tasks = self.read("roles/security/tasks/main.yml")
        self.assertIn("99-booking-scheduler-deploy", tasks)
        self.assertIn("visudo -cf %s", tasks)
        policy = self.read("roles/security/templates/99-booking-scheduler-deploy.j2")
        self.assertIn("NOPASSWD", policy)
        self.assertIn("/usr/bin/python3", policy)
        self.assertNotIn("ALL=(ALL) NOPASSWD: ALL", policy)


if __name__ == "__main__":
    unittest.main()
