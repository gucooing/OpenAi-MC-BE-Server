package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const DefaultFileName = "server.properties"

type Config struct {
	ServerName   string
	Address      string
	Port         int
	MaxPlayers   int
	ViewDistance int
	OnlineMode   bool
	GameMode     string
	LogLevel     string
	LogFile      string
	ColorLogs    bool
}

var DefaultConfig = Config{
	ServerName:   "BetterAltay-Go Server",
	Address:      "0.0.0.0",
	Port:         19132,
	MaxPlayers:   20,
	ViewDistance: 8,
	OnlineMode:   false,
	GameMode:     "Survival",
	LogLevel:     "info",
	LogFile:      "logs/server.log",
	ColorLogs:    true,
}

func Load(path string) (Config, bool, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Config{}, false, fmt.Errorf("create config directory: %w", err)
	}

	if _, err := os.Stat(path); err != nil {
		if !os.IsNotExist(err) {
			return Config{}, false, fmt.Errorf("stat config %q: %w", path, err)
		}
		if err := os.WriteFile(path, []byte(DefaultConfig.MarshalProperties()), 0o644); err != nil {
			return Config{}, false, fmt.Errorf("write default config %q: %w", path, err)
		}
		return DefaultConfig, true, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return Config{}, false, fmt.Errorf("open config %q: %w", path, err)
	}
	defer file.Close()

	cfg := DefaultConfig
	scanner := bufio.NewScanner(file)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, found := strings.Cut(line, "=")
		if !found {
			return Config{}, false, fmt.Errorf("parse config %q line %d: expected key=value", path, lineNumber)
		}
		if err := cfg.apply(strings.TrimSpace(key), strings.TrimSpace(value)); err != nil {
			return Config{}, false, fmt.Errorf("parse config %q line %d: %w", path, lineNumber, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return Config{}, false, fmt.Errorf("read config %q: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, false, fmt.Errorf("validate config %q: %w", path, err)
	}

	return cfg, false, nil
}

func (cfg Config) MarshalProperties() string {
	var builder strings.Builder
	builder.WriteString("# BetterAltay-Go server configuration\n")
	builder.WriteString("server-name=" + cfg.ServerName + "\n")
	builder.WriteString("server-address=" + cfg.Address + "\n")
	builder.WriteString(fmt.Sprintf("server-port=%d\n", cfg.Port))
	builder.WriteString(fmt.Sprintf("max-players=%d\n", cfg.MaxPlayers))
	builder.WriteString(fmt.Sprintf("view-distance=%d\n", cfg.ViewDistance))
	builder.WriteString(fmt.Sprintf("online-mode=%t\n", cfg.OnlineMode))
	builder.WriteString("gamemode=" + cfg.GameMode + "\n")
	builder.WriteString("log-level=" + cfg.LogLevel + "\n")
	builder.WriteString("log-file=" + cfg.LogFile + "\n")
	builder.WriteString(fmt.Sprintf("color-logs=%t\n", cfg.ColorLogs))
	return builder.String()
}

func (cfg Config) Validate() error {
	if cfg.ServerName == "" {
		return fmt.Errorf("server-name cannot be empty")
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return fmt.Errorf("server-port must be between 1 and 65535")
	}
	if cfg.MaxPlayers < 1 {
		return fmt.Errorf("max-players must be at least 1")
	}
	if cfg.ViewDistance < 2 {
		return fmt.Errorf("view-distance must be at least 2")
	}
	if cfg.GameMode == "" {
		return fmt.Errorf("gamemode cannot be empty")
	}
	return nil
}

func (cfg *Config) apply(key string, value string) error {
	switch key {
	case "server-name":
		cfg.ServerName = value
	case "server-address":
		cfg.Address = value
	case "server-port":
		port, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("server-port must be an integer: %w", err)
		}
		cfg.Port = port
	case "max-players":
		maxPlayers, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("max-players must be an integer: %w", err)
		}
		cfg.MaxPlayers = maxPlayers
	case "view-distance":
		viewDistance, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("view-distance must be an integer: %w", err)
		}
		cfg.ViewDistance = viewDistance
	case "online-mode":
		onlineMode, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("online-mode must be true or false: %w", err)
		}
		cfg.OnlineMode = onlineMode
	case "gamemode":
		cfg.GameMode = value
	case "log-level":
		cfg.LogLevel = value
	case "log-file":
		cfg.LogFile = value
	case "color-logs":
		colorLogs, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("color-logs must be true or false: %w", err)
		}
		cfg.ColorLogs = colorLogs
	default:
		return fmt.Errorf("unknown key %q", key)
	}
	return nil
}
