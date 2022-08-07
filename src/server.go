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

var UVConfig config.UltravioletConfig = config.UltravioletConfig{
	ListenTo: ":25565",
}

var Servers map[string]string = map[string]string{
	"localhost": "localhost:25566",
}

var connections map[string]net.Conn = map[string]net.Conn{}

var bedrockServerConfig BedrockServerConfig = BedrockServerConfig{
	BaseConfig: BaseConfig{
		ListenTo: ":19132",
		ProxyTo:  ":19133",
	},
	ID: 23894692837498,
	ServerStatus: BedrockStatus{
		Edition: "MCPE",
		Description: Description{
			Text: "This is a test server - Ultraviolet",
		},
		Version: Version{
			Name:     "1.19.10",
			Protocol: 534,
		},
		Players: Players{
			Online: 0,
			Max:    100,
		},
		Gamemode: GameMode{
			Name: "Survival",
			ID:   1,
		},
		Port: Port{
			IPv4: 19132,
			IPv6: -1,
		},
	},
}

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

	for {}
}

const (
	IDConnectedPing                  byte = 0x00
	IDUnconnectedPing                byte = 0x01
	IDUnconnectedPingOpenConnections byte = 0x02
	IDConnectedPong                  byte = 0x03
	IDDetectLostConnections          byte = 0x04
	IDOpenConnectionRequest1         byte = 0x05
	IDOpenConnectionReply1           byte = 0x06
	IDOpenConnectionRequest2         byte = 0x07
	IDOpenConnectionReply2           byte = 0x08
	IDConnectionRequest              byte = 0x09
	IDConnectionRequestAccepted      byte = 0x10
	IDNewIncomingConnection          byte = 0x13
	IDDisconnectNotification         byte = 0x15

	IDIncompatibleProtocolVersion byte = 0x19

	IDUnconnectedPong byte = 0x1c
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

func BasicProxySetupBedrock() error {
	listener, err := CreateBedrockListener(bedrockServerConfig)
	if err != nil {
		log.Panicf("Error creating listener: %v", err)
		return err
	}
	return StartBedrockServer(listener, bedrockServerConfig)
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
	log.Printf("%v -> %v", addr, b.Bytes())
	pc.WriteTo(b.Bytes(), addr)

	return nil
}

func BasicProxySetupJava() error {
	ln, err := CreateListener(UVConfig)
	if err != nil {
		return err
	}

	for {
		log.Println("-----------------------------------------------------")
		log.Println("Waiting for connection")
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		log.Println("Connection accepted")

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
	log.Printf("Packet: %#v", hsPacket)

	server, _ := ConnectToServer(hsPacket)
	return ProxyConnection(conn, server)
}

func ReadPacketData(r io.Reader) (int, []byte, error) {
	packetLength, _ := ReadVarInt(r)
	log.Println("Packet length:", packetLength)

	if packetLength < 1 {
		return packetLength, []byte{}, nil
	}

	data := make([]byte, packetLength)

	n, err := r.Read(data)
	log.Println("Read:", n, "bytes")
	log.Println(data)
	if err != nil {
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
	log.Println("Connected to server")

	pk.WriteTo(server)

	log.Println("Proxying connection to server")
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
