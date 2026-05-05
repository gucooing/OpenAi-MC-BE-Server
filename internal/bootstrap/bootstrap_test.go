package bootstrap

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
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

func TestRunStartsMCPEListener(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	dataPath := t.TempDir()
	configPath := filepath.Join(dataPath, "server.properties")
	configContent := strings.Join([]string{
		"server-address=127.0.0.1",
		"server-port=" + strconv.Itoa(freeUDPPort(t)),
		"log-file=",
		"color-logs=false",
	}, "\n")
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("WriteFile() returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	if err := RunContext(ctx, &stdout, &stderr, []string{"-data-path", dataPath}); err != nil {
		t.Fatalf("RunContext() returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "mcpe listener started") {
		t.Fatalf("startup output %q does not contain MCPE listener message", stdout.String())
	}
}

func freeUDPPort(t *testing.T) int {
	t.Helper()

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket() returned error: %v", err)
	}
	defer conn.Close()

	_, portString, err := net.SplitHostPort(conn.LocalAddr().String())
	if err != nil {
		t.Fatalf("SplitHostPort() returned error: %v", err)
	}
	port, err := strconv.Atoi(portString)
	if err != nil {
		t.Fatalf("Atoi() returned error: %v", err)
	}
	return port
}
