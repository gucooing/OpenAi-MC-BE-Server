package bootstrap

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunPrintsVersion(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := RunContext(context.Background(), &stdout, &stderr, []string{"-version"}); err != nil {
		t.Fatalf("RunContext() returned error: %v", err)
	}

	output := stdout.String()
	for _, want := range []string{"BetterAltay-Go", "protocol=944", "minecraft=1.26.10", "source=BetterAltay/3.28.0", "fork=1.39.3"} {
		if !strings.Contains(output, want) {
			t.Fatalf("version output %q does not contain %q", output, want)
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunCheckCreatesDefaultConfig(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	dataPath := t.TempDir()

	if err := RunContext(context.Background(), &stdout, &stderr, []string{"-data-path", dataPath, "-check"}); err != nil {
		t.Fatalf("RunContext() returned error: %v", err)
	}

	configPath := filepath.Join(dataPath, "server.properties")
	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("default config was not created: %v", err)
	}
	if !strings.Contains(string(configBytes), "server-port=19132") {
		t.Fatalf("default config %q does not contain server port", string(configBytes))
	}
	if !strings.Contains(stdout.String(), "configuration check complete") {
		t.Fatalf("startup output %q does not contain check completion", stdout.String())
	}
}

func TestRunWaitsForShutdown(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	dataPath := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := RunContext(ctx, &stdout, &stderr, []string{"-data-path", dataPath}); err != nil {
		t.Fatalf("RunContext() returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "shutdown requested") {
		t.Fatalf("startup output %q does not contain shutdown message", stdout.String())
	}
}
