package sessions

import "github.com/leefowlercu/sigil/internal/appserver/protocol"

// State stores one connection-local handshake/session state.
type State struct {
	Initialized     bool
	Ready           bool
	ProtocolVersion string
	Client          protocol.ClientInfo
}
