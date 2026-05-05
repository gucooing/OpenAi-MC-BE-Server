package protocol

import (
	"bytes"
	"fmt"

	gtprotocol "github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

const (
	CurrentProtocol = gtprotocol.CurrentProtocol
	CurrentVersion  = gtprotocol.CurrentVersion

	DefaultCompressionThreshold = 256
	MaxDecompressedBatchBytes   = 2 * 1024 * 1024
)

type Packet = packet.Packet

type Direction int

const (
	FromClient Direction = iota
	FromServer
)

type Codec struct {
	Compression          packet.Compression
	CompressionThreshold int
	MaxDecompressedLen   int
	ShieldID             int32
	EnableLimits         bool
	clientPool           packet.Pool
	serverPool           packet.Pool
}

func NewCodec() Codec {
	return Codec{
		Compression:          packet.DefaultCompression,
		CompressionThreshold: DefaultCompressionThreshold,
		MaxDecompressedLen:   MaxDecompressedBatchBytes,
		EnableLimits:         true,
		clientPool:           packet.NewClientPool(),
		serverPool:           packet.NewServerPool(),
	}
}

func (codec Codec) EncodePacket(pk Packet) (data []byte, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			data = nil
			err = fmt.Errorf("encode packet %T: %v", pk, recovered)
		}
	}()

	buffer := bytes.NewBuffer(nil)
	if err := (&packet.Header{PacketID: pk.ID()}).Write(buffer); err != nil {
		return nil, fmt.Errorf("write packet header: %w", err)
	}
	pk.Marshal(gtprotocol.NewWriter(buffer, codec.ShieldID))
	return buffer.Bytes(), nil
}

func (codec Codec) DecodePacket(data []byte, direction Direction) (pk Packet, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			pk = nil
			err = fmt.Errorf("decode packet: %v", recovered)
		}
	}()

	buffer := bytes.NewBuffer(data)
	var header packet.Header
	if err := header.Read(buffer); err != nil {
		return nil, fmt.Errorf("read packet header: %w", err)
	}

	factory := codec.pool(direction)[header.PacketID]
	if factory == nil {
		return nil, fmt.Errorf("unknown packet id %d for %s", header.PacketID, direction)
	}

	pk = factory()
	pk.Marshal(gtprotocol.NewReader(buffer, codec.ShieldID, codec.EnableLimits))
	if buffer.Len() != 0 {
		return nil, fmt.Errorf("packet %T has %d unread bytes", pk, buffer.Len())
	}
	return pk, nil
}

func (codec Codec) EncodeBatch(packets [][]byte) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := packet.NewEncoder(&buffer)
	if codec.Compression != nil {
		encoder.EnableCompression(codec.Compression, codec.CompressionThreshold)
	}
	if err := encoder.Encode(packets); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func (codec Codec) DecodeBatch(data []byte) ([][]byte, error) {
	decoder := packet.NewDecoder(bytes.NewReader(data))
	if codec.Compression != nil {
		decoder.EnableCompression(codec.Compression, codec.MaxDecompressedLen)
	}
	return decoder.Decode()
}

func (codec Codec) pool(direction Direction) packet.Pool {
	switch direction {
	case FromClient:
		if codec.clientPool != nil {
			return codec.clientPool
		}
		return packet.NewClientPool()
	case FromServer:
		if codec.serverPool != nil {
			return codec.serverPool
		}
		return packet.NewServerPool()
	default:
		return packet.Pool{}
	}
}

func (direction Direction) String() string {
	switch direction {
	case FromClient:
		return "client"
	case FromServer:
		return "server"
	default:
		return "unknown"
	}
}
