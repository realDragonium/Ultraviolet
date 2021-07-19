package old_proxy_test

import (
	"net"
	"testing"
	"time"

	"github.com/realDragonium/Ultraviolet/mc"
	"github.com/realDragonium/Ultraviolet/old_proxy"
)

func BenchmarkReadPackets_Status(b *testing.B) {
	hs := mc.ServerBoundHandshake{
		NextState: mc.StatusState,
		// ServerAddress: "Something///oiweuroijdslfjelwrjklejlkjlskdjflkwjelrkjlksjdfvm.werlkjdslkfjlwkrjelkdjslkvjlkjewlrkjlkdjslkvjlwjekrja;lsfkojsaelrkjewlkrjrflisdufiljewlrlsdiupvjlerkm;sdjglijhewlirj;esdfjgioerteuhjlkshfiywuehrkjhesdiuygweuhrjkhsdkfhuewyioroiewuoifu",
		ServerAddress: "Something",
	}.Marshal()
	hsBytes, _ := hs.Marshal()
	packet := mc.Packet{ID: 0x00}
	statusRequestBytes, _ := packet.Marshal()
	newConn := func() net.Conn {
		return &benchConnection{
			handshake: hsBytes,
			packet:    statusRequestBytes,
		}
	}

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		bconn := newConn()
		old_proxy.ReadPackets(bconn)
	}
}

func BenchmarkReadConnection_Status(b *testing.B) {
	reqCh := make(chan old_proxy.McRequest, 5)

	go func() {
		closeAns := old_proxy.NewMcAnswerClose()
		for {
			req := <-reqCh
			req.Ch <- closeAns
		}
	}()

	hs := mc.ServerBoundHandshake{
		NextState: mc.StatusState,
		// ServerAddress: "Something///oiweuroijdslfjelwrjklejlkjlskdjflkwjelrkjlksjdfvm.werlkjdslkfjlwkrjelkdjslkvjlkjewlrkjlkdjslkvjlwjekrja;lsfkojsaelrkjewlkrjrflisdufiljewlrlsdiupvjlerkm;sdjglijhewlirj;esdfjgioerteuhjlkshfiywuehrkjhesdiuygweuhrjkhsdkfhuewyioroiewuoifu",
		ServerAddress: "Something",
	}.Marshal()
	hsBytes, _ := hs.Marshal()
	packet := mc.Packet{ID: 0x00}
	statusRequestBytes, _ := packet.Marshal()
	newConn := func() net.Conn {
		return &benchConnection{
			handshake: hsBytes,
			packet:    statusRequestBytes,
		}
	}

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		bconn := newConn()
		old_proxy.ReadConnection_Bench2(bconn, reqCh)
	}
}

func BenchmarkReadConnection_Login(b *testing.B) {
	reqCh := make(chan old_proxy.McRequest)

	go func() {
		closeAns := old_proxy.NewMcAnswerClose()
		for {
			req := <-reqCh
			req.Ch <- closeAns
		}
	}()

	hs := mc.ServerBoundHandshake{
		NextState:     mc.StatusState,
		ServerAddress: "Something",
		// ServerAddress: "Something///oiweuroijdslfjelwrjklejlkjlskdjflkwjelrkjlksjdfvm.werlkjdslkfjlwkrjelkdjslkvjlkjewlrkjlkdjslkvjlwjekrja;lsfkojsaelrkjewlkrjrflisdufiljewlrlsdiupvjlerkm;sdjglijhewlirj;esdfjgioerteuhjlkshfiywuehrkjhesdiuygweuhrjkhsdkfhuewyioroiewuoifu",
	}.Marshal()
	hsBytes, _ := hs.Marshal()
	packet := mc.ServerLoginStart{Name: "ultraviolet"}.Marshal()
	statusRequestBytes, _ := packet.Marshal()
	newConn := func() net.Conn {
		return &benchConnection{
			handshake: hsBytes,
			packet:    statusRequestBytes,
		}
	}

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		bconn := newConn()
		old_proxy.ReadConnection_Bench2(bconn, reqCh)
	}
}

type benchConnection struct {
	count     int
	handshake []byte
	packet    []byte
}

func (c *benchConnection) Read(b []byte) (n int, err error) {
	n = 0
	if c.count == 0 {
		n = len(c.handshake)
		copy(b, c.handshake)
	} else {
		n = len(c.packet)
		copy(b, c.packet)
	}
	c.count++
	return n, nil
}

func (c *benchConnection) Write(b []byte) (n int, err error) {
	return 0, nil
}

func (c *benchConnection) Close() error {
	return nil
}
func (c *benchConnection) LocalAddr() net.Addr {
	return &net.IPAddr{}
}

func (c *benchConnection) RemoteAddr() net.Addr {
	return &net.IPAddr{}
}

func (c *benchConnection) SetDeadline(t time.Time) error {
	return nil
}

func (c *benchConnection) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *benchConnection) SetWriteDeadline(t time.Time) error {
	return nil
}
