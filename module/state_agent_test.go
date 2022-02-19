package module_test

import (
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/realDragonium/Ultraviolet/core"
	"github.com/realDragonium/Ultraviolet/module"
)

var (
	ErrEmptyConnCreator = errors.New("this is a test conn creator which doesnt provide connections")
	defaultChTimeout    = 25 * time.Millisecond
)

func TestAlwaysOnlineState(t *testing.T) {
	stateAgent := module.AlwaysOnlineState{}

	if stateAgent.State() != core.Online {
		t.Errorf("expected to be online but got %v instead", stateAgent.State())
	}
}

func TestAlwaysOfflineState(t *testing.T) {
	stateAgent := module.AlwaysOfflineState{}

	if stateAgent.State() != core.Offline {
		t.Errorf("expected to be offline but got %v instead", stateAgent.State())
	}
}

type stateConnCreator struct {
	callAmount  int
	returnError bool
}

func (creator *stateConnCreator) Conn() func() (net.Conn, error) {
	creator.callAmount++
	if creator.returnError {
		return func() (net.Conn, error) {
			return nil, ErrEmptyConnCreator
		}
	}
	return func() (net.Conn, error) {
		return &net.TCPConn{}, nil
	}
}

func TestMcServerState(t *testing.T) {
	tt := []struct {
		returnError   bool
		expectedState core.ServerState
	}{
		{
			expectedState: core.Offline,
			returnError:   true,
		},
		{
			expectedState: core.Online,
			returnError:   false,
		},
	}
	t.Run("single run state", func(t *testing.T) {
		for _, tc := range tt {
			name := fmt.Sprintf("returnError:%v - expectedState:%v", tc.returnError, tc.expectedState)
			t.Run(name, func(t *testing.T) {
				cooldown := time.Minute
				connCreator := stateConnCreator{
					returnError: tc.returnError,
				}
				stateAgent := module.NewMcServerState(cooldown, &connCreator)
				state := stateAgent.State()
				if state != tc.expectedState {
					t.Errorf("expected to be %v but got %v instead", tc.expectedState, state)
				}
				if connCreator.callAmount != 1 {
					t.Errorf("expected connCreator to be called %v times but was called %v time", 1, connCreator.callAmount)
				}
			})
		}
	})

	t.Run("doesnt call again while in cooldown", func(t *testing.T) {
		for _, tc := range tt {
			name := fmt.Sprintf("returnError:%v - expectedState:%v", tc.returnError, tc.expectedState)
			t.Run(name, func(t *testing.T) {
				cooldown := time.Minute
				connCreator := stateConnCreator{
					returnError: tc.returnError,
				}
				stateAgent := module.NewMcServerState(cooldown, &connCreator)
				stateAgent.State()
				state := stateAgent.State()
				if state != tc.expectedState {
					t.Errorf("expected to be %v but got %v instead", tc.expectedState, state)
				}
				if connCreator.callAmount != 1 {
					t.Errorf("expected connCreator to be called %v times but was called %v time", 1, connCreator.callAmount)
				}
			})
		}
	})

	t.Run("does call again after cooldown", func(t *testing.T) {
		for _, tc := range tt {
			name := fmt.Sprintf("returnError:%v - expectedState:%v", tc.returnError, tc.expectedState)
			t.Run(name, func(t *testing.T) {
				cooldown := time.Millisecond
				connCreator := stateConnCreator{
					returnError: tc.returnError,
				}
				stateAgent := module.NewMcServerState(cooldown, &connCreator)
				stateAgent.State()
				time.Sleep(cooldown)
				state := stateAgent.State()
				if state != tc.expectedState {
					t.Errorf("expected to be %v but got %v instead", tc.expectedState, state)
				}
				if connCreator.callAmount != 2 {
					t.Errorf("expected connCreator to be called %v times but was called %v time", 2, connCreator.callAmount)
				}
			})
		}
	})
}
