package bootstrap

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gucooing/bds/internal/config"
	"gucooing/bds/internal/logging"
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

	server, err := appserver.New(appserver.Options{
		Config:       serverConfig,
		Logger:       logger.Logger,
		ConsoleInput: os.Stdin,
		DataPath:     options.DataPath,
		Brand:        Name,
		Version:      CurrentVersion.String(),
	})
	if err != nil {
		return err
	}

	if err := server.Start(); err != nil {
		return err
	}
	defer server.Stop()

	logger.Info("mcpe listener started", "address", server.Addr(), "online_mode", serverConfig.OnlineMode)
	logger.Info("runtime waiting for shutdown", "max_players", serverConfig.MaxPlayers, "view_distance", serverConfig.ViewDistance)
	select {
	case <-ctx.Done():
	case <-server.Done():
	}
	server.Stop()
	server.Wait()
	logger.Info("shutdown requested", "reason", ctx.Err())
	return nil
}
