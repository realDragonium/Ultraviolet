package ultraviolet

import "time"

type StateAgent interface {
	State() ServerState
}

func NewMcServerState(cooldown time.Duration, connCreator ConnectionCreator) StateAgent {
	return &McServerState{
		state:       UNKNOWN,
		cooldown:    cooldown,
		connCreator: connCreator,
		startTime:   time.Time{},
	}
}

type McServerState struct {
	state       ServerState
	cooldown    time.Duration
	startTime   time.Time
	connCreator ConnectionCreator
}

func (server *McServerState) State() ServerState {
	if time.Since(server.startTime) <= server.cooldown {
		return server.state
	}
	server.startTime = time.Now()
	connFunc := server.connCreator.Conn()
	conn, err := connFunc()
	if err != nil {
		server.state = OFFLINE
	} else {
		server.state = ONLINE
		conn.Close()
	}
	return server.state
}

type AlwaysOnlineState struct{}

func (agent AlwaysOnlineState) State() ServerState {
	return ONLINE
}

type AlwaysOfflineState struct{}

func (agent AlwaysOfflineState) State() ServerState {
	return OFFLINE
}
