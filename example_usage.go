package stomp

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gorilla/websocket"
)

// ExampleUsage demonstrates how to use the subscription listener functionality
func ExampleUsage() {
	ctx := context.Background()

	// Create WebSocket upgrader
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins in this example
		},
	}

	// Create STOMP server
	server := NewStompServer(upgrader)

	// Add subscription listeners for mission updates
	err := server.AddSubscriptionListener("/topic/mission/{missionId}/updates", func(ctx context.Context, topic string, params map[string]string) {
		missionId := params["missionId"]
		slog.Info("Mission %s subscribed to updates on topic: %s", missionId, topic)

		// Start streaming mission updates
		startMissionUpdateStreaming(missionId)
	})
	if err != nil {
		slog.Error("Failed to add mission update listener: %v", err)
		return
	}

	// Start HTTP server
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		server.HandleWebSocket(ctx, w, r)
	})
	slog.Info("STOMP WebSocket server starting on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		slog.Error("Server failed to start: %v", err)
	}
}

func startMissionUpdateStreaming(missionId string) {
	fmt.Printf("Starting mission update streaming for mission: %s\n", missionId)
	// Implement your mission update streaming logic here
}
