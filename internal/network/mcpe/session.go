package mcpe

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"

	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

type PacketConn interface {
	WritePacket(packet.Packet) error
	WritePacketUncompressed(packet.Packet) error
	EnableCompression()
	EnableEncryption([32]byte)
	CompressionThreshold() int
	CompressionAlgorithm() uint16
	Flush() error
	RemoteAddr() net.Addr
}

type PacketClient interface {
	HandlePacket(context.Context, packet.Packet) error
	State() int
}

type ClientFactory func(PacketConn) PacketClient

type DisconnectAware interface {
	OnDisconnect(context.Context)
}

type session struct {
	conn       net.Conn
	codec      codec
	logger     *slog.Logger
	client     PacketClient
	compressed bool
	writeMu    sync.Mutex
}

func newSession(newClient ClientFactory, logger *slog.Logger, conn net.Conn) *session {
	session := &session{
		conn:   conn,
		codec:  newCodec(),
		logger: logger,
	}
	session.client = newClient(session)
	return session
}

func (session *session) Serve(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = session.conn.Close()
		case <-done:
		}
	}()
	defer close(done)
	defer func() {
		if client, ok := session.client.(DisconnectAware); ok {
			client.OnDisconnect(ctx)
		}
	}()

	buffer := make([]byte, maxDecompressedBatchBytes)
	for {
		n, err := session.conn.Read(buffer)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("read mcpe batch: %w", err)
		}
		if n == 0 {
			continue
		}
		if err := session.handleBatch(ctx, buffer[:n]); err != nil {
			return err
		}
	}
}

func (session *session) handleBatch(ctx context.Context, data []byte) error {
	packets, err := session.decodeBatch(data)
	if err != nil {
		return fmt.Errorf("decode mcpe batch: %w", err)
	}
	for _, payload := range packets {
		pk, err := session.codec.DecodePacket(payload, fromClient)
		if err != nil {
			return fmt.Errorf("decode mcpe packet: %w", err)
		}
		if session.logger != nil && pk.ID() != packet.IDPlayerAuthInput {
			session.logger.Debug("mcpe packet received", "remote", session.conn.RemoteAddr(), "packet_id", pk.ID(), "packet", fmt.Sprintf("%T", pk), "state", session.client.State())
		}
		if err := session.client.HandlePacket(ctx, pk); err != nil {
			return err
		}
	}
	return nil
}

func (session *session) decodeBatch(data []byte) ([][]byte, error) {
	codec := session.codec
	if !session.compressed {
		codec.Compression = nil
	}
	return codec.DecodeBatch(data)
}

func (session *session) WritePacket(pk packet.Packet) error {
	return session.writePacket(pk, session.compressed)
}

func (session *session) WritePacketUncompressed(pk packet.Packet) error {
	return session.writePacket(pk, false)
}

func (session *session) EnableCompression() {
	session.compressed = true
}

func (session *session) EnableEncryption(key [32]byte) {
	session.codec.EnableEncryption(key)
}

func (session *session) CompressionThreshold() int {
	return session.codec.CompressionThreshold
}

func (session *session) CompressionAlgorithm() uint16 {
	return session.codec.Compression.EncodeCompression()
}

func (session *session) Flush() error {
	return nil
}

func (session *session) RemoteAddr() net.Addr {
	return session.conn.RemoteAddr()
}

func (session *session) writePacket(pk packet.Packet, compressed bool) error {
	session.writeMu.Lock()
	defer session.writeMu.Unlock()

	payload, err := session.codec.EncodePacket(pk)
	if err != nil {
		return err
	}
	codec := session.codec
	if !compressed {
		codec.Compression = nil
	}
	batch, err := codec.EncodeBatch([][]byte{payload})
	if err != nil {
		return err
	}

	_, err = session.conn.Write(batch)
	return err
}
