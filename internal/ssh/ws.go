package ssh

import (
	"net/http"

	"nhooyr.io/websocket"
)

// ServeWebSocket terminates an SSH connection tunneled over a WebSocket. The
// HTTP handshake is expected to be authenticated by the surrounding API
// middleware (API key / OIDC); SSH publickey/cert auth then runs inside. This
// is the proxy-free fallback for restricted networks and the `stacy ssh` CLI.
func (s *Server) ServeWebSocket(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"*"}})
	if err != nil {
		s.logger.Debug().Err(err).Msg("ssh websocket accept failed")
		return
	}
	// Adapt the WebSocket to a net.Conn carrying the SSH transport, then run the
	// normal gateway handshake/serve loop. HandleConn closes the conn on return.
	nc := websocket.NetConn(r.Context(), c, websocket.MessageBinary)
	s.HandleConn(r.Context(), nc)
	_ = c.Close(websocket.StatusNormalClosure, "")
}
