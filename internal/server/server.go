package server

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	gtprotocol "github.com/sandertv/gophertunnel/minecraft/protocol"
	"gucooing/bds/internal/command"
	"gucooing/bds/internal/config"
	networkmcpe "gucooing/bds/internal/network/mcpe"
	appworld "gucooing/bds/internal/world"
)

const (
	tickRate     = 20
	tickInterval = time.Second / tickRate
	taskQueueCap = 128
)

type Options struct {
	Config       config.Config
	Logger       *slog.Logger
	ConsoleInput io.Reader
	DataPath     string
	Brand        string
	Version      string
	World        appworld.ChunkProvider
	PlayerStore  PlayerStore
}

type Server struct {
	config       config.Config
	logger       *slog.Logger
	consoleInput io.Reader
	dataPath     string
	brand        string
	version      string
	playerStore  PlayerStore

	ctx       context.Context
	cancel    context.CancelFunc
	startOnce sync.Once
	stopOnce  sync.Once
	done      chan struct{}

	mcpeHandler *MCPEHandler
	mcpeServer  *networkmcpe.Server

	taskQueue chan func(context.Context)

	statsMu      sync.RWMutex
	startedAt    time.Time
	tick         uint64
	lastTickTime time.Duration
	tps          float64
}

type Stats struct {
	StartedAt    time.Time
	Uptime       time.Duration
	Tick         uint64
	LastTickTime time.Duration
	TPS          float64
	Online       int
	MaxPlayers   int
	Goroutines   int
	MemoryAlloc  uint64
	HeapInUse    uint64
	TaskQueueLen int
	TaskQueueCap int
}

type Info struct {
	ServerName       string
	Brand            string
	ProtocolVersion  int
	MinecraftVersion string
	GameMode         string
	OnlinePlayers    int
	MaxPlayers       int
	Address          net.Addr
}

type PlayerSnapshot struct {
	UUID      string
	Name      string
	XUID      string
	RuntimeID uint64
	Position  appworld.Vec3
	Velocity  appworld.Vec3
	Pitch     float32
	Yaw       float32
	HeadYaw   float32
	LastSeen  time.Time
}

type PlayerStore interface {
	SavePlayer(context.Context, PlayerSnapshot) error
}

func New(options Options) (*Server, error) {
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	if options.Brand == "" {
		options.Brand = "BetterAltay-Go"
	}
	if options.Version == "" {
		options.Version = options.Brand
	}
	if err := options.Config.Validate(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	server := &Server{
		config:       options.Config,
		logger:       options.Logger,
		consoleInput: options.ConsoleInput,
		dataPath:     options.DataPath,
		brand:        options.Brand,
		version:      options.Version,
		playerStore:  options.PlayerStore,
		ctx:          ctx,
		cancel:       cancel,
		done:         make(chan struct{}),
		taskQueue:    make(chan func(context.Context), taskQueueCap),
		tps:          tickRate,
	}

	handler, err := NewMCPEHandler(MCPEOptions{
		ServerName:   options.Config.ServerName,
		ServerBrand:  options.Brand,
		GameMode:     options.Config.GameMode,
		MaxPlayers:   options.Config.MaxPlayers,
		ViewDistance: options.Config.ViewDistance,
		OnlineMode:   options.Config.OnlineMode,
		Logger:       options.Logger,
		World:        options.World,
		Shutdown:     server.Stop,
		PlayerJoined: func(*MCPEClient) {
			server.updateMOTD()
		},
		PlayerLeft: func(client *MCPEClient) {
			server.savePlayer(client)
			server.updateMOTD()
		},
	})
	if err != nil {
		cancel()
		return nil, err
	}
	server.mcpeHandler = handler
	return server, nil
}

func (server *Server) Start() error {
	var err error
	server.startOnce.Do(func() {
		server.startedAt = time.Now()
		err = server.startNetwork()
		if err != nil {
			server.cancel()
			close(server.done)
			return
		}
		go server.run()
		if server.consoleInput != nil {
			go server.runConsoleInput(server.consoleInput)
		}
		server.logger.Info("server runtime started", "name", server.config.ServerName, "address", server.mcpeServer.Addr(), "max_players", server.config.MaxPlayers, "view_distance", server.config.ViewDistance)
	})
	return err
}

func (server *Server) Run(ctx context.Context) error {
	if err := server.Start(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		server.Stop()
	case <-server.Done():
	}
	server.Wait()
	return nil
}

func (server *Server) Stop() {
	server.stopOnce.Do(func() {
		server.cancel()
	})
}

func (server *Server) Done() <-chan struct{} {
	return server.done
}

func (server *Server) Wait() {
	<-server.done
}

func (server *Server) Submit(task func(context.Context)) error {
	if task == nil {
		return nil
	}
	select {
	case <-server.ctx.Done():
		return context.Canceled
	case server.taskQueue <- task:
		return nil
	default:
		return fmt.Errorf("server task queue is full")
	}
}

func (server *Server) ExecuteConsoleCommand(ctx context.Context, line string) command.Result {
	return server.mcpeHandler.ExecuteConsoleCommand(ctx, line)
}

func (server *Server) Stats() Stats {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	server.statsMu.RLock()
	stats := Stats{
		StartedAt:    server.startedAt,
		Tick:         server.tick,
		LastTickTime: server.lastTickTime,
		TPS:          server.tps,
		Online:       server.OnlinePlayers(),
		MaxPlayers:   server.config.MaxPlayers,
		Goroutines:   runtime.NumGoroutine(),
		MemoryAlloc:  mem.Alloc,
		HeapInUse:    mem.HeapInuse,
		TaskQueueLen: len(server.taskQueue),
		TaskQueueCap: cap(server.taskQueue),
	}
	server.statsMu.RUnlock()
	if !stats.StartedAt.IsZero() {
		stats.Uptime = time.Since(stats.StartedAt)
	}
	return stats
}

func (server *Server) Info() Info {
	return Info{
		ServerName:       server.config.ServerName,
		Brand:            server.brand,
		ProtocolVersion:  gtprotocol.CurrentProtocol,
		MinecraftVersion: gtprotocol.CurrentVersion,
		GameMode:         server.config.GameMode,
		OnlinePlayers:    server.OnlinePlayers(),
		MaxPlayers:       server.config.MaxPlayers,
		Address:          server.Addr(),
	}
}

func (server *Server) OnlinePlayers() int {
	return len(server.mcpeHandler.allPlayers())
}

func (server *Server) Handler() *MCPEHandler {
	return server.mcpeHandler
}

func (server *Server) Addr() net.Addr {
	if server.mcpeServer == nil {
		return nil
	}
	return server.mcpeServer.Addr()
}

func (server *Server) startNetwork() error {
	listenAddress := net.JoinHostPort(server.config.Address, strconv.Itoa(server.config.Port))
	mcpeServer, err := networkmcpe.Listen(networkmcpe.Options{
		Address:     listenAddress,
		ServerName:  server.config.ServerName,
		ServerBrand: server.brand,
		GameMode:    server.config.GameMode,
		MaxPlayers:  server.config.MaxPlayers,
		Logger:      server.logger,
		OnlineCount: server.OnlinePlayers,
		NewClient: func(conn networkmcpe.PacketConn) networkmcpe.PacketClient {
			return NewMCPEClient(server.mcpeHandler, conn)
		},
	})
	if err != nil {
		return err
	}
	server.mcpeServer = mcpeServer
	return nil
}

func (server *Server) run() {
	defer close(server.done)
	defer server.logCrash()
	defer func() {
		if server.mcpeServer != nil {
			if err := server.mcpeServer.Close(); err != nil {
				server.logger.Warn("mcpe listener close failed", "error", err)
			}
		}
		stats := server.Stats()
		server.logger.Info("server runtime stopped", "uptime", stats.Uptime.String(), "ticks", stats.Tick, "online", stats.Online)
	}()

	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-server.ctx.Done():
			return
		case task := <-server.taskQueue:
			server.runTask(task)
		case <-ticker.C:
			server.tickOnce()
		}
	}
}

func (server *Server) tickOnce() {
	start := time.Now()
	server.drainTasks()
	elapsed := time.Since(start)

	server.statsMu.Lock()
	server.tick++
	server.lastTickTime = elapsed
	if elapsed <= 0 {
		server.tps = tickRate
	} else {
		instant := float64(time.Second) / float64(elapsed)
		if instant > tickRate {
			instant = tickRate
		}
		server.tps = (server.tps * 0.95) + (instant * 0.05)
	}
	server.statsMu.Unlock()
}

func (server *Server) drainTasks() {
	for {
		select {
		case task := <-server.taskQueue:
			server.runTask(task)
		default:
			return
		}
	}
}

func (server *Server) runTask(task func(context.Context)) {
	defer func() {
		if recovered := recover(); recovered != nil {
			server.logger.Error("server task panicked", "panic", recovered)
		}
	}()
	task(server.ctx)
}

func (server *Server) runConsoleInput(input io.Reader) {
	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		result := server.ExecuteConsoleCommand(server.ctx, line)
		for _, message := range result.Messages {
			if result.Success {
				server.logger.Info(message)
			} else {
				server.logger.Warn(message)
			}
		}
		select {
		case <-server.ctx.Done():
			return
		default:
		}
	}
	if err := scanner.Err(); err != nil && server.ctx.Err() == nil {
		server.logger.Warn("console input stopped", "error", err)
	}
}

func (server *Server) updateMOTD() {
	if server.mcpeServer != nil {
		server.mcpeServer.UpdateOnlinePlayers(server.OnlinePlayers())
	}
}

func (server *Server) savePlayer(client *MCPEClient) {
	if server.playerStore == nil || client.runtimeID == 0 {
		return
	}
	snapshot := PlayerSnapshot{
		UUID:      client.player.uuid.String(),
		Name:      client.login.Identity.DisplayName,
		XUID:      client.login.Identity.XUID,
		RuntimeID: client.runtimeID,
		Position:  vec3FromMGL(client.player.position),
		Velocity:  vec3FromMGL(client.player.velocity),
		Pitch:     client.player.pitch,
		Yaw:       client.player.yaw,
		HeadYaw:   client.player.headYaw,
		LastSeen:  time.Now(),
	}
	if err := server.playerStore.SavePlayer(context.Background(), snapshot); err != nil {
		server.logger.Warn("save player data failed", "name", snapshot.Name, "uuid", snapshot.UUID, "error", err)
	}
}

func vec3FromMGL(value interface {
	X() float32
	Y() float32
	Z() float32
}) appworld.Vec3 {
	return appworld.Vec3{
		X: value.X(),
		Y: value.Y(),
		Z: value.Z(),
	}
}

func (server *Server) logCrash() {
	if recovered := recover(); recovered != nil {
		stats := server.Stats()
		server.logger.Error(
			"server runtime panicked",
			"panic", recovered,
			"version", server.version,
			"uptime", stats.Uptime.String(),
			"ticks", stats.Tick,
			"online", stats.Online,
			"goroutines", stats.Goroutines,
		)
		server.cancel()
	}
}
