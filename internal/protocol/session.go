package protocol

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"

	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func DebugSessionHandler(logger *slog.Logger) func(context.Context, net.Conn) error {
	return func(ctx context.Context, conn net.Conn) error {
		return ServeDebugSession(ctx, conn, logger)
	}
}

func ServeDebugSession(ctx context.Context, conn net.Conn, logger *slog.Logger) error {
	codec := NewCodec()
	compressed := false
	buffer := make([]byte, MaxDecompressedBatchBytes)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, err := conn.Read(buffer)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("read mcpe batch: %w", err)
		}
		if n == 0 {
			continue
		}

		packets, err := decodeSessionBatch(codec, buffer[:n], compressed)
		if err != nil {
			logDebug(logger, "mcpe batch decode failed", conn, "client->server", "error", err, "bytes", n, "compressed", compressed)
			return err
		}
		for _, data := range packets {
			pk, err := codec.DecodePacket(data, FromClient)
			if err != nil {
				logDebug(logger, "mcpe packet decode failed", conn, "client->server", "error", err, "bytes", len(data), "compressed", compressed)
				return err
			}
			logPacket(logger, conn, "client->server", pk)

			switch pk := pk.(type) {
			case *packet.RequestNetworkSettings:
				if err := handleNetworkSettingsRequest(conn, codec, logger, pk); err != nil {
					return err
				}
				compressed = true
			case *packet.Login:
				logLogin(logger, conn, pk)
			}
		}
	}
}

func decodeSessionBatch(codec Codec, data []byte, compressed bool) ([][]byte, error) {
	sessionCodec := codec
	if !compressed {
		sessionCodec.Compression = nil
	}
	return sessionCodec.DecodeBatch(data)
}

func handleNetworkSettingsRequest(conn net.Conn, codec Codec, logger *slog.Logger, pk *packet.RequestNetworkSettings) error {
	if pk.ClientProtocol != int32(CurrentProtocol) {
		status := packet.PlayStatusLoginFailedClient
		if pk.ClientProtocol > int32(CurrentProtocol) {
			status = packet.PlayStatusLoginFailedServer
		}
		failure := &packet.PlayStatus{Status: status}
		_ = writeSessionPacket(conn, codec, failure, false)
		logPacket(logger, conn, "server->client", failure)
		return fmt.Errorf("incompatible protocol version: expected %d, got %d", CurrentProtocol, pk.ClientProtocol)
	}

	settings := &packet.NetworkSettings{
		CompressionThreshold: uint16(codec.CompressionThreshold),
		CompressionAlgorithm: codec.Compression.EncodeCompression(),
	}
	if err := writeSessionPacket(conn, codec, settings, false); err != nil {
		return fmt.Errorf("write network settings: %w", err)
	}
	logPacket(logger, conn, "server->client", settings)
	return nil
}

func writeSessionPacket(conn net.Conn, codec Codec, pk Packet, compressed bool) error {
	payload, err := codec.EncodePacket(pk)
	if err != nil {
		return err
	}
	sessionCodec := codec
	if !compressed {
		sessionCodec.Compression = nil
	}
	batch, err := sessionCodec.EncodeBatch([][]byte{payload})
	if err != nil {
		return err
	}
	_, err = conn.Write(batch)
	return err
}

func logPacket(logger *slog.Logger, conn net.Conn, direction string, pk Packet) {
	logDebug(logger, "mcpe packet parsed", conn, direction, "packet_id", pk.ID(), "packet", fmt.Sprintf("%T", pk))
}

func logLogin(logger *slog.Logger, conn net.Conn, pk *packet.Login) {
	data, err := ParseLoginPacket(pk)
	if err != nil {
		logDebug(logger, "mcpe login parse failed", conn, "client->server", "error", err, "client_protocol", pk.ClientProtocol)
		return
	}
	logDebug(
		logger,
		"mcpe login parsed",
		conn,
		"client->server",
		"client_protocol", pk.ClientProtocol,
		"display_name", data.Identity.DisplayName,
		"identity", data.Identity.Identity,
		"xuid", data.Identity.XUID,
		"device_os", data.Client.DeviceOS,
		"game_version", data.Client.GameVersion,
		"language", data.Client.LanguageCode,
		"xbox_live_authenticated", data.Auth.XBOXLiveAuthenticated,
	)
}

func logDebug(logger *slog.Logger, message string, conn net.Conn, direction string, attrs ...any) {
	if logger == nil {
		return
	}
	base := []any{
		"remote", conn.RemoteAddr(),
		"direction", direction,
	}
	logger.Debug(message, append(base, attrs...)...)
}
