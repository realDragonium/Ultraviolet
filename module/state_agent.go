package module

import (
	"time"

	"github.com/realDragonium/Ultraviolet/core"
)

type StateAgent interface {
	State() core.ServerState
}

func NewMcServerState(cooldown time.Duration, connCreator ConnectionCreator) StateAgent {
	return &McServerState{
		state:       core.Unknown,
		cooldown:    cooldown,
		connCreator: connCreator,
		startTime:   time.Time{},
	}
}

type McServerState struct {
	state       core.ServerState
	cooldown    time.Duration
	startTime   time.Time
	connCreator ConnectionCreator
}

func (server *McServerState) State() core.ServerState {
	if time.Since(server.startTime) <= server.cooldown {
		return server.state
	}
	server.startTime = time.Now()
	connFunc := server.connCreator.Conn()
	conn, err := connFunc()
	if err != nil {
		server.state = core.Offline
	} else {
		server.state = core.Online
		conn.Close()
	}
	return server.state
}

type AlwaysOnlineState struct{}

func (agent AlwaysOnlineState) State() core.ServerState {
	return core.Online
}

type AlwaysOfflineState struct{}

func (agent AlwaysOfflineState) State() core.ServerState {
	return core.Offline
}
