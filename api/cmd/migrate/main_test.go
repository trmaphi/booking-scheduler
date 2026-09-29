package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestMainReportsInvalidDatabaseURLWithoutLeakingIt(t *testing.T) {
	if os.Getenv("GO_MIGRATE_MAIN_HELPER") == "1" {
		main()
		return
	}

	const databaseURL = "postgres://user:entrypoint-secret@private-database:5432/private-name?connect_timeout=not-a-number"
	cmd := exec.Command(os.Args[0], "-test.run=^TestMainReportsInvalidDatabaseURLWithoutLeakingIt$")
	cmd.Env = append(os.Environ(),
		"GO_MIGRATE_MAIN_HELPER=1",
		"DATABASE_URL="+databaseURL,
		"ALLOWED_ORIGIN=http://localhost:3000",
		"LISTEN_ADDRESS=:8080",
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("migration entry point succeeded, want invalid DATABASE_URL failure")
	}
	message := string(output)
	if !strings.Contains(message, "DATABASE_URL") {
		t.Fatalf("migration entry point output = %q, want DATABASE_URL", message)
	}
	for _, sensitive := range []string{databaseURL, "entrypoint-secret", "private-database", "private-name", "connect_timeout"} {
		if strings.Contains(message, sensitive) {
			t.Errorf("migration entry point output = %q, leaked %q", message, sensitive)
		}
	}
}
