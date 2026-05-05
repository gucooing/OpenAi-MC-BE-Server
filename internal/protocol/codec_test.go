package protocol

import (
	"bytes"
	"testing"

	gtprotocol "github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func TestCodecUsesGophertunnelProtocolBaseline(t *testing.T) {
	if CurrentProtocol != 944 {
		t.Fatalf("CurrentProtocol = %d, want 944", CurrentProtocol)
	}
	if CurrentVersion != "1.26.10" {
		t.Fatalf("CurrentVersion = %q, want 1.26.10", CurrentVersion)
	}
	if CurrentProtocol != gtprotocol.CurrentProtocol {
		t.Fatalf("CurrentProtocol = %d, gophertunnel = %d", CurrentProtocol, gtprotocol.CurrentProtocol)
	}
}

func TestPacketRoundTripNetworkSettings(t *testing.T) {
	codec := NewCodec()
	source := &packet.NetworkSettings{
		CompressionThreshold: 256,
		CompressionAlgorithm: packet.CompressionAlgorithmFlate,
		ClientThrottle:       true,
		ClientThrottleScalar: 0.5,
	}

	encoded, err := codec.EncodePacket(source)
	if err != nil {
		t.Fatalf("EncodePacket() returned error: %v", err)
	}

	decoded, err := codec.DecodePacket(encoded, FromServer)
	if err != nil {
		t.Fatalf("DecodePacket() returned error: %v", err)
	}

	got, ok := decoded.(*packet.NetworkSettings)
	if !ok {
		t.Fatalf("DecodePacket() = %T, want *packet.NetworkSettings", decoded)
	}
	if got.CompressionThreshold != source.CompressionThreshold || got.CompressionAlgorithm != source.CompressionAlgorithm || got.ClientThrottle != source.ClientThrottle || got.ClientThrottleScalar != source.ClientThrottleScalar {
		t.Fatalf("NetworkSettings = %+v, want %+v", got, source)
	}
}

func TestBatchRoundTripUsesGophertunnelCompression(t *testing.T) {
	codec := NewCodec()
	first := []byte{0x01, 0x02, 0x03}
	second := bytes.Repeat([]byte{0x7f}, 512)

	encoded, err := codec.EncodeBatch([][]byte{first, second})
	if err != nil {
		t.Fatalf("EncodeBatch() returned error: %v", err)
	}

	decoded, err := codec.DecodeBatch(encoded)
	if err != nil {
		t.Fatalf("DecodeBatch() returned error: %v", err)
	}
	if len(decoded) != 2 || !bytes.Equal(decoded[0], first) || !bytes.Equal(decoded[1], second) {
		t.Fatalf("DecodeBatch() = %v, want original packets", decoded)
	}
}

func TestDecodePacketRejectsUnknownID(t *testing.T) {
	buffer := bytes.NewBuffer(nil)
	if err := (&packet.Header{PacketID: 999}).Write(buffer); err != nil {
		t.Fatalf("Header.Write() returned error: %v", err)
	}

	_, err := NewCodec().DecodePacket(buffer.Bytes(), FromClient)
	if err == nil {
		t.Fatalf("DecodePacket() error = nil, want unknown packet id error")
	}
}

func TestDecodePacketConvertsReaderPanicToError(t *testing.T) {
	codec := NewCodec()
	buffer := bytes.NewBuffer(nil)
	if err := (&packet.Header{PacketID: packet.IDNetworkSettings}).Write(buffer); err != nil {
		t.Fatalf("Header.Write() returned error: %v", err)
	}

	_, err := codec.DecodePacket(buffer.Bytes(), FromServer)
	if err == nil {
		t.Fatalf("DecodePacket() error = nil, want truncated payload error")
	}
}
