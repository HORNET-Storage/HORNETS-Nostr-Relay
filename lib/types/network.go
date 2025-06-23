// Network communication and streaming types
package types

import (
	"bytes"
	"context"
	"io"

	"github.com/gofiber/contrib/websocket"
)

// Stream interface for network communication
type Stream interface {
	Read(p []byte) (int, error)
	Write(p []byte) (int, error)
	Close() error
	Context() context.Context
}

// WebSocketStream implements Stream interface for WebSocket connections
type WebSocketStream struct {
	Conn        *websocket.Conn
	Ctx         context.Context
	writeBuffer bytes.Buffer
}

// NewWebSocketStream creates a new WebSocket stream
func NewWebSocketStream(conn *websocket.Conn, ctx context.Context) *WebSocketStream {
	return &WebSocketStream{
		Conn: conn,
		Ctx:  ctx,
	}
}

// Read reads data from the WebSocket connection
func (ws *WebSocketStream) Read(msg []byte) (int, error) {
	_, reader, err := ws.Conn.ReadMessage()
	if err != nil {
		return 0, err
	}
	return io.ReadFull(bytes.NewReader(reader), msg)
}

// Write writes data to the WebSocket buffer
func (ws *WebSocketStream) Write(msg []byte) (int, error) {
	ws.writeBuffer.Write(msg)
	return len(msg), nil
}

// Flush sends buffered data through the WebSocket connection
func (ws *WebSocketStream) Flush() error {
	err := ws.Conn.WriteMessage(websocket.BinaryMessage, ws.writeBuffer.Bytes())
	if err != nil {
		return err
	}
	ws.writeBuffer.Reset()
	return nil
}

// Close closes the WebSocket connection
func (ws *WebSocketStream) Close() error {
	return ws.Conn.Close()
}

// Context returns the context for the WebSocket stream
func (ws *WebSocketStream) Context() context.Context {
	return ws.Ctx
}
