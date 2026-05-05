package server

import (
	"fmt"

	gtlogin "github.com/sandertv/gophertunnel/minecraft/protocol/login"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

type loginData struct {
	Identity gtlogin.IdentityData
	Client   gtlogin.ClientData
	Auth     gtlogin.AuthResult
}

func parseLoginPacket(pk *packet.Login) (loginData, error) {
	if pk == nil {
		return loginData{}, fmt.Errorf("parse login packet: nil packet")
	}
	return parseConnectionRequest(pk.ConnectionRequest)
}

func parseConnectionRequest(request []byte) (loginData, error) {
	identity, client, auth, err := gtlogin.Parse(request, nil)
	if err != nil {
		return loginData{}, fmt.Errorf("parse login request: %w", err)
	}
	return loginData{
		Identity: identity,
		Client:   client,
		Auth:     auth,
	}, nil
}
