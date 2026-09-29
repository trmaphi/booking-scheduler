package config_test

import (
	"strings"
	"testing"

	"scheduler/api/internal/config"
)

func TestLoadAcceptsLocalConfiguration(t *testing.T) {
	values := map[string]string{
		"DATABASE_URL":   "postgres://scheduler:local@database:5432/scheduler",
		"ALLOWED_ORIGIN": "http://localhost:3000",
	}
	got, err := config.Load(func(key string) string { return values[key] })
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.ListenAddress != ":8080" || got.DatabaseURL != values["DATABASE_URL"] || got.AllowedOrigin != values["ALLOWED_ORIGIN"] {
		t.Errorf("Load() = %+v, want default listen address and supplied connection settings", got)
	}

	values["LISTEN_ADDRESS"] = "127.0.0.1:9000"
	got, err = config.Load(func(key string) string { return values[key] })
	if err != nil || got.ListenAddress != "127.0.0.1:9000" {
		t.Errorf("Load() with explicit listen address = %+v, %v", got, err)
	}
}

func TestLoadReportsMissingVariableNames(t *testing.T) {
	_, err := config.Load(func(string) string { return "" })
	if err == nil {
		t.Fatal("Load() error = nil, want required variables named")
	}
	for _, key := range []string{"ALLOWED_ORIGIN", "DATABASE_URL"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("Load() error = %q, want %s", err, key)
		}
	}
	if strings.Index(err.Error(), "ALLOWED_ORIGIN") > strings.Index(err.Error(), "DATABASE_URL") {
		t.Errorf("Load() error = %q, want keys in alphabetical order", err)
	}
}

func TestLoadDoesNotLeakValues(t *testing.T) {
	values := map[string]string{
		"DATABASE_URL":   "mysql://user:secret-password@database/scheduler",
		"ALLOWED_ORIGIN": "http://localhost:3000/private-secret",
	}
	_, err := config.Load(func(key string) string { return values[key] })
	if err == nil {
		t.Fatal("Load() error = nil, want invalid URL and origin")
	}
	for _, key := range []string{"ALLOWED_ORIGIN", "DATABASE_URL"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("Load() error = %q, want %s", err, key)
		}
	}
	for _, secret := range []string{"secret-password", "private-secret", "mysql://", "localhost:3000"} {
		if strings.Contains(err.Error(), secret) {
			t.Errorf("Load() error = %q, leaked configuration value", err)
		}
	}
}

func TestLoadRejectsEmptyQueryOrigin(t *testing.T) {
	values := map[string]string{
		"DATABASE_URL":   "postgres://scheduler:local@database:5432/scheduler",
		"ALLOWED_ORIGIN": "http://localhost:3000?",
	}
	_, err := config.Load(func(key string) string { return values[key] })
	if err == nil || !strings.Contains(err.Error(), "ALLOWED_ORIGIN") {
		t.Fatalf("Load() error = %v, want invalid ALLOWED_ORIGIN", err)
	}
	if strings.Contains(err.Error(), values["ALLOWED_ORIGIN"]) {
		t.Errorf("Load() error = %q, leaked origin", err)
	}
}

func TestLoadRejectsInvalidListenAddressWithoutLeakingValue(t *testing.T) {
	values := map[string]string{
		"LISTEN_ADDRESS": "localhost:private-port",
		"DATABASE_URL":   "postgres://scheduler:local@database:5432/scheduler",
		"ALLOWED_ORIGIN": "http://localhost:3000",
	}
	_, err := config.Load(func(key string) string { return values[key] })
	if err == nil || !strings.Contains(err.Error(), "LISTEN_ADDRESS") {
		t.Fatalf("Load() error = %v, want invalid LISTEN_ADDRESS", err)
	}
	if strings.Contains(err.Error(), values["LISTEN_ADDRESS"]) || strings.Contains(err.Error(), "private-port") {
		t.Errorf("Load() error = %q, leaked listen address", err)
	}
}

func TestLoadAcceptsSupportedListenAddresses(t *testing.T) {
	for _, listenAddress := range []string{":8080", "localhost:8080", "127.0.0.1:8080", "[::1]:8080"} {
		t.Run(listenAddress, func(t *testing.T) {
			values := map[string]string{
				"LISTEN_ADDRESS": listenAddress,
				"DATABASE_URL":   "postgres://scheduler:local@database:5432/scheduler",
				"ALLOWED_ORIGIN": "http://localhost:3000",
			}
			if _, err := config.Load(func(key string) string { return values[key] }); err != nil {
				t.Fatalf("Load() error = %v, want valid listen address", err)
			}
		})
	}
}
