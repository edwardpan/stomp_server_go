package stomp

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// Connection represents a STOMP connection over WebSocket
type Connection struct {
	ID            string
	ws            *websocket.Conn
	server        *StompServer
	version       string
	sessionID     string
	subscriptions map[string]*Subscription
	transactions  map[string]*Transaction
	mutex         sync.RWMutex
	heartbeat     *Heartbeat
	closed        bool
	ctx           context.Context
}

// Subscription represents a STOMP subscription
type Subscription struct {
	ID          string
	Destination string
	Ack         string // auto, client, client-individual
	Selector    string
}

// Transaction represents a STOMP transaction
type Transaction struct {
	ID       string
	Messages []*Frame
}

// Heartbeat manages heartbeat functionality
type Heartbeat struct {
	ClientSend time.Duration
	ClientRecv time.Duration
	ServerSend time.Duration
	ServerRecv time.Duration
	ticker     *time.Ticker
	stopCh     chan struct{}
}

// NewConnection creates a new STOMP connection
func NewConnection(ctx context.Context, ws *websocket.Conn, server *StompServer) *Connection {
	return &Connection{
		ID:            uuid.New().String(),
		ws:            ws,
		server:        server,
		subscriptions: make(map[string]*Subscription),
		transactions:  make(map[string]*Transaction),
		closed:        false,
		ctx:           ctx,
	}
}

// handleConnection handles the STOMP protocol for this connection
func (c *Connection) handleConnection(ctx context.Context) {
	defer func() {
		c.close(ctx)
		c.server.removeConnection(c.ID)
	}()

	for {
		if c.closed {
			break
		}

		_, message, err := c.ws.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				slog.Error("Read message error: %v", err)
			}
			break
		}

		// g.Log().Debug(ctx, "Received message type:", messageType, ", message:", string(message))

		frame, err := ParseFrame(message)
		if err != nil {
			// g.Log().Debug(ctx, "Parse frame error:", err)
			// c.SendError("Invalid frame format", err.Error())
			continue
		}

		err = c.handleFrame(frame)
		if err != nil {
			slog.Error("Handle frame error: %v", err)
			// c.SendError("Frame handling error", err.Error())
		}
	}
}

// handleFrame processes a STOMP frame
func (c *Connection) handleFrame(frame *Frame) error {
	switch frame.Command {
	case "CONNECT", "STOMP":
		return c.handleConnect(frame)
	case "SEND":
		return c.handleSend(frame)
	case "SUBSCRIBE":
		return c.handleSubscribe(frame)
	case "UNSUBSCRIBE":
		return c.handleUnsubscribe(frame)
	case "ACK":
		return c.handleAck(frame)
	case "NACK":
		return c.handleNack(frame)
	case "BEGIN":
		return c.handleBegin(frame)
	case "COMMIT":
		return c.handleCommit(frame)
	case "ABORT":
		return c.handleAbort(frame)
	case "DISCONNECT":
		return c.handleDisconnect(frame)
	default:
		return fmt.Errorf("unknown command: %s", frame.Command)
	}
}

// handleConnect processes CONNECT/STOMP frame
func (c *Connection) handleConnect(frame *Frame) error {
	// Determine protocol version
	version := frame.Headers["accept-version"]
	if version == "" {
		c.version = "1.0" // Default to 1.0 if not specified
	} else {
		// Choose the highest supported version
		supportedVersions := []string{"1.0", "1.1", "1.2"}
		clientVersions := strings.Split(version, ",")

		c.version = "1.0" // Default
		for _, supported := range supportedVersions {
			for _, client := range clientVersions {
				if strings.TrimSpace(client) == supported {
					c.version = supported
				}
			}
		}
	}

	// Generate session ID
	c.sessionID = uuid.New().String()

	// Handle heartbeat (for versions 1.1+)
	if c.version != "1.0" {
		heartbeatHeader := frame.Headers["heart-beat"]
		if heartbeatHeader != "" {
			c.setupHeartbeat(heartbeatHeader)
		}
	}

	// Send CONNECTED frame
	connectedFrame := &Frame{
		Command: "CONNECTED",
		Headers: map[string]string{
			"version": c.version,
			"session": c.sessionID,
			"server":  "GoFrame-STOMP/1.0",
		},
	}

	// Add heartbeat info for versions 1.1+
	if c.version != "1.0" && c.heartbeat != nil {
		connectedFrame.Headers["heart-beat"] = fmt.Sprintf("%d,%d",
			int(c.heartbeat.ServerSend.Milliseconds()),
			int(c.heartbeat.ServerRecv.Milliseconds()))
	}

	return c.SendFrame(connectedFrame)
}

// setupHeartbeat configures heartbeat based on client request
func (c *Connection) setupHeartbeat(heartbeatHeader string) {
	parts := strings.Split(heartbeatHeader, ",")
	if len(parts) != 2 {
		return
	}

	clientSend, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
	clientRecv, _ := strconv.Atoi(strings.TrimSpace(parts[1]))

	c.heartbeat = &Heartbeat{
		ClientSend: time.Duration(clientSend) * time.Millisecond,
		ClientRecv: time.Duration(clientRecv) * time.Millisecond,
		ServerSend: 20 * time.Second, // Default server send interval
		ServerRecv: 20 * time.Second, // Default server receive timeout
		stopCh:     make(chan struct{}),
	}

	// Start heartbeat if needed
	if c.heartbeat.ServerSend > 0 && c.heartbeat.ClientRecv > 0 {
		interval := c.heartbeat.ServerSend
		if c.heartbeat.ClientRecv < interval {
			interval = c.heartbeat.ClientRecv
		}

		c.heartbeat.ticker = time.NewTicker(interval)
		go c.sendHeartbeat()
	}
}

// sendHeartbeat sends periodic heartbeat frames
func (c *Connection) sendHeartbeat() {
	for {
		select {
		case <-c.heartbeat.ticker.C:
			if c.closed {
				return
			}
			// Send heartbeat (just a newline)
			// TODO heartbeat
			heartbeanFrame := &Frame{
				Command: "MESSAGE",
				Headers: make(map[string]string),
				Body:    []byte("\n"),
			}
			heartbeanFrame.Headers["destination"] = "/heartbeat"

			c.SendFrame(heartbeanFrame)
			// c.ws.WriteMessage(websocket.TextMessage, []byte("heartbeat\n"))
		case <-c.heartbeat.stopCh:
			return
		}
	}
}

// SendFrame sends a STOMP frame to the client
func (c *Connection) SendFrame(frame *Frame) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.closed {
		return fmt.Errorf("connection is closed")
	}

	data := frame.Marshal()
	return c.ws.WriteMessage(websocket.TextMessage, data)
}

// SendError sends an ERROR frame to the client
func (c *Connection) SendError(message, description string) {
	errorFrame := &Frame{
		Command: "ERROR",
		Headers: map[string]string{
			"message": message,
		},
		Body: []byte(description),
	}
	c.SendFrame(errorFrame)
}

// close closes the connection and cleans up resources
func (c *Connection) close(ctx context.Context) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.closed {
		return
	}

	c.closed = true

	// Stop heartbeat
	if c.heartbeat != nil && c.heartbeat.ticker != nil {
		c.heartbeat.ticker.Stop()
		close(c.heartbeat.stopCh)
	}

	// Remove all subscriptions
	for _, sub := range c.subscriptions {
		c.server.removeSubscription(sub.Destination, c.ID)
	}

	// Close WebSocket connection
	c.ws.Close()
}
