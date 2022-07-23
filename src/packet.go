package ultravioletv2

type MCPacket struct {

}

type ServerBoundHandshakePacket struct{
	MCPacket
	ProtocolVersion int
	ServerAddress   string
	ServerPort      int16
	NextState       int
}

func (pk ServerBoundHandshakePacket) IsStatusRequest() bool {
	return false
}

func (pk ServerBoundHandshakePacket) IsLoginRequest() bool {
	return false
}

type ServerBoundStatusRequestPacket struct{
	MCPacket
}

type ClientBoundStatusResponsePacket struct{
	MCPacket
}


type ServerBoundPingRequestPacket struct {
	MCPacket
}

type ClientBoundPongResponsePacket struct {
	MCPacket
}
