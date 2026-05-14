package raknet

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"

	raknetlib "github.com/sandertv/go-raknet"
)

type SessionHandler func(context.Context, net.Conn) error

type Options struct {
	Address        string
	PongInfo       PongInfo
	Logger         *slog.Logger
	SessionHandler SessionHandler
}

type Server struct {
	listener *raknetlib.Listener
	logger   *slog.Logger
	handler  SessionHandler
	mu       sync.Mutex
	closed   bool
	sessions map[*raknetlib.Conn]struct{}
	wg       sync.WaitGroup
	done     chan struct{}
	ctx      context.Context
	cancel   context.CancelFunc
	pongInfo PongInfo
}

func Listen(options Options) (*Server, error) {
	if options.Address == "" {
		return nil, fmt.Errorf("raknet listen address cannot be empty")
	}

	listener, err := raknetlib.ListenConfig{ErrorLog: options.Logger}.Listen(options.Address)
	if err != nil {
		return nil, err
	}

	server := &Server{
		listener: listener,
		logger:   options.Logger,
		handler:  options.SessionHandler,
		sessions: make(map[*raknetlib.Conn]struct{}),
		done:     make(chan struct{}),
	}
	server.ctx, server.cancel = context.WithCancel(context.Background())
	server.SetPongInfo(options.PongInfo)
	go server.acceptLoop()
	return server, nil
}

func (server *Server) Addr() net.Addr {
	return server.listener.Addr()
}

func (server *Server) Close() error {
	server.mu.Lock()
	if server.closed {
		server.mu.Unlock()
		<-server.done
		return nil
	}
	server.closed = true
	sessions := make([]*raknetlib.Conn, 0, len(server.sessions))
	for conn := range server.sessions {
		sessions = append(sessions, conn)
	}
	server.mu.Unlock()

	server.cancel()
	err := server.listener.Close()
	for _, conn := range sessions {
		_ = conn.Close()
	}
	server.wg.Wait()
	<-server.done
	return err
}

func (server *Server) SetPongInfo(info PongInfo) {
	server.mu.Lock()
	defer server.mu.Unlock()

	if info.ServerID == 0 {
		info.ServerID = server.listener.ID()
	}
	server.pongInfo = info
	server.listener.PongData(info.Data())
}

func (server *Server) acceptLoop() {
	defer close(server.done)
	for {
		conn, err := server.listener.Accept()
		if err != nil {
			if errors.Is(err, raknetlib.ErrListenerClosed) {
				return
			}
			if server.logger != nil {
				server.logger.Warn("raknet accept failed", "error", err)
			}
			continue
		}
		raknetConn, ok := conn.(*raknetlib.Conn)
		if !ok {
			_ = conn.Close()
			continue
		}
		if !server.register(raknetConn) {
			_ = raknetConn.Close()
			continue
		}
		go server.serveSession(raknetConn)
	}
}

func (server *Server) register(conn *raknetlib.Conn) bool {
	server.mu.Lock()
	defer server.mu.Unlock()

	if server.closed {
		return false
	}
	server.sessions[conn] = struct{}{}
	server.wg.Add(1)
	return true
}

func (server *Server) unregister(conn *raknetlib.Conn) {
	server.mu.Lock()
	delete(server.sessions, conn)
	server.mu.Unlock()
	server.wg.Done()
}

func (server *Server) serveSession(conn *raknetlib.Conn) {
	defer server.unregister(conn)
	defer conn.Close()
	if server.logger != nil {
		server.logger.Info("raknet connection accepted", "remote", conn.RemoteAddr())
	}

	sessionCtx, cancel := context.WithCancel(server.ctx)
	defer cancel()

	go func() {
		select {
		case <-conn.Context().Done():
			cancel()
		case <-sessionCtx.Done():
		}
	}()

	if server.handler != nil {
		if err := server.handler(sessionCtx, conn); err != nil && server.logger != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, net.ErrClosed) {
			server.logger.Warn("raknet session handler failed", "remote", conn.RemoteAddr(), "error", err)
		}
		return
	}

	buffer := make([]byte, 1500)
	for {
		select {
		case <-sessionCtx.Done():
			return
		default:
		}
		n, err := conn.Read(buffer)
		if err != nil {
			if server.logger != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, context.Canceled) {
				server.logger.Debug("raknet connection closed", "remote", conn.RemoteAddr(), "error", err)
			}
			return
		}
		if server.logger != nil {
			server.logger.Debug("raknet payload received", "remote", conn.RemoteAddr(), "bytes", n)
		}
	}
}
