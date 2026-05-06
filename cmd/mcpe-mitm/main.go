package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	raknet "github.com/sandertv/go-raknet"
)

type options struct {
	listen   string
	upstream string
	output   string
}

type event struct {
	Time      string `json:"time"`
	Session   uint64 `json:"session,omitempty"`
	Phase     string `json:"phase"`
	Direction string `json:"direction,omitempty"`
	Bytes     int    `json:"bytes,omitempty"`
	Src       string `json:"src,omitempty"`
	Dst       string `json:"dst,omitempty"`
	Data      any    `json:"data,omitempty"`
	Message   string `json:"message,omitempty"`
	Error     string `json:"error,omitempty"`
}

type streamData struct {
	Payload []byte `json:"payload"`
}

type eventLogger struct {
	mu  sync.Mutex
	enc *json.Encoder
}

var nextSessionID atomic.Uint64

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	opts := parseFlags()
	if opts.upstream == "" {
		fmt.Fprintln(os.Stderr, "-upstream is required, for example 127.0.0.1:19132")
		os.Exit(2)
	}

	out, closeOut, err := openOutput(opts.output)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer closeOut()
	logger := &eventLogger{enc: json.NewEncoder(out)}

	listener, err := raknet.ListenConfig{}.Listen(opts.listen)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer listener.Close()

	if pong, err := pingUpstream(ctx, opts.upstream); err == nil {
		listener.PongData(pong)
	} else {
		logger.write(event{Phase: "event", Message: "upstream ping failed; server-list pong will be empty", Error: err.Error()})
	}

	logger.write(event{
		Phase:   "event",
		Message: "transparent raknet mitm listening",
		Data: map[string]any{
			"listen":   opts.listen,
			"upstream": opts.upstream,
		},
	})
	fmt.Fprintf(os.Stderr, "mcpe-mitm transparent relay listening on %s, forwarding to %s, writing %s\n", opts.listen, opts.upstream, opts.output)

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, raknet.ErrListenerClosed) || errors.Is(err, net.ErrClosed) {
				return
			}
			logger.write(event{Phase: "event", Message: "accept failed", Error: err.Error()})
			continue
		}
		go handleConn(ctx, conn, opts, logger)
	}
}

func parseFlags() options {
	var opts options
	flags := flag.NewFlagSet("mcpe-mitm", flag.ExitOnError)
	flags.StringVar(&opts.listen, "listen", "0.0.0.0:19133", "local RakNet address for the Bedrock client")
	flags.StringVar(&opts.upstream, "upstream", "127.0.0.1:19132", "upstream Bedrock server address")
	flags.StringVar(&opts.output, "out", "logs/mcpe-mitm.ndjson", "NDJSON stream log path, or - for stdout")
	flags.Parse(os.Args[1:])
	return opts
}

func openOutput(path string) (io.Writer, func(), error) {
	if path == "-" {
		return os.Stdout, func() {}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, nil, fmt.Errorf("create output directory: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, nil, fmt.Errorf("create output log: %w", err)
	}
	return f, func() { _ = f.Close() }, nil
}

func pingUpstream(ctx context.Context, address string) ([]byte, error) {
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return raknet.PingContext(pingCtx, address)
}

func handleConn(ctx context.Context, client net.Conn, opts options, logger *eventLogger) {
	session := nextSessionID.Add(1)
	defer client.Close()

	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	upstream, err := raknet.DialContext(dialCtx, opts.upstream)
	cancel()
	if err != nil {
		logger.write(event{Session: session, Phase: "event", Message: "upstream dial failed", Src: addrString(client.RemoteAddr()), Error: err.Error()})
		return
	}
	defer upstream.Close()

	logger.write(event{
		Session: session,
		Phase:   "event",
		Message: "relay connected",
		Src:     addrString(client.RemoteAddr()),
		Dst:     addrString(upstream.RemoteAddr()),
	})

	done := make(chan error, 2)
	go relayStream(ctx, session, "client->upstream", client, upstream, logger, done)
	go relayStream(ctx, session, "upstream->client", upstream, client, logger, done)

	err = <-done
	if err != nil && ctx.Err() == nil && !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
		logger.write(event{Session: session, Phase: "event", Message: "relay closed", Error: err.Error()})
	} else {
		logger.write(event{Session: session, Phase: "event", Message: "relay closed"})
	}
}

func relayStream(ctx context.Context, session uint64, direction string, src, dst net.Conn, logger *eventLogger, done chan<- error) {
	buffer := make([]byte, 2*1024*1024)
	for {
		select {
		case <-ctx.Done():
			done <- ctx.Err()
			return
		default:
		}

		n, err := src.Read(buffer)
		if n > 0 {
			payload := append([]byte(nil), buffer[:n]...)
			logger.write(event{
				Session:   session,
				Phase:     "stream",
				Direction: direction,
				Bytes:     len(payload),
				Src:       addrString(src.RemoteAddr()),
				Dst:       addrString(dst.RemoteAddr()),
				Data:      streamData{Payload: payload},
			})
			if err := writeFull(dst, payload); err != nil {
				done <- err
				return
			}
		}
		if err != nil {
			done <- err
			return
		}
	}
}

func writeFull(conn net.Conn, data []byte) error {
	for len(data) > 0 {
		n, err := conn.Write(data)
		if err != nil {
			return err
		}
		data = data[n:]
	}
	return nil
}

func (logger *eventLogger) write(ev event) {
	ev.Time = time.Now().Format(time.RFC3339Nano)
	logger.mu.Lock()
	defer logger.mu.Unlock()
	_ = logger.enc.Encode(ev)
}

func addrString(addr net.Addr) string {
	if addr == nil {
		return ""
	}
	return addr.String()
}
