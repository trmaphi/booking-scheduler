package config

import (
	"errors"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

type Config struct {
	ListenAddress string
	DatabaseURL   string
	AllowedOrigin string
}

func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		ListenAddress: strings.TrimSpace(getenv("LISTEN_ADDRESS")),
		DatabaseURL:   strings.TrimSpace(getenv("DATABASE_URL")),
		AllowedOrigin: strings.TrimSpace(getenv("ALLOWED_ORIGIN")),
	}
	if cfg.ListenAddress == "" {
		cfg.ListenAddress = ":8080"
	}

	var invalid []string
	if !validListenAddress(cfg.ListenAddress) {
		invalid = append(invalid, "LISTEN_ADDRESS")
	}
	if !validDatabaseURL(cfg.DatabaseURL) {
		invalid = append(invalid, "DATABASE_URL")
	}
	if !validOrigin(cfg.AllowedOrigin) {
		invalid = append(invalid, "ALLOWED_ORIGIN")
	}
	if len(invalid) > 0 {
		sort.Strings(invalid)
		return Config{}, errors.New("invalid configuration: " + strings.Join(invalid, ", "))
	}
	return cfg, nil
}

func validListenAddress(raw string) bool {
	_, port, err := net.SplitHostPort(raw)
	if err != nil {
		return false
	}
	portNumber, err := strconv.Atoi(port)
	return err == nil && portNumber >= 1 && portNumber <= 65535
}

func validDatabaseURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "postgres" || u.Scheme == "postgresql") && u.Host != "" && u.Opaque == ""
}

func validOrigin(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.Opaque == "" && u.User == nil && u.Path == "" && u.RawPath == "" && u.RawQuery == "" && !u.ForceQuery && u.Fragment == "" && u.String() == raw
}
