package ultraviolet

//go:generate stringer -type=ServerState
type ServerState byte

const (
	Unknown ServerState = iota
	Online
	Offline
)
