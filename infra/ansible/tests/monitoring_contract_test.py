from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[1]


class MonitoringContractTest(unittest.TestCase):
    def read(self, relative: str) -> str:
        return (ROOT / relative).read_text()

    def test_alloy_uses_signed_repository(self):
        tasks = self.read("roles/monitoring/tasks/main.yml")
        self.assertIn("https://apt.grafana.com/gpg-full.key", tasks)
        self.assertIn("signed-by=/etc/apt/keyrings/grafana.asc", tasks)
        self.assertIn("https://apt.grafana.com", tasks)

    def test_alloy_collects_only_approved_host_metrics_every_sixty_seconds(self):
        config = self.read("roles/monitoring/templates/config.alloy.j2")
        self.assertIn('prometheus.exporter.unix "host"', config)
        self.assertIn('set_collectors = ["cpu", "diskstats", "filesystem", "loadavg", "meminfo", "netdev"]', config)
        self.assertIn('scrape_interval = "60s"', config)
        self.assertIn('prometheus.remote_write "grafana_cloud"', config)
        self.assertRegex(config, r'instance\s*=\s*sys\.env\("MONITORING_INSTANCE"\)')
        self.assertRegex(config, r'environment\s*=\s*sys\.env\("MONITORING_ENVIRONMENT"\)')
        for forbidden in ("loki.", "tempo.", "otelcol.", "pyroscope."):
            self.assertNotIn(forbidden, config.lower())

    def test_alloy_credentials_are_root_only_and_suppressed(self):
        tasks = self.read("roles/monitoring/tasks/main.yml")
        self.assertRegex(tasks, r"(?s)Render protected Alloy credentials.*?mode: \"0600\".*?no_log: true")
        env = self.read("roles/monitoring/templates/alloy.env.j2")
        self.assertIn("GRAFANA_ACCESS_POLICY_TOKEN", env)
        self.assertNotIn("grafana_access_policy_token", self.read("roles/monitoring/templates/config.alloy.j2"))

    def test_alloy_configuration_is_validated_before_restart(self):
        tasks = self.read("roles/monitoring/tasks/main.yml")
        self.assertLess(tasks.index("Validate changed Alloy configuration"), tasks.index("Enable and start Alloy service"))
        self.assertRegex(tasks, r"(?s)- /usr/bin/alloy\s+- validate")
        self.assertIn("notify: Restart Alloy", tasks)

    def test_alloy_service_is_idempotently_enabled(self):
        tasks = self.read("roles/monitoring/tasks/main.yml")
        self.assertRegex(tasks, r"(?s)Enable and start Alloy service.*?state: started.*?enabled: true")
        handlers = self.read("roles/monitoring/handlers/main.yml")
        self.assertIn("state: restarted", handlers)


if __name__ == "__main__":
    unittest.main()
