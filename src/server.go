package ultravioletv2

import (
	"bytes"
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

func Run() error {
	ln, err := net.Listen("tcp", UVConfig.ListenTo)
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

func ProcessConnection(conn net.Conn) error {
	_, data, err := ReadPacketData(conn)
	if err != nil {
		log.Printf("Error reading packet data: %v", err)
		return err
	}
	
	r := bytes.NewReader(data)
	hsPacket, _ := ReadServerBoundHandshake(r)
	log.Printf("Packet: %#v", hsPacket)

	return ConnectToServer(conn, data)
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

func ConnectToServer(client net.Conn, data []byte) error {

	server, err := net.Dial("tcp", "localhost:25566")
	if err != nil {
		log.Println("Error connecting to server:", err)
		return err
	}
	log.Println("Connected to server")
	length := len(data) + 1

	WriteVarInt(server, length)
	WriteVarInt(server, 0x00)
	server.Write(data)

	log.Println("Proxying connection to server")
	return ProxyConnection(client, server)
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
