package protocol

import (
	"fmt"

	gtlogin "github.com/sandertv/gophertunnel/minecraft/protocol/login"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

type IdentityData = gtlogin.IdentityData
type ClientData = gtlogin.ClientData
type AuthResult = gtlogin.AuthResult

type LoginData struct {
	Identity IdentityData
	Client   ClientData
	Auth     AuthResult
}

func ParseLoginPacket(pk *packet.Login) (LoginData, error) {
	if pk == nil {
		return LoginData{}, fmt.Errorf("parse login packet: nil packet")
	}
	return ParseConnectionRequest(pk.ConnectionRequest)
}

func ParseConnectionRequest(request []byte) (LoginData, error) {
	identity, client, auth, err := gtlogin.Parse(request, nil)
	if err != nil {
		return LoginData{}, fmt.Errorf("parse login request: %w", err)
	}
	return LoginData{
		Identity: identity,
		Client:   client,
		Auth:     auth,
	}, nil
}
