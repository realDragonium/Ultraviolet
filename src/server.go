package ultravioletv2

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"

	"github.com/realDragonium/Ultraviolet/config"
	"github.com/realDragonium/Ultraviolet/core"
)

var (
	UVConfig config.UltravioletConfig = config.UltravioletConfig{
		ListenTo: ":25565",
	}
	Servers     map[string]string   = map[string]string{}
	connections map[string]net.Conn = map[string]net.Conn{}
)

func Run(cfgPath string) error {
	log.Println("Going to run!")

	bedrockServerConfigs, err := ReadBedrockConfigs(cfgPath)
	if err != nil {
		return err
	}

	for _, cfg := range bedrockServerConfigs {
		listener, err := CreateBedrockListener(cfg)
		if err != nil {
			return err
		}
		go StartBedrockServer(listener, cfg)
	}

	javaServerConfigs, err := ReadJavaConfigs(cfgPath)
	if err != nil {
		return err
	}

	for _, cfg := range javaServerConfigs {
		for _, domain := range cfg.Domains {
			Servers[domain] = cfg.ListenTo
		}
	}

	go BasicProxySetupJava()

	log.Println("Finished starting up")
	select {}
}

const (
	IDUnconnectedPing                byte = 0x01
	IDUnconnectedPingOpenConnections byte = 0x02
	IDUnconnectedPong                byte = 0x1c
)

func CreateBedrockListener(cfg BedrockServerConfig) (net.PacketConn, error) {
	listener, err := net.ListenPacket("udp", cfg.ListenTo)
	if err != nil {
		log.Panicf("Error listening to udp: %v", err)
		return listener, err
	}
	return listener, nil
}

func StartBedrockServer(listener net.PacketConn, cfg BedrockServerConfig) error {
	buf := make([]byte, 2048)
	for {
		n, addr, err := listener.ReadFrom(buf)
		if err != nil {
			return err
		}
		go serve(cfg, listener, addr, buf[:n])
	}
}

func serve(cfg BedrockServerConfig, pc net.PacketConn, addr net.Addr, bb []byte) error {
	buf := bytes.NewBuffer(bb)
	packetID, _ := buf.ReadByte()

	switch packetID {
	case IDUnconnectedPing, IDUnconnectedPingOpenConnections:
		return handleUnconnectedPing(cfg, pc, addr, buf)
	default:
		serverConn, ok := connections[addr.String()]
		if !ok {
			serverConn, _ = net.Dial("udp", cfg.ProxyTo)
		}

		serverConn.Write(bb)

		if !ok {
			connections[addr.String()] = serverConn

			go func() {
				for {
					b := make([]byte, 2048)
					n, err := serverConn.Read(b)
					if err != nil {
						break
					}
					pc.WriteTo(b[:n], addr)
				}
			}()
		}
	}

	return nil
}

func handleUnconnectedPing(cfg BedrockServerConfig, pc net.PacketConn, addr net.Addr, b *bytes.Buffer) error {
	pk := &UnconnectedPing{}
	if err := pk.Read(b); err != nil {
		return fmt.Errorf("error reading unconnected ping: %v", err)
	}
	b.Reset()
	pong := UnconnectedPong{ServerGUID: cfg.ID, SendTimestamp: pk.SendTimestamp, Data: cfg.Status()}
	(&pong).Write(b)
	pc.WriteTo(b.Bytes(), addr)

	return nil
}

func BasicProxySetupJava() error {
	ln, err := CreateListener(UVConfig)
	if err != nil {
		return err
	}

	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}

		ProcessConnection(conn)
	}
}

func CreateListener(cfg config.UltravioletConfig) (net.Listener, error) {
	return net.Listen("tcp", cfg.ListenTo)
}

func ProcessConnection(conn net.Conn) error {
	_, data, err := ReadPacketData(conn)
	if err != nil {
		log.Printf("Error reading packet data: %v", err)
		return err
	}

	r := bytes.NewReader(data)
	hsPacket, _ := ReadServerBoundHandshake(r)

	server, _ := ConnectToServer(hsPacket)
	return ProxyConnection(conn, server)
}

func ReadPacketData(r io.Reader) (int, []byte, error) {
	packetLength, _ := ReadVarInt(r)

	if packetLength < 1 {
		return packetLength, []byte{}, nil
	}

	data := make([]byte, packetLength)

	if _, err := r.Read(data); err != nil {
		log.Println("got error during reading of bytes: ", err)
		return 0, data, err
	}

	return packetLength, data, nil
}

func ConnectToServer(pk ServerBoundHandshakePacket) (net.Conn, error) {
	addr, _ := ServerAddress(pk)

	server, err := net.Dial("tcp", addr)
	if err != nil {
		log.Println("Error connecting to server:", err)
		return nil, err
	}

	pk.WriteTo(server)

	return server, err
}

func ServerAddress(hsPacket ServerBoundHandshakePacket) (string, error) {
	serverAddr, ok := Servers[hsPacket.ServerAddress]

	if !ok {
		return "", core.ErrNoServerFound
	}

	return serverAddr, nil
}

func ProxyConnection(client, server net.Conn) error {
	go func() {
		pipe(server, client)
		client.Close()
	}()
	pipe(client, server)
	server.Close()

	return nil
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
