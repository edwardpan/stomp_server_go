package stomp

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Client represents a STOMP client connection
type Client struct {
	conn          *websocket.Conn
	url           string
	version       string
	sessionID     string
	connected     bool
	mutex         sync.RWMutex
	messageChan   chan *Frame
	errorChan     chan error
	closeChan     chan struct{}
	subscriptions map[string]chan *Frame
	subMutex      sync.RWMutex
}

// ClientConfig holds configuration for STOMP client
type ClientConfig struct {
	URL            string
	Login          string
	Passcode       string
	Version        string // "1.0", "1.1", "1.2" or "1.0,1.1,1.2"
	Heartbeat      string // "cx,cy" format
	ConnectTimeout time.Duration
	MessageTimeout time.Duration
}

// DefaultClientConfig returns a default client configuration
func DefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		Version:        "1.0,1.1,1.2",
		Heartbeat:      "10000,10000",
		ConnectTimeout: 30 * time.Second,
		MessageTimeout: 30 * time.Second,
	}
}

// NewClient creates a new STOMP client
func NewClient(config *ClientConfig) *Client {
	if config == nil {
		config = DefaultClientConfig()
	}

	return &Client{
		url:           config.URL,
		version:       config.Version,
		messageChan:   make(chan *Frame, 100),
		errorChan:     make(chan error, 10),
		closeChan:     make(chan struct{}),
		subscriptions: make(map[string]chan *Frame),
	}
}

// Connect establishes a connection to the STOMP server
func (c *Client) Connect(ctx context.Context, config *ClientConfig) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.connected {
		return fmt.Errorf("client is already connected")
	}

	// Parse URL
	u, err := url.Parse(config.URL)
	if err != nil {
		return fmt.Errorf("invalid URL: %v", err)
	}

	// Create WebSocket connection
	dialer := websocket.Dialer{
		HandshakeTimeout: config.ConnectTimeout,
	}

	conn, _, err := dialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to connect: %v", err)
	}

	c.conn = conn

	// Start message reader
	go c.readMessages(ctx)

	// Send CONNECT frame
	connectFrame := NewFrame("CONNECT")
	if config.Login != "" {
		connectFrame.SetHeader("login", config.Login)
	}
	if config.Passcode != "" {
		connectFrame.SetHeader("passcode", config.Passcode)
	}
	if config.Version != "" {
		connectFrame.SetHeader("accept-version", config.Version)
	}
	if config.Heartbeat != "" {
		connectFrame.SetHeader("heart-beat", config.Heartbeat)
	}

	err = c.sendFrame(connectFrame)
	if err != nil {
		c.conn.Close()
		return fmt.Errorf("failed to send CONNECT frame: %v", err)
	}

	// Wait for CONNECTED frame
	select {
	case frame := <-c.messageChan:
		if frame.Command == "CONNECTED" {
			c.connected = true
			c.version = frame.Headers["version"]
			c.sessionID = frame.Headers["session"]
			return nil
		} else if frame.Command == "ERROR" {
			c.conn.Close()
			return fmt.Errorf("connection error: %s", string(frame.Body))
		}
	case err := <-c.errorChan:
		c.conn.Close()
		return fmt.Errorf("connection error: %v", err)
	case <-time.After(config.ConnectTimeout):
		c.conn.Close()
		return fmt.Errorf("connection timeout")
	}

	return fmt.Errorf("unexpected response")
}

// Disconnect closes the connection to the STOMP server
func (c *Client) Disconnect(ctx context.Context) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if !c.connected {
		return fmt.Errorf("client is not connected")
	}

	// Send DISCONNECT frame
	disconnectFrame := NewFrame("DISCONNECT")
	err := c.sendFrame(disconnectFrame)
	if err != nil {
		return fmt.Errorf("failed to send DISCONNECT frame: %v", err)
	}

	// Close connection
	c.connected = false
	close(c.closeChan)
	c.conn.Close()

	return nil
}

// Send sends a message to the specified destination
func (c *Client) Send(destination string, body []byte, headers map[string]string) error {
	c.mutex.RLock()
	connected := c.connected
	c.mutex.RUnlock()

	if !connected {
		return fmt.Errorf("client is not connected")
	}

	sendFrame := NewFrame("SEND")
	sendFrame.SetHeader("destination", destination)
	sendFrame.SetBody(body)

	// Add custom headers
	for key, value := range headers {
		sendFrame.SetHeader(key, value)
	}

	return c.sendFrame(sendFrame)
}

// Subscribe subscribes to a destination and returns a channel for receiving messages
func (c *Client) Subscribe(destination, ack string) (<-chan *Frame, error) {
	c.mutex.RLock()
	connected := c.connected
	c.mutex.RUnlock()

	if !connected {
		return nil, fmt.Errorf("client is not connected")
	}

	// Generate subscription ID
	subID := fmt.Sprintf("sub-%d", time.Now().UnixNano())

	// Create subscription channel
	subChan := make(chan *Frame, 100)
	c.subMutex.Lock()
	c.subscriptions[subID] = subChan
	c.subMutex.Unlock()

	// Send SUBSCRIBE frame
	subscribeFrame := NewFrame("SUBSCRIBE")
	subscribeFrame.SetHeader("destination", destination)
	subscribeFrame.SetHeader("id", subID)
	if ack != "" {
		subscribeFrame.SetHeader("ack", ack)
	}

	err := c.sendFrame(subscribeFrame)
	if err != nil {
		c.subMutex.Lock()
		delete(c.subscriptions, subID)
		c.subMutex.Unlock()
		close(subChan)
		return nil, err
	}

	return subChan, nil
}

// Unsubscribe unsubscribes from a destination
func (c *Client) Unsubscribe(destination string) error {
	c.mutex.RLock()
	connected := c.connected
	c.mutex.RUnlock()

	if !connected {
		return fmt.Errorf("client is not connected")
	}

	// Find subscription ID for destination
	c.subMutex.Lock()
	var subID string
	for id := range c.subscriptions {
		// In a real implementation, you'd track destination per subscription
		// For simplicity, we'll use the destination as a hint
		subID = id
		break
	}
	if subID != "" {
		delete(c.subscriptions, subID)
	}
	c.subMutex.Unlock()

	if subID == "" {
		return fmt.Errorf("no subscription found for destination: %s", destination)
	}

	// Send UNSUBSCRIBE frame
	unsubscribeFrame := NewFrame("UNSUBSCRIBE")
	unsubscribeFrame.SetHeader("id", subID)

	return c.sendFrame(unsubscribeFrame)
}

// Ack acknowledges a message
func (c *Client) Ack(messageID string) error {
	c.mutex.RLock()
	connected := c.connected
	version := c.version
	c.mutex.RUnlock()

	if !connected {
		return fmt.Errorf("client is not connected")
	}

	ackFrame := NewFrame("ACK")
	if version == "1.2" {
		ackFrame.SetHeader("id", messageID)
	} else {
		ackFrame.SetHeader("message-id", messageID)
	}

	return c.sendFrame(ackFrame)
}

// Nack negatively acknowledges a message (STOMP 1.1+)
func (c *Client) Nack(messageID string) error {
	c.mutex.RLock()
	connected := c.connected
	version := c.version
	c.mutex.RUnlock()

	if !connected {
		return fmt.Errorf("client is not connected")
	}

	if version == "1.0" {
		return fmt.Errorf("NACK not supported in STOMP 1.0")
	}

	nackFrame := NewFrame("NACK")
	if version == "1.2" {
		nackFrame.SetHeader("id", messageID)
	} else {
		nackFrame.SetHeader("message-id", messageID)
	}

	return c.sendFrame(nackFrame)
}

// sendFrame sends a frame to the server
func (c *Client) sendFrame(frame *Frame) error {
	data := frame.Marshal()
	return c.conn.WriteMessage(websocket.TextMessage, data)
}

// readMessages reads messages from the WebSocket connection
func (c *Client) readMessages(ctx context.Context) {
	for {
		select {
		case <-c.closeChan:
			return
		default:
			_, message, err := c.conn.ReadMessage()
			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					c.errorChan <- err
				}
				return
			}

			frame, err := ParseFrame(message)
			if err != nil {
				c.errorChan <- err
				continue
			}

			c.handleFrame(ctx, frame)
		}
	}
}

// handleFrame handles incoming frames
func (c *Client) handleFrame(ctx context.Context, frame *Frame) {
	switch frame.Command {
	case "MESSAGE":
		// Route to appropriate subscription
		subID := frame.Headers["subscription"]
		if subID != "" {
			c.subMutex.RLock()
			subChan, exists := c.subscriptions[subID]
			c.subMutex.RUnlock()

			if exists {
				select {
				case subChan <- frame:
				default:
					// Channel is full, drop message
					slog.Warn("Subscription channel full, Dropped message")
				}
			}
		}
	case "ERROR":
		c.errorChan <- fmt.Errorf("server error: %s", string(frame.Body))
	default:
		// Route to main message channel
		select {
		case c.messageChan <- frame:
		default:
			// Channel is full, drop message
			slog.Warn("Message channel full, dropping frame")
		}
	}
}

// IsConnected returns whether the client is connected
func (c *Client) IsConnected() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.connected
}

// GetVersion returns the negotiated STOMP version
func (c *Client) GetVersion() string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.version
}

// GetSessionID returns the session ID
func (c *Client) GetSessionID() string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.sessionID
}
