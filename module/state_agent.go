package module

import (
	"time"

	ultraviolet "github.com/realDragonium/Ultraviolet"
)

type StateAgent interface {
	State() ultraviolet.ServerState
}

func NewMcServerState(cooldown time.Duration, connCreator ConnectionCreator) StateAgent {
	return &McServerState{
		state:       ultraviolet.Unknown,
		cooldown:    cooldown,
		connCreator: connCreator,
		startTime:   time.Time{},
	}
}

type McServerState struct {
	state       ultraviolet.ServerState
	cooldown    time.Duration
	startTime   time.Time
	connCreator ConnectionCreator
}

func (server *McServerState) State() ultraviolet.ServerState {
	if time.Since(server.startTime) <= server.cooldown {
		return server.state
	}
	server.startTime = time.Now()
	connFunc := server.connCreator.Conn()
	conn, err := connFunc()
	if err != nil {
		server.state = ultraviolet.Offline
	} else {
		server.state = ultraviolet.Online
		conn.Close()
	}
	return server.state
}

type AlwaysOnlineState struct{}

func (agent AlwaysOnlineState) State() ultraviolet.ServerState {
	return ultraviolet.Online
}

type AlwaysOfflineState struct{}

func (agent AlwaysOfflineState) State() ultraviolet.ServerState {
	return ultraviolet.Offline
}
