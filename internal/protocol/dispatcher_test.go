package protocol

import (
	"context"
	"errors"
	"testing"

	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func TestDispatcherRoutesByPacketID(t *testing.T) {
	dispatcher := NewDispatcher()
	want := &packet.RequestNetworkSettings{ClientProtocol: CurrentProtocol}
	var got Packet

	if err := dispatcher.Register(packet.IDRequestNetworkSettings, func(_ context.Context, pk Packet) error {
		got = pk
		return nil
	}); err != nil {
		t.Fatalf("Register() returned error: %v", err)
	}
	if err := dispatcher.Dispatch(context.Background(), want); err != nil {
		t.Fatalf("Dispatch() returned error: %v", err)
	}
	if got != want {
		t.Fatalf("handler packet = %T %p, want %T %p", got, got, want, want)
	}
}

func TestDispatcherReportsUnhandledPacket(t *testing.T) {
	err := NewDispatcher().Dispatch(context.Background(), &packet.RequestNetworkSettings{})
	if !errors.Is(err, ErrUnhandledPacket) {
		t.Fatalf("Dispatch() error = %v, want ErrUnhandledPacket", err)
	}
}

func TestDispatcherRejectsNilInputs(t *testing.T) {
	dispatcher := NewDispatcher()
	if err := dispatcher.Register(packet.IDRequestNetworkSettings, nil); !errors.Is(err, ErrNilPacketHandler) {
		t.Fatalf("Register() error = %v, want ErrNilPacketHandler", err)
	}
	if err := dispatcher.Dispatch(context.Background(), nil); !errors.Is(err, ErrNilPacket) {
		t.Fatalf("Dispatch() error = %v, want ErrNilPacket", err)
	}
}
