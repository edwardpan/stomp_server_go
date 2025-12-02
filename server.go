package stomp

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// StompServer represents a STOMP server over WebSocket
type StompServer struct {
	upgrader              websocket.Upgrader
	connections           map[string]*Connection
	mutex                 sync.RWMutex
	handlers              map[string]*MessageHandlerListener
	subscriptions         map[string]map[string]*Subscription
	subMutex              sync.RWMutex
	subscriptionListeners []*SubscriptionListener
	listenerMutex         sync.RWMutex
}

type MessageHandlerListener struct {
	Pattern    string
	Regex      *regexp.Regexp
	ParamNames []string
	Handler    MessageHandler
}

// MessageHandler defines the interface for handling STOMP messages
type MessageHandler func(ctx context.Context, conn *Connection, frame *Frame, params map[string]string) error

// SubscriptionListener represents a subscription listener with pattern matching
type SubscriptionListener struct {
	Pattern    string                       // Topic pattern with placeholders like /topic/mission/{missionId}/log
	Regex      *regexp.Regexp               // Compiled regex for matching
	ParamNames []string                     // Parameter names extracted from pattern
	Callback   SubscriptionListenerCallback // Callback function with extracted parameters
}

// SubscriptionListenerCallback defines the callback function signature for subscription listeners
type SubscriptionListenerCallback func(ctx context.Context, topic string, params map[string]string)

// NewStompServer creates a new STOMP server instance
func NewStompServer(upgrader websocket.Upgrader) *StompServer {
	return &StompServer{
		upgrader:              upgrader,
		connections:           make(map[string]*Connection),
		handlers:              make(map[string]*MessageHandlerListener),
		subscriptions:         make(map[string]map[string]*Subscription),
		subscriptionListeners: make([]*SubscriptionListener, 0),
	}
}

func (s *StompServer) GetConnections() map[string]*Connection {
	return s.connections
}

func (s *StompServer) GetSubscriptions() map[string]map[string]*Subscription {
	return s.subscriptions
}

// SetMessageHandler sets a handler for a specific destination
func (s *StompServer) SetMessageHandler(pattern string, handler MessageHandler) error {
	// Convert pattern to regex and extract parameter names
	regexPattern, paramNames, err := s.convertPatternToRegex(pattern)
	if err != nil {
		return fmt.Errorf("invalid pattern '%s': %v", pattern, err)
	}

	// Compile regex
	regex, err := regexp.Compile(regexPattern)
	if err != nil {
		return fmt.Errorf("failed to compile regex for pattern '%s': %v", pattern, err)
	}

	listener := &MessageHandlerListener{
		Pattern:    pattern,
		Regex:      regex,
		ParamNames: paramNames,
		Handler:    handler,
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.handlers[pattern] = listener

	return nil
}

// HandleWebSocket upgrades HTTP connection to WebSocket and handles STOMP protocol
func (s *StompServer) HandleWebSocket(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	// 记录请求信息
	slog.Debug("收到WebSocket连接请求 - 远程地址: %s, 协议: %v",
		r.RemoteAddr, r.Header.Get("Sec-WebSocket-Protocol"))

	// 设置更长的读写超时
	upgrader := s.upgrader
	upgrader.HandshakeTimeout = 10 * time.Second

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("WebSocket upgrade failed: %v", err)
		return
	}

	// 配置WebSocket连接
	// conn.SetReadLimit(65536) // 设置最大消息大小为64KB
	// conn.SetReadDeadline(time.Time{})  // 禁用读超时
	// conn.SetWriteDeadline(time.Time{}) // 禁用写超时

	// conn.SetPingHandler(func(string) error {
	// 	// 收到Ping消息时发送Pong消息
	// 	slog.Info("收到Ping消息: %s", conn.RemoteAddr())
	// 	return conn.WriteMessage(websocket.PongMessage, []byte("pong\n"))
	// })
	// // 设置更合理的Pong处理函数，避免连接异常关闭
	// conn.SetPongHandler(func(string) error {
	// 	// 收到Pong消息时重置读取超时
	// 	slog.Info("收到Pong消息: %s", conn.RemoteAddr())
	// 	conn.SetReadDeadline(time.Now().Add(20 * time.Second))
	// 	return nil
	// })

	stompConn := NewConnection(ctx, conn, s)
	s.addConnection(stompConn)

	slog.Debug("新的STOMP连接已建立: %s", stompConn.ID)
	go stompConn.handleConnection(ctx)
}

// addConnection adds a new connection to the server
func (s *StompServer) addConnection(conn *Connection) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.connections[conn.ID] = conn
}

// removeConnection removes a connection from the server
func (s *StompServer) removeConnection(connID string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	delete(s.connections, connID)
}

// addSubscription adds a subscription for a connection
func (s *StompServer) addSubscription(destination, connID string, sub *Subscription) {
	s.subMutex.Lock()
	defer s.subMutex.Unlock()

	if s.subscriptions[destination] == nil {
		s.subscriptions[destination] = make(map[string]*Subscription)
	}
	s.subscriptions[destination][connID] = sub
}

// removeSubscription removes a subscription
func (s *StompServer) removeSubscription(destination, connID string) {
	s.subMutex.Lock()
	defer s.subMutex.Unlock()

	if subs, exists := s.subscriptions[destination]; exists {
		delete(subs, connID)
		if len(subs) == 0 {
			delete(s.subscriptions, destination)
		}
	}
}

// SendToDestination sends a message to all subscribers of a destination
func (s *StompServer) SendToDestination(destination string, body []byte, headers map[string]string) error {
	s.subMutex.RLock()
	subs, exists := s.subscriptions[destination]
	s.subMutex.RUnlock()

	if !exists {
		return nil
	}

	for connID, sub := range subs {
		s.mutex.RLock()
		conn, exists := s.connections[connID]
		s.mutex.RUnlock()

		if exists {
			messageFrame := &Frame{
				Command: "MESSAGE",
				Headers: make(map[string]string),
				Body:    body,
			}

			// Copy provided headers
			for k, v := range headers {
				messageFrame.Headers[k] = v
			}

			// Add required MESSAGE headers
			messageFrame.Headers["destination"] = destination
			messageFrame.Headers["subscription"] = sub.ID
			messageFrame.Headers["message-id"] = generateMessageID()

			conn.SendFrame(messageFrame)
		}
	}

	return nil
}

// generateMessageID generates a unique message ID
func generateMessageID() string {
	return fmt.Sprintf("msg-%d", time.Now().UnixNano())
}

// AddSubscriptionListener adds a subscription listener with pattern matching
// pattern: topic pattern with placeholders like "/topic/mission/{missionId}/log"
// callback: function to call when a matching subscription is made
func (s *StompServer) AddSubscriptionListener(pattern string, callback SubscriptionListenerCallback) error {
	// Convert pattern to regex and extract parameter names
	regexPattern, paramNames, err := s.convertPatternToRegex(pattern)
	if err != nil {
		return fmt.Errorf("invalid pattern '%s': %v", pattern, err)
	}

	// Compile regex
	regex, err := regexp.Compile(regexPattern)
	if err != nil {
		return fmt.Errorf("failed to compile regex for pattern '%s': %v", pattern, err)
	}

	listener := &SubscriptionListener{
		Pattern:    pattern,
		Regex:      regex,
		ParamNames: paramNames,
		Callback:   callback,
	}

	s.listenerMutex.Lock()
	s.subscriptionListeners = append(s.subscriptionListeners, listener)
	s.listenerMutex.Unlock()

	return nil
}

// convertPatternToRegex converts a pattern like "/topic/mission/{missionId}/log" to regex
func (s *StompServer) convertPatternToRegex(pattern string) (string, []string, error) {
	var paramNames []string
	regexPattern := pattern

	// Find all parameter placeholders like {paramName}
	paramRegex := regexp.MustCompile(`\{([^}]+)\}`)
	matches := paramRegex.FindAllStringSubmatch(pattern, -1)

	for _, match := range matches {
		if len(match) >= 2 {
			paramName := match[1]
			paramNames = append(paramNames, paramName)
			// Replace {paramName} with a capturing group that matches non-slash characters
			regexPattern = strings.Replace(regexPattern, match[0], "([^/]+)", 1)
		}
	}

	// Escape special regex characters except for our capturing groups
	regexPattern = "^" + regexPattern + "$"

	return regexPattern, paramNames, nil
}

// extractParameters extracts parameters from a topic using the listener's regex
func (s *StompServer) extractParameters(listener *SubscriptionListener, topic string) map[string]string {
	matches := listener.Regex.FindStringSubmatch(topic)
	if matches == nil {
		return nil
	}

	params := make(map[string]string)
	for i, paramName := range listener.ParamNames {
		if i+1 < len(matches) {
			params[paramName] = matches[i+1]
		}
	}

	return params
}

// triggerSubscriptionListeners triggers matching subscription listeners when a subscription is made
func (s *StompServer) triggerSubscriptionListeners(ctx context.Context, destination string) {
	s.listenerMutex.RLock()
	// 使用切片表达式共享底层数组，而不是创建完整副本，减少内存消耗
	listeners := s.subscriptionListeners[:]
	s.listenerMutex.RUnlock()

	for _, listener := range listeners {
		if params := s.extractParameters(listener, destination); params != nil {
			// Call the callback asynchronously
			go func(l *SubscriptionListener, topic string, p map[string]string) {
				defer func() {
					if r := recover(); r != nil {
						slog.Error("Subscription listener callback panic for pattern '%s': %v", l.Pattern, r)
					}
				}()
				l.Callback(context.Background(), topic, p)
			}(listener, destination, params)
		}
	}
}
