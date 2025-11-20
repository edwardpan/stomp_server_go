package stomp

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestFrameParsing tests STOMP frame parsing and marshaling
func TestFrameParsing(t *testing.T) {
	tests := []struct {
		name     string
		rawFrame string
		expected *Frame
	}{
		{
			name:     "CONNECT frame",
			rawFrame: "CONNECT\nlogin:guest\npasscode:guest\n\n\x00",
			expected: &Frame{
				Command: "CONNECT",
				Headers: map[string]string{
					"login":    "guest",
					"passcode": "guest",
				},
				Body: []byte{},
			},
		},
		{
			name:     "SEND frame with body",
			rawFrame: "SEND\ndestination:/queue/test\ncontent-type:text/plain\n\nHello World\x00",
			expected: &Frame{
				Command: "SEND",
				Headers: map[string]string{
					"destination":  "/queue/test",
					"content-type": "text/plain",
				},
				Body: []byte("Hello World"),
			},
		},
		{
			name:     "SUBSCRIBE frame",
			rawFrame: "SUBSCRIBE\ndestination:/topic/chat\nid:sub-1\nack:client\n\n\x00",
			expected: &Frame{
				Command: "SUBSCRIBE",
				Headers: map[string]string{
					"destination": "/topic/chat",
					"id":          "sub-1",
					"ack":         "client",
				},
				Body: []byte{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame, err := ParseFrame([]byte(tt.rawFrame))
			if err != nil {
				t.Fatalf("ParseFrame() error = %v", err)
			}

			if frame.Command != tt.expected.Command {
				t.Errorf("Command = %v, want %v", frame.Command, tt.expected.Command)
			}

			for key, expectedValue := range tt.expected.Headers {
				if actualValue, exists := frame.Headers[key]; !exists || actualValue != expectedValue {
					t.Errorf("Header[%s] = %v, want %v", key, actualValue, expectedValue)
				}
			}

			if !bytes.Equal(frame.Body, tt.expected.Body) {
				t.Errorf("Body = %v, want %v", frame.Body, tt.expected.Body)
			}
		})
	}
}

// TestFrameMarshaling tests STOMP frame marshaling
func TestFrameMarshaling(t *testing.T) {
	frame := &Frame{
		Command: "SEND",
		Headers: map[string]string{
			"destination":  "/queue/test",
			"content-type": "text/plain",
		},
		Body: []byte("Hello World"),
	}

	data := frame.Marshal()
	parsed, err := ParseFrame(data)
	if err != nil {
		t.Fatalf("ParseFrame() error = %v", err)
	}

	if parsed.Command != frame.Command {
		t.Errorf("Command = %v, want %v", parsed.Command, frame.Command)
	}

	if !bytes.Equal(parsed.Body, frame.Body) {
		t.Errorf("Body = %v, want %v", parsed.Body, frame.Body)
	}
}

// TestFrameValidation tests STOMP frame validation
func TestFrameValidation(t *testing.T) {
	tests := []struct {
		name    string
		frame   *Frame
		wantErr bool
	}{
		{
			name: "valid CONNECT frame",
			frame: &Frame{
				Command: "CONNECT",
				Headers: map[string]string{},
			},
			wantErr: false,
		},
		{
			name: "valid SEND frame",
			frame: &Frame{
				Command: "SEND",
				Headers: map[string]string{"destination": "/queue/test"},
			},
			wantErr: false,
		},
		{
			name: "invalid SEND frame - missing destination",
			frame: &Frame{
				Command: "SEND",
				Headers: map[string]string{},
			},
			wantErr: true,
		},
		{
			name: "invalid frame - empty command",
			frame: &Frame{
				Command: "",
				Headers: map[string]string{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.frame.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestHeaderEscaping tests header escaping for STOMP 1.1+
func TestHeaderEscaping(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "escape colon",
			input:    "key:value",
			expected: "key\\cvalue",
		},
		{
			name:     "escape newline",
			input:    "line1\nline2",
			expected: "line1\\nline2",
		},
		{
			name:     "escape carriage return",
			input:    "line1\rline2",
			expected: "line1\\rline2",
		},
		{
			name:     "escape backslash",
			input:    "path\\to\\file",
			expected: "path\\\\to\\\\file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			escaped := escapeHeaderValue(tt.input)
			if escaped != tt.expected {
				t.Errorf("escapeHeaderValue() = %v, want %v", escaped, tt.expected)
			}

			// Test round-trip
			unescaped := unescapeHeaderValue(escaped)
			if unescaped != tt.input {
				t.Errorf("unescapeHeaderValue() = %v, want %v", unescaped, tt.input)
			}
		})
	}
}

// TestStompServer tests the STOMP server functionality
func TestStompServer(t *testing.T) {
	ctx := context.Background()
	server := NewStompServer(websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins in development
		},
		Error: func(w http.ResponseWriter, r *http.Request, status int, reason error) {
			// Implement error handling logic here
		},
		ReadBufferSize:    4096, // 增加读缓冲区大小
		WriteBufferSize:   4096, // 增加写缓冲区大小
		EnableCompression: true, // 启用压缩
		// 支持STOMP子协议
		Subprotocols: []string{"v10.stomp", "v11.stomp", "v12.stomp", "stomp"},
	})

	// Test message handler registration
	handlerCalled := false
	server.SetMessageHandler("/test", func(ctx context.Context, conn *Connection, frame *Frame, params map[string]string) error {
		handlerCalled = true
		return nil
	})

	// Create test HTTP server
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		server.HandleWebSocket(ctx, w, r)
	}))
	defer testServer.Close()

	// Convert HTTP URL to WebSocket URL
	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	// Test WebSocket connection
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Send CONNECT frame
	connectFrame := NewFrame("CONNECT")
	connectFrame.SetHeader("accept-version", "1.2")
	connectFrame.SetHeader("host", "localhost")

	err = conn.WriteMessage(websocket.TextMessage, connectFrame.Marshal())
	if err != nil {
		t.Fatalf("Failed to send CONNECT: %v", err)
	}

	// Read CONNECTED frame
	_, message, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read CONNECTED: %v", err)
	}

	connectedFrame, err := ParseFrame(message)
	if err != nil {
		t.Fatalf("Failed to parse CONNECTED: %v", err)
	}

	if connectedFrame.Command != "CONNECTED" {
		t.Errorf("Expected CONNECTED, got %s", connectedFrame.Command)
	}

	// Test subscription
	subscribeFrame := NewFrame("SUBSCRIBE")
	subscribeFrame.SetHeader("destination", "/test")
	subscribeFrame.SetHeader("id", "sub-1")

	err = conn.WriteMessage(websocket.TextMessage, subscribeFrame.Marshal())
	if err != nil {
		t.Fatalf("Failed to send SUBSCRIBE: %v", err)
	}

	// Test sending message
	sendFrame := NewFrame("SEND")
	sendFrame.SetHeader("destination", "/test")
	sendFrame.SetBody([]byte("test message"))

	err = conn.WriteMessage(websocket.TextMessage, sendFrame.Marshal())
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Give some time for message processing
	time.Sleep(100 * time.Millisecond)

	if !handlerCalled {
		t.Error("Message handler was not called")
	}
}

// TestClientServerIntegration tests client-server integration
func TestClientServerIntegration(t *testing.T) {
	ctx := context.Background()
	// Create server
	server := NewStompServer(websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins in development
		},
		Error: func(w http.ResponseWriter, r *http.Request, status int, reason error) {
			// Implement error handling logic here
		},
		ReadBufferSize:    4096, // 增加读缓冲区大小
		WriteBufferSize:   4096, // 增加写缓冲区大小
		EnableCompression: true, // 启用压缩
		// 支持STOMP子协议
		Subprotocols: []string{"v10.stomp", "v11.stomp", "v12.stomp", "stomp"},
	})
	messageReceived := make(chan string, 1)

	server.SetMessageHandler("/topic/test", func(ctx context.Context, conn *Connection, frame *Frame, params map[string]string) error {
		messageReceived <- string(frame.Body)
		return server.SendToDestination("/topic/test", frame.Body, nil)
	})

	// Create test HTTP server
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		server.HandleWebSocket(ctx, w, r)
	}))
	defer testServer.Close()

	// Convert HTTP URL to WebSocket URL
	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	// Create client
	config := &ClientConfig{
		URL:            wsURL,
		Version:        "1.2",
		ConnectTimeout: 5 * time.Second,
	}

	client := NewClient(config)
	err := client.Connect(ctx, config)
	if err != nil {
		t.Fatalf("Failed to connect client: %v", err)
	}
	defer client.Disconnect(ctx)

	// Subscribe to topic
	messageChan, err := client.Subscribe("/topic/test", "auto")
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// Send message
	testMessage := "Hello, STOMP!"
	err = client.Send("/topic/test", []byte(testMessage), nil)
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Wait for message to be received by handler
	select {
	case receivedMessage := <-messageReceived:
		if receivedMessage != testMessage {
			t.Errorf("Expected %s, got %s", testMessage, receivedMessage)
		}
	case <-time.After(2 * time.Second):
		t.Error("Message not received by handler")
	}

	// Wait for message to be received by subscriber
	select {
	case frame := <-messageChan:
		if string(frame.Body) != testMessage {
			t.Errorf("Expected %s, got %s", testMessage, string(frame.Body))
		}
	case <-time.After(2 * time.Second):
		t.Error("Message not received by subscriber")
	}
}

// TestProtocolVersionNegotiation tests STOMP version negotiation
func TestProtocolVersionNegotiation(t *testing.T) {
	tests := []struct {
		name            string
		clientVersions  string
		expectedVersion string
	}{
		{
			name:            "STOMP 1.0 only",
			clientVersions:  "1.0",
			expectedVersion: "1.0",
		},
		{
			name:            "STOMP 1.2 preferred",
			clientVersions:  "1.0,1.1,1.2",
			expectedVersion: "1.2",
		},
		{
			name:            "STOMP 1.1 only",
			clientVersions:  "1.1",
			expectedVersion: "1.1",
		},
		{
			name:            "No version specified",
			clientVersions:  "",
			expectedVersion: "1.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			server := NewStompServer(websocket.Upgrader{
				CheckOrigin: func(r *http.Request) bool {
					return true // Allow all origins in development
				},
				Error: func(w http.ResponseWriter, r *http.Request, status int, reason error) {
					// Implement error handling logic here
				},
				ReadBufferSize:    4096, // 增加读缓冲区大小
				WriteBufferSize:   4096, // 增加写缓冲区大小
				EnableCompression: true, // 启用压缩
				// 支持STOMP子协议
				Subprotocols: []string{"v10.stomp", "v11.stomp", "v12.stomp", "stomp"},
			})

			// Create test HTTP server
			testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				server.HandleWebSocket(ctx, w, r)
			}))
			defer testServer.Close()

			// Convert HTTP URL to WebSocket URL
			wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

			// Test WebSocket connection
			conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			if err != nil {
				t.Fatalf("Failed to connect: %v", err)
			}
			defer conn.Close()

			// Send CONNECT frame with version
			connectFrame := NewFrame("CONNECT")
			if tt.clientVersions != "" {
				connectFrame.SetHeader("accept-version", tt.clientVersions)
			}

			err = conn.WriteMessage(websocket.TextMessage, connectFrame.Marshal())
			if err != nil {
				t.Fatalf("Failed to send CONNECT: %v", err)
			}

			// Read CONNECTED frame
			_, message, err := conn.ReadMessage()
			if err != nil {
				t.Fatalf("Failed to read CONNECTED: %v", err)
			}

			connectedFrame, err := ParseFrame(message)
			if err != nil {
				t.Fatalf("Failed to parse CONNECTED: %v", err)
			}

			if connectedFrame.Command != "CONNECTED" {
				t.Errorf("Expected CONNECTED, got %s", connectedFrame.Command)
			}

			actualVersion := connectedFrame.Headers["version"]
			if actualVersion != tt.expectedVersion {
				t.Errorf("Expected version %s, got %s", tt.expectedVersion, actualVersion)
			}
		})
	}
}

// BenchmarkFrameParsing benchmarks frame parsing performance
func BenchmarkFrameParsing(b *testing.B) {
	rawFrame := "SEND\ndestination:/queue/test\ncontent-type:text/plain\ncontent-length:11\n\nHello World\x00"
	data := []byte(rawFrame)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ParseFrame(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkFrameMarshaling benchmarks frame marshaling performance
func BenchmarkFrameMarshaling(b *testing.B) {
	frame := &Frame{
		Command: "SEND",
		Headers: map[string]string{
			"destination":  "/queue/test",
			"content-type": "text/plain",
		},
		Body: []byte("Hello World"),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = frame.Marshal()
	}
}

// Example test showing how to use the STOMP library
func ExampleStompServer_basic() {
	// Create STOMP server
	server := NewStompServer(websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins in development
		},
		Error: func(w http.ResponseWriter, r *http.Request, status int, reason error) {
			// Implement error handling logic here
		},
		ReadBufferSize:    4096, // 增加读缓冲区大小
		WriteBufferSize:   4096, // 增加写缓冲区大小
		EnableCompression: true, // 启用压缩
		// 支持STOMP子协议
		Subprotocols: []string{"v10.stomp", "v11.stomp", "v12.stomp", "stomp"},
	})

	// Set message handler
	server.SetMessageHandler("/topic/example", func(ctx context.Context, conn *Connection, frame *Frame, params map[string]string) error {
		fmt.Printf("Received: %s\n", string(frame.Body))
		return server.SendToDestination("/topic/example", frame.Body, nil)
	})

	// In a real application, you would integrate with GoFrame:
	// s := g.Server()
	// s.BindHandler("/stomp", func(r *ghttp.Request) {
	//     server.HandleWebSocket(r.Response.Writer, r.Request)
	// })
	// s.Run()

	fmt.Println("STOMP server created")
	// Output: STOMP server created
}
