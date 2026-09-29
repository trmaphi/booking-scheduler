from pathlib import Path
import re
import unittest


ROOT = Path(__file__).resolve().parents[1]


class BackupContractTest(unittest.TestCase):
    def read(self, relative: str) -> str:
        return (ROOT / relative).read_text()

    def test_failed_or_empty_dump_never_reaches_restic(self):
        script = self.read("roles/backup/templates/backup-postgres.sh.j2")
        self.assertIn("set -Eeuo pipefail", script)
        self.assertRegex(script, r"pg_dump[\s\S]*?test -s[\s\S]*?restic backup")
        self.assertLess(script.index("test -s"), script.index("restic backup"))

    def test_temporary_dump_is_removed_on_every_exit(self):
        script = self.read("roles/backup/templates/backup-postgres.sh.j2")
        self.assertRegex(script, r"trap ['\"]rm -rf .* EXIT")

    def test_credentials_are_root_only_and_suppressed(self):
        tasks = self.read("roles/backup/tasks/main.yml")
        self.assertRegex(tasks, r"(?s)Render protected restic environment.*?mode: \"0600\".*?no_log: true")

    def test_repository_init_retention_and_check_are_strict(self):
        script = self.read("roles/backup/templates/backup-postgres.sh.j2")
        self.assertIn("restic cat config", script)
        self.assertIn("restic init", script)
        self.assertIn("--keep-daily 7", script)
        self.assertIn("--keep-weekly 4", script)
        self.assertIn("--keep-monthly 6", script)
        self.assertIn("restic check", script)
        service = self.read("roles/backup/templates/booking-scheduler-backup.service.j2")
        self.assertIn("Type=oneshot", service)

    def test_timer_is_nightly_with_randomized_delay(self):
        timer = self.read("roles/backup/templates/booking-scheduler-backup.timer.j2")
        defaults = self.read("roles/backup/defaults/main.yml")
        self.assertIn('backup_on_calendar: "*-*-*', defaults)
        self.assertIn("RandomizedDelaySec=", timer)
        self.assertIn("Persistent=true", timer)

    def test_restore_requires_exact_snapshot_and_confirmation(self):
        restore = self.read("playbooks/restore.yml")
        self.assertIn("restore_snapshot_id", restore)
        self.assertIn("^[a-f0-9]{8,64}$", restore)
        self.assertIn("restore_snapshot_id != 'latest'", restore)
        self.assertIn("confirm_restore_cutover | bool", restore)

    def test_restore_verifies_disposable_database_before_tagged_cutover(self):
        restore = self.read("playbooks/restore.yml")
        self.assertLess(restore.index("Create disposable restore database"), restore.index("Probe required booking tables"))
        self.assertLess(restore.index("Probe required booking tables"), restore.index("Cut over verified restore candidate"))
        self.assertIn("pg_restore --list", restore)
        self.assertIn("public.appointments", restore)
        self.assertRegex(restore, r"(?s)Cut over verified restore candidate.*?tags: \[never, restore_cutover\]")


if __name__ == "__main__":
    unittest.main()
