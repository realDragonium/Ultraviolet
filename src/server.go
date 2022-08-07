package ultravioletv2

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"strings"

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

func Run() error {
	log.Println("Going to run!")
	// return BasicProxySetupBedrock()
	return BasicTestProxyToBedrock()
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

	// bitFlagDatagram is set for every valid datagram. It is used to identify packets that are datagrams.
	bitFlagDatagram = 0x80
	// bitFlagACK is set for every ACK packet.
	bitFlagACK = 0x40
	// bitFlagNACK is set for every NACK packet.
	bitFlagNACK = 0x20

	// currentProtocol is the current RakNet protocol version. This is Minecraft specific.
	currentProtocol byte = 10

	maxMTUSize    = 1400
	maxWindowSize = 2048
)

func BasicTestProxyToBedrock() error {
	udpListenToAddr := ":19132"
	conn, err := net.ListenPacket("udp", udpListenToAddr)
	if err != nil {
		log.Panicf("Error listening to udp: %v", err)
		return err
	}

	for {
		readBuffer := make([]byte, 2048)
		n, addr, _ := conn.ReadFrom(readBuffer)

		b := bytes.NewBuffer(readBuffer[:n])
		packetID, _ := b.ReadByte()
		otherConn, _ := net.Dial("udp", "0.0.0.0:19133")

		otherConn.Write(readBuffer[:n])
		// log.Printf("Wrote data: %#v", readBuffer[:n])

		readBuf := make([]byte, 2048)
		n, _ = otherConn.Read(readBuf)
		// log.Printf("Read data: %#v", readBuf[:n])

		readB := bytes.NewBuffer(readBuf[:n])

		switch packetID {
		case IDUnconnectedPing, IDUnconnectedPingOpenConnections:
			pingPk := UnconnectedPing{}
			pingPk.Read(b)
			log.Printf("%#v", pingPk)

			readB.ReadByte()
			pongPk := UnconnectedPong{}
			pongPk.Read(readB)

			// log.Printf("pongPk data %v", string(pongPk.Data))

			info := string(pongPk.Data)

			firstPart := strings.SplitAfter(info, "MCPE;")[0]
			lastPart := strings.SplitAfter(info, "A Minecraft Server;")[1]
			info = fmt.Sprintf("%sThis Server - UV;%s", firstPart, lastPart)

			firstPart = strings.SplitAfter(info, "Survival;1;")[0]
			info = fmt.Sprintf("%s19132;-1;", firstPart)

			firstPart = strings.SplitAfter(info, "100;")[0]
			lastPart = strings.SplitAfter(info, "Geyser;")[1]
			info = fmt.Sprintf("%s3394339436721259499;Ultraviolet;%s", firstPart, lastPart)

			// payload := "MCPE;This Server - UV;534;1.19.10;6;100;3394339436721259498;Ultraviolet;Survival;1;19132;-1;"
			// log.Printf("%s", info)
			// log.Printf("%s", payload)
			pongPk.Data = info
			// pongPk.ServerGUID = 3394339436721259499
			// pongPk.SendTimestamp = pingPk.SendTimestamp

			log.Printf("pong Pk: %#v", pongPk)
			pongBuffer := bytes.NewBuffer([]byte{})
			pongPk.Write(pongBuffer)
			_, err := conn.WriteTo(pongBuffer.Bytes(), addr)
			if err != nil {
				log.Printf("Error writing to server: %v", err)
				return err
			}

			// pong := UnconnectedPong{ServerGUID: pongPk.ServerGUID, SendTimestamp: pongPk.SendTimestamp, Data: pongPk.Data}
			// pongBuffer := bytes.NewBuffer([]byte{})
			// (&pong).Write(pongBuffer)

			// conn.WriteTo(pongBuffer.Bytes(), addr)

			// log.Printf("wrote number of bytes: %#v to %v", n, addr)
			// receivingPk := &UnconnectedPong{}
			// receivingPk.Read(readBuf)
			// log.Printf("%#v", receivingPk)
		default:
			log.Printf("Unknown packet: %#v - %v", packetID, b.Bytes())
			log.Printf("Response: %#v", readB.Bytes())
			conn.WriteTo(readB.Bytes(), addr)

			go func() {
				for {
					n, _, err := conn.ReadFrom(readBuffer)
					if err != nil {
						return
					}
					otherConn.Write(readBuffer[:n])
				}
			}()
			for {
				n, err = otherConn.Read(readBuf)
				if err != nil {
					break
				}
				conn.WriteTo(readBuf[:n], addr)
			}
		}
	}

	return nil
}

func BasicProxySetupBedrock() error {
	udpListenToAddr := ":19132"
	pc, err := net.ListenPacket("udp", udpListenToAddr)
	if err != nil {
		log.Panicf("Error listening to udp: %v", err)
		return err
	}

	for {
		buf := make([]byte, 2048)
		n, addr, err := pc.ReadFrom(buf)
		if err != nil {
			continue
		}
		// log.Println(addr)
		go serve(pc, addr, buf[:n])
	}
}

func serve(pc net.PacketConn, addr net.Addr, buf []byte) error {
	otherConn, ok := connections[addr.String()]
	if !ok {
		b := bytes.NewBuffer(buf)
		packetID, _ := b.ReadByte()

		switch packetID {
		case IDUnconnectedPing, IDUnconnectedPingOpenConnections:
			return handleUnconnectedPing(pc, addr, b)
		case IDOpenConnectionRequest1:
			return handleOpenConnectionRequest1(pc, addr, b)
		case IDOpenConnectionRequest2:
			return handleOpenConnectionRequest2(pc, addr, b)
		default:

			log.Printf("%v Packet Read: %#v", addr, buf)

			// p := make([]byte, 2048)
			// conn, err := net.Dial("udp", "0.0.0.0:19133")
			// if err != nil {
			// 	return fmt.Errorf("Some error %v", err)
			// }

			// _, err = conn.Write(buf)
			// if err != nil {
			// 	log.Printf("Error writing to server: %v", err)
			// 	return err
			// }

			// bufio.NewReader(conn).Read(p)
			// log.Printf("Packet: %#v", p)

			// pc.WriteTo(p, addr)

			// conn.Close()
		}
	}
	log.Printf("comminucation %v -> %v", addr, otherConn.RemoteAddr().String())
	otherConn.Write(buf)

	readBuf := make([]byte, 2048)
	n, _ := otherConn.Read(readBuf)
	log.Printf("comminucation %v <- %v", addr, otherConn.RemoteAddr().String())
	pc.WriteTo(readBuf[:n], addr)

	return nil
}

func handleOpenConnectionRequest1(pc net.PacketConn, addr net.Addr, b *bytes.Buffer) error {
	packet := &OpenConnectionRequest1{}
	if err := packet.Read(b); err != nil {
		return fmt.Errorf("error reading open connection request 1: %v", err)
	}
	b.Reset()
	mtuSize := packet.MaximumSizeNotDropped
	if mtuSize > maxMTUSize {
		mtuSize = maxMTUSize
	}

	(&OpenConnectionReply1{ServerGUID: 0, Secure: false, ServerPreferredMTUSize: mtuSize}).Write(b)
	_, err := pc.WriteTo(b.Bytes(), addr)
	return err
}

func handleOpenConnectionRequest2(pc net.PacketConn, addr net.Addr, b *bytes.Buffer) error {
	packet := &OpenConnectionRequest2{}
	if err := packet.Read(b); err != nil {
		return fmt.Errorf("error reading open connection request 2: %v", err)
	}
	b.Reset()

	mtuSize := packet.ClientPreferredMTUSize
	if mtuSize > maxMTUSize {
		mtuSize = maxMTUSize
	}

	udp_addr, _ := addr.(*net.UDPAddr)
	(&OpenConnectionReply2{ServerGUID: 0, ClientAddress: *udp_addr, MTUSize: mtuSize}).Write(b)
	if _, err := pc.WriteTo(b.Bytes(), addr); err != nil {
		return fmt.Errorf("error sending open connection reply 2: %v", err)
	}

	// conn := newConn(listener.conn, addr, packet.ClientPreferredMTUSize)
	// conn.close = func() {
	// 	// Make sure to remove the connection from the Listener once the Conn is closed.
	// 	connections.Delete(addr.String())
	// }

	otherConn, _ := net.Dial("udp", "0.0.0.0:19133")
	connections[addr.String()] = otherConn

	return nil
}

// handleUnconnectedPing handles an unconnected ping packet stored in buffer b, coming from an address addr.
func handleUnconnectedPing(pc net.PacketConn, addr net.Addr, b *bytes.Buffer) error {
	payload := "MCPE;LINE_1;534;1.19.10.24;1;10;23984798238;Line2;Survival;1;19132;19132;"
	// payload := []byte("MCPE;Dedicated Server;534;1.19.10.24;0;10;13253860892328930865;Bedrock level;Survival;1;19132;19132;")

	pk := &UnconnectedPing{}
	if err := pk.Read(b); err != nil {
		return fmt.Errorf("error reading unconnected ping: %v", err)
	}
	b.Reset()
	pong := UnconnectedPong{ServerGUID: 23984798236, SendTimestamp: pk.SendTimestamp, Data: payload}
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
	return net.Listen("udp", cfg.ListenTo)
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
