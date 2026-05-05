package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadCreatesDefaultConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultFileName)

	cfg, created, err := Load(path)
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if !created {
		t.Fatalf("Load() created = false, want true")
	}
	if cfg.Port != 19132 {
		t.Fatalf("Port = %d, want 19132", cfg.Port)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() returned error: %v", err)
	}
	if !strings.Contains(string(content), "server-name=BetterAltay-Go Server") {
		t.Fatalf("default config %q does not contain server name", string(content))
	}
	if !strings.Contains(string(content), "log-file=logs/server.log") {
		t.Fatalf("default config %q does not contain log file", string(content))
	}
}

func TestLoadReadsProperties(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultFileName)
	content := strings.Join([]string{
		"server-name=Local Test",
		"server-address=127.0.0.1",
		"server-port=19133",
		"max-players=5",
		"view-distance=4",
		"online-mode=true",
		"log-level=debug",
		"log-file=",
		"color-logs=false",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() returned error: %v", err)
	}

	cfg, created, err := Load(path)
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if created {
		t.Fatalf("Load() created = true, want false")
	}
	if cfg.ServerName != "Local Test" || cfg.Address != "127.0.0.1" || cfg.Port != 19133 || !cfg.OnlineMode {
		t.Fatalf("Config = %+v, want values from properties", cfg)
	}
	if cfg.LogFile != "" || cfg.ColorLogs {
		t.Fatalf("Config = %+v, want log file disabled and color disabled", cfg)
	}
}

func TestLoadRejectsInvalidPort(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultFileName)
	if err := os.WriteFile(path, []byte("server-port=70000\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() returned error: %v", err)
	}

	_, _, err := Load(path)
	if err == nil {
		t.Fatalf("Load() error = nil, want invalid port error")
	}
	if !strings.Contains(err.Error(), "server-port") {
		t.Fatalf("Load() error = %v, want server-port context", err)
	}
}
