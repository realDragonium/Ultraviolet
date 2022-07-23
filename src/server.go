package ultravioletv2

import (
	"bytes"
	"io"
	"log"
	"net"
)

func Run() error {
	ln, err := net.Listen("tcp", ":25565")
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

		_, packetId, data, err := ReadPacketData(conn)
		if err != nil {
			log.Printf("Error reading packet data: %v", err)
			return err
		}
		log.Printf("Packet id: %#x", packetId)

		r := bytes.NewReader(data)
		hsPacket, _ := ReadServerBoundHandshake(r)
		log.Printf("Packet: %#v", hsPacket)

		ConnectToServer(conn, data)
	}
}

func ReadServerBoundHandshake(r io.Reader) (pk ServerBoundHandshakePacket, err error) {
	pk.ProtocolVersion, err = ReadVarInt(r)
	if err != nil {
		return
	}
	pk.ServerAddress, err = ReadString(r)
	if err != nil {
		return
	}
	pk.ServerPort, err = ReadShort(r)
	if err != nil {
		return
	}
	pk.NextState, err = ReadVarInt(r)
	if err != nil {
		return
	}

	return
}

func ReadPacketData(r io.Reader) (int, byte, []byte, error) {
	packetLength, _ := ReadVarInt(r)
	log.Println("Packet length:", packetLength)

	packetId, _ := ReadByte(r)

	if packetLength-1 < 1 {
		return packetLength, packetId, []byte{}, nil
	}

	data := make([]byte, packetLength-1)

	n, err := r.Read(data)
	log.Println("Read:", n, "bytes")
	log.Println(data)
	if err != nil {
		log.Println("got error during reading of bytes: ", err)
		return 0, 0, data, err
	}

	return packetLength, packetId, data, nil
}

func ConnectToServer(client net.Conn, data []byte) {
	server, err := net.Dial("tcp", "localhost:25566")
	if err != nil {
		log.Println("Error connecting to server:", err)
		return
	}
	log.Println("Connected to server")
	length := len(data) + 1

	WriteVarInt(server, length)
	WriteVarInt(server, 0x00)
	server.Write(data)

	log.Println("Proxying connection to server")
	ProxyConnection(client, server)
}

func ProxyConnection(client, server net.Conn) {
	go func() {
		pipe(server, client)
		client.Close()
	}()
	pipe(client, server)
	server.Close()
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
