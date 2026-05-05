package protocol

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var (
	ErrNilPacket        = errors.New("nil packet")
	ErrNilPacketHandler = errors.New("nil packet handler")
	ErrUnhandledPacket  = errors.New("unhandled packet")
)

type Handler func(context.Context, Packet) error

type Dispatcher struct {
	mu       sync.RWMutex
	handlers map[uint32]Handler
}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{handlers: make(map[uint32]Handler)}
}

func (dispatcher *Dispatcher) Register(packetID uint32, handler Handler) error {
	if handler == nil {
		return ErrNilPacketHandler
	}

	dispatcher.mu.Lock()
	dispatcher.handlers[packetID] = handler
	dispatcher.mu.Unlock()
	return nil
}

func (dispatcher *Dispatcher) Dispatch(ctx context.Context, pk Packet) error {
	if pk == nil {
		return ErrNilPacket
	}

	dispatcher.mu.RLock()
	handler := dispatcher.handlers[pk.ID()]
	dispatcher.mu.RUnlock()
	if handler == nil {
		return fmt.Errorf("%w: packet id %d (%T)", ErrUnhandledPacket, pk.ID(), pk)
	}
	return handler(ctx, pk)
}
