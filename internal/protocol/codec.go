package protocol

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	gtprotocol "github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

const (
	CurrentProtocol = gtprotocol.CurrentProtocol
	CurrentVersion  = gtprotocol.CurrentVersion

	DefaultCompressionThreshold = 256
	MaxDecompressedBatchBytes   = 2 * 1024 * 1024

	batchHeader       = 0xfe
	maximumBatchItems = 812
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
	encryption           *batchEncryption
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

func (codec *Codec) EnableEncryption(keyBytes [32]byte) {
	codec.encryption = newBatchEncryption(keyBytes)
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
	var body bytes.Buffer
	for _, payload := range packets {
		if err := gtprotocol.WriteVaruint32(&body, uint32(len(payload))); err != nil {
			return nil, fmt.Errorf("encode batch: write packet length: %w", err)
		}
		if _, err := body.Write(payload); err != nil {
			return nil, fmt.Errorf("encode batch: write packet payload: %w", err)
		}
	}

	data := body.Bytes()
	prefix := []byte{batchHeader}
	if compression := codec.Compression; compression != nil {
		if len(data) < codec.CompressionThreshold {
			compression = packet.NopCompression
		}
		compressed, err := compression.Compress(data)
		if err != nil {
			return nil, fmt.Errorf("compress batch: %w", err)
		}
		data = compressed
		prefix = append(prefix, byte(compression.EncodeCompression()))
	}

	batch := append(prefix, data...)
	if codec.encryption != nil {
		batch = codec.encryption.outgoing.encrypt(batch)
	}
	return batch, nil
}

func (codec Codec) DecodeBatch(data []byte) ([][]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	if data[0] != batchHeader {
		return nil, fmt.Errorf("decode batch: invalid header %x, expected %x", data[0], batchHeader)
	}
	body := data[1:]
	if codec.encryption != nil {
		body = append([]byte(nil), body...)
		codec.encryption.incoming.decrypt(body)
		if err := codec.encryption.incoming.verify(body); err != nil {
			return nil, fmt.Errorf("verify batch: %w", err)
		}
		body = body[:len(body)-8]
	}

	if codec.Compression != nil {
		if len(body) == 0 {
			return nil, fmt.Errorf("decompress batch: missing compression header")
		}
		if body[0] == 0xff {
			body = body[1:]
		} else {
			compression, ok := packet.CompressionByID(uint16(body[0]))
			if !ok {
				return nil, fmt.Errorf("decompress batch: unknown compression algorithm %v", body[0])
			}
			if compression != codec.Compression {
				return nil, fmt.Errorf("decompress batch: unexpected compression algorithm: got %v, expected %v", compression, codec.Compression)
			}
			decompressed, err := compression.Decompress(body[1:], codec.MaxDecompressedLen)
			if err != nil {
				return nil, fmt.Errorf("decompress batch: %w", err)
			}
			body = decompressed
		}
	}

	buffer := bytes.NewBuffer(body)
	packets := make([][]byte, 0, 1)
	for buffer.Len() != 0 {
		var length uint32
		if err := gtprotocol.Varuint32(buffer, &length); err != nil {
			return nil, fmt.Errorf("decode batch: read packet length: %w", err)
		}
		if uint32(buffer.Len()) < length {
			return nil, fmt.Errorf("decode batch: packet length %d exceeds remaining %d", length, buffer.Len())
		}
		packets = append(packets, append([]byte(nil), buffer.Next(int(length))...))
	}
	if len(packets) > maximumBatchItems && codec.EnableLimits {
		return nil, fmt.Errorf("decode batch: number of packets %v exceeds max=%v", len(packets), maximumBatchItems)
	}
	return packets, nil
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

type batchEncryption struct {
	outgoing *cryptState
	incoming *cryptState
}

func newBatchEncryption(keyBytes [32]byte) *batchEncryption {
	return &batchEncryption{
		outgoing: newCryptState(keyBytes),
		incoming: newCryptState(keyBytes),
	}
}

type cryptState struct {
	counter uint64
	key     []byte
	stream  cipher.Stream
}

func newCryptState(keyBytes [32]byte) *cryptState {
	block, _ := aes.NewCipher(keyBytes[:])
	iv := append(append([]byte(nil), keyBytes[:12]...), 0, 0, 0, 2)
	return &cryptState{
		key:    keyBytes[:],
		stream: cipher.NewCTR(block, iv),
	}
}

func (state *cryptState) encrypt(data []byte) []byte {
	var counter [8]byte
	binary.LittleEndian.PutUint64(counter[:], state.counter)
	state.counter++

	hash := sha256.New()
	hash.Write(counter[:])
	hash.Write(data[1:])
	hash.Write(state.key)
	data = append(data, hash.Sum(nil)[:8]...)

	state.stream.XORKeyStream(data[1:], data[1:])
	return data
}

func (state *cryptState) decrypt(data []byte) {
	state.stream.XORKeyStream(data, data)
}

func (state *cryptState) verify(data []byte) error {
	if len(data) < 8 {
		return fmt.Errorf("encrypted packet must be at least 8 bytes long, got %v", len(data))
	}
	got := data[len(data)-8:]

	var counter [8]byte
	binary.LittleEndian.PutUint64(counter[:], state.counter)
	state.counter++

	hash := sha256.New()
	hash.Write(counter[:])
	hash.Write(data[:len(data)-8])
	hash.Write(state.key)
	want := hash.Sum(nil)[:8]
	if !bytes.Equal(got, want) {
		return fmt.Errorf("invalid checksum of packet %v: expected %x, got %x", state.counter-1, want, got)
	}
	return nil
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
