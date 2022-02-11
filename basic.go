package ultraviolet

import (
	"errors"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/realDragonium/Ultraviolet/core"
	"github.com/realDragonium/Ultraviolet/mc"
)

var (
	ConnTimeoutDuration  = 5 * time.Second

)

type API interface {
	Run(addr string)
	Close()
}

func ReadStuff(conn net.Conn) (reqData core.RequestData, err error) {
	conn.SetDeadline(time.Now().Add(ConnTimeoutDuration))
	mcConn := mc.NewMcConn(conn)

	handshakePacket, err := mcConn.ReadPacket()
	if errors.Is(err, os.ErrDeadlineExceeded) {
		err = core.ErrClientToSlow
		return
	} else if err != nil {
		return
	}

	handshake, err := mc.UnmarshalServerBoundHandshake(handshakePacket)
	if err != nil {
		log.Printf("error while parsing handshake: %v", err)
	}
	reqType := mc.RequestState(handshake.NextState)
	if reqType == mc.UnknownState {
		err = core.ErrNotValidHandshake
		return
	}

	packet, err := mcConn.ReadPacket()
	if errors.Is(err, os.ErrDeadlineExceeded) {
		err = core.ErrClientToSlow
		return
	} else if err != nil {
		return
	}
	conn.SetDeadline(time.Time{})

	serverAddr := strings.ToLower(handshake.ParseServerAddress())
	reqData = core.RequestData{
		Type:       reqType,
		ServerAddr: serverAddr,
		Addr:       conn.RemoteAddr(),
		Handshake:  handshake,
	}

	if reqType == mc.Login {
		loginStart, err := mc.UnmarshalServerBoundLoginStart(packet)
		if err != nil {
			log.Printf("error while parsing login packet: %v", err)
			return reqData, err
		}
		reqData.Username = string(loginStart.Name)
	}

	return reqData, nil
}

func LookupServer(req core.RequestData, servers core.ServerCatalog) (core.Server, error) {
	return servers.Find(req.ServerAddr)
}

func SendResponse(conn net.Conn, pk mc.Packet, withPing bool) error {
	conn.SetDeadline(time.Now().Add(ConnTimeoutDuration))

	mcConn := mc.NewMcConn(conn)

	if err := mcConn.WritePacket(pk); err != nil {
		return err
	}

	if withPing {
		pingPacket, err := mcConn.ReadPacket()
		if err != nil {
			return err
		}

		mcConn.WritePacket(pingPacket)
	}

	return nil
}

func FullRun(conn net.Conn, servers core.ServerCatalog) error {
	reqData, err := ReadStuff(conn)
	if err != nil {
		return err
	}

	server, err := LookupServer(reqData, servers)
	if errors.Is(err, core.ErrNoServerFound) && reqData.Type == mc.Status {
		return SendResponse(conn, servers.DefaultStatus(), true)
	} else if err != nil {
		log.Printf("got error: %v", err)
		return conn.Close()
	}

	return ProcessServer(conn, server, reqData)
}

func ProcessServer(conn net.Conn, server core.Server, reqData core.RequestData) error {
	action := server.ConnAction(reqData)

	if action == core.PROXY {
		go ProxyConnection(conn, server, reqData)
		return nil
	}

	defer conn.Close()

	var responsePk mc.Packet
	switch action {
	// TOOD: Figure this one out
	// case core.VERIFY_CONN:
		// responsePk = servers.VerifyConn()
	case core.STATUS:
		responsePk = server.Status()
	case core.CLOSE:
		return nil
	}

	return SendResponse(conn, responsePk, action == core.STATUS)
}

func ProxyConnection(client net.Conn, server core.Server, reqData core.RequestData) (err error) {
	serverConn, err := server.CreateConn(reqData)
	if err != nil {
		return
	}

	go func() {
		pipe(serverConn, client)
		client.Close()
	}()

	go func() {
		pipe(client, serverConn)
		serverConn.Close()
	}()

	return
}

func pipe(c1, c2 net.Conn) {
	buffer := make([]byte, 0xffff)
	for {
		n, err := c1.Read(buffer)
		if err != nil {
			return
		}
		_, err = c2.Write(buffer[:n])
		if err != nil {
			return
		}
	}
}
