package conn_test

import (
	"net"
	"testing"
	"time"

	"github.com/realDragonium/UltraViolet/conn"
)

var defaultChTimeout time.Duration = 10 * time.Millisecond

type testListener struct {
	newConnCh <-chan net.Conn
}

func (l *testListener) Close() error {
	return nil
}

func (l *testListener) Addr() net.Addr {
	return nil
}

func (l *testListener) Accept() (net.Conn, error) {
	return <-l.newConnCh, nil
}

func TestListener(t *testing.T) {
	t.Run("accept call", func(t *testing.T) {
		newConnCh := make(chan net.Conn)
		listener := &testListener{
			newConnCh: newConnCh,
		}
		go func() {
			conn.ServeListener(listener)
		}()
		select {
		case newConnCh <- &net.TCPConn{}:
			t.Log("Listener called accept")
		case <-time.After(defaultChTimeout):
			t.Error("Listener didnt accept connection")
		}
	})

	t.Run("accept call", func(t *testing.T) {
		newConnCh := make(chan net.Conn)
		listener := &testListener{
			newConnCh: newConnCh,
		}
		go func() {
			conn.ServeListener(listener)
		}()
		select {
		case newConnCh <- &net.TCPConn{}:
			t.Log("Listener called accept")
		case <-time.After(defaultChTimeout):
			t.Error("Listener didnt accept connection")
		}
	})

}
