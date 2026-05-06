package bootstrap

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gucooing/bds/internal/config"
	"gucooing/bds/internal/logging"
	networkmcpe "gucooing/bds/internal/network/mcpe"
	appserver "gucooing/bds/internal/server"
)

type Options struct {
	ShowVersion bool
	DataPath    string
	ConfigPath  string
	CheckOnly   bool
}

func RunContext(ctx context.Context, stdout, stderr io.Writer, args []string) error {
	options := Options{
		DataPath:   ".",
		ConfigPath: config.DefaultFileName,
	}

	flags := flag.NewFlagSet("bds", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.BoolVar(&options.ShowVersion, "version", false, "print version information and exit")
	flags.StringVar(&options.DataPath, "data-path", options.DataPath, "server data directory")
	flags.StringVar(&options.ConfigPath, "config", options.ConfigPath, "server configuration file")
	flags.BoolVar(&options.CheckOnly, "check", false, "load configuration and exit")
	if err := flags.Parse(args); err != nil {
		return err
	}

	fmt.Fprintln(stdout, CurrentVersion.String())
	if options.ShowVersion {
		return nil
	}

	configPath := options.ConfigPath
	if !filepath.IsAbs(configPath) {
		configPath = filepath.Join(options.DataPath, configPath)
	}

	serverConfig, createdConfig, err := config.Load(configPath)
	if err != nil {
		return err
	}

	logFile := serverConfig.LogFile
	if logFile != "" && !filepath.IsAbs(logFile) {
		logFile = filepath.Join(options.DataPath, logFile)
	}
	logger, err := logging.New(stdout, logging.Options{
		Level:    serverConfig.LogLevel,
		Color:    serverConfig.ColorLogs,
		FilePath: logFile,
	})
	if err != nil {
		return err
	}
	defer logger.Close()

	logger.Info("configuration loaded", "path", configPath, "created", createdConfig)
	logger.Info("server bootstrap complete", "name", serverConfig.ServerName, "address", serverConfig.Address, "port", serverConfig.Port)
	if options.CheckOnly {
		logger.Info("configuration check complete")
		return nil
	}
	if ctx.Err() != nil {
		logger.Info("shutdown requested", "reason", ctx.Err())
		return nil
	}

	runtimeCtx, shutdown := context.WithCancel(ctx)
	defer shutdown()

	listenAddress := net.JoinHostPort(serverConfig.Address, strconv.Itoa(serverConfig.Port))
	mcpeHandler, err := appserver.NewMCPEHandler(appserver.MCPEOptions{
		ServerName:   serverConfig.ServerName,
		ServerBrand:  Name,
		GameMode:     serverConfig.GameMode,
		MaxPlayers:   serverConfig.MaxPlayers,
		ViewDistance: serverConfig.ViewDistance,
		OnlineMode:   serverConfig.OnlineMode,
		Logger:       logger.Logger,
		Shutdown:     shutdown,
	})
	if err != nil {
		return err
	}
	mcpeServer, err := networkmcpe.Listen(networkmcpe.Options{
		Address:     listenAddress,
		ServerName:  serverConfig.ServerName,
		ServerBrand: Name,
		GameMode:    serverConfig.GameMode,
		MaxPlayers:  serverConfig.MaxPlayers,
		Logger:      logger.Logger,
		NewClient: func(conn networkmcpe.PacketConn) networkmcpe.PacketClient {
			return appserver.NewMCPEClient(mcpeHandler, conn)
		},
	})
	if err != nil {
		return err
	}
	defer mcpeServer.Close()

	logger.Info("mcpe listener started", "address", mcpeServer.Addr(), "online_mode", serverConfig.OnlineMode)
	go runConsoleInput(runtimeCtx, os.Stdin, logger, mcpeHandler)
	logger.Info("runtime waiting for shutdown", "max_players", serverConfig.MaxPlayers, "view_distance", serverConfig.ViewDistance)
	<-runtimeCtx.Done()
	logger.Info("shutdown requested", "reason", runtimeCtx.Err())
	return nil
}

func runConsoleInput(ctx context.Context, input io.Reader, logger *logging.Logger, handler *appserver.MCPEHandler) {
	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
		result := handler.ExecuteConsoleCommand(ctx, line)
		for _, message := range result.Messages {
			if result.Success {
				logger.Info(message)
			} else {
				logger.Warn(message)
			}
		}
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		logger.Warn("console input stopped", "error", err)
	}
}
