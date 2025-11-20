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

	// Add subscription listeners for drone logs
	err := server.AddSubscriptionListener("/topic/drone/{droneId}/log", func(ctx context.Context, topic string, params map[string]string) {
		droneId := params["droneId"]
		slog.Info("Drone %s subscribed to logs on topic: %s", droneId, topic)

		// Here you can implement your business logic
		// For example, start streaming logs for this specific drone
		startDroneLogStreaming(droneId)
	})
	if err != nil {
		slog.Error("Failed to add drone log listener: %v", err)
		return
	}

	// Add subscription listeners for drone status
	err = server.AddSubscriptionListener("/topic/drone/{droneId}/status", func(ctx context.Context, topic string, params map[string]string) {
		droneId := params["droneId"]
		slog.Info("Drone %s subscribed to status on topic: %s", droneId, topic)

		// Start streaming status for this specific drone
		startDroneStatusStreaming(droneId)
	})
	if err != nil {
		slog.Error("Failed to add drone status listener: %v", err)
		return
	}

	// Add subscription listeners for mission updates
	err = server.AddSubscriptionListener("/topic/mission/{missionId}/updates", func(ctx context.Context, topic string, params map[string]string) {
		missionId := params["missionId"]
		slog.Info("Mission %s subscribed to updates on topic: %s", missionId, topic)

		// Start streaming mission updates
		startMissionUpdateStreaming(missionId)
	})
	if err != nil {
		slog.Error("Failed to add mission update listener: %v", err)
		return
	}

	// Add subscription listeners for complex patterns with multiple parameters
	err = server.AddSubscriptionListener("/topic/fleet/{fleetId}/drone/{droneId}/telemetry", func(ctx context.Context, topic string, params map[string]string) {
		fleetId := params["fleetId"]
		droneId := params["droneId"]
		slog.Info("Fleet %s, Drone %s subscribed to telemetry on topic: %s", fleetId, droneId, topic)

		// Start streaming telemetry for this specific drone in the fleet
		startDroneTelemetryStreaming(fleetId, droneId)
	})
	if err != nil {
		slog.Error("Failed to add drone telemetry listener: %v", err)
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

// Example callback functions (implement your business logic here)
func startDroneLogStreaming(droneId string) {
	fmt.Printf("Starting log streaming for drone: %s\n", droneId)
	// Implement your log streaming logic here
}

func startDroneStatusStreaming(droneId string) {
	fmt.Printf("Starting status streaming for drone: %s\n", droneId)
	// Implement your status streaming logic here
}

func startMissionUpdateStreaming(missionId string) {
	fmt.Printf("Starting mission update streaming for mission: %s\n", missionId)
	// Implement your mission update streaming logic here
}

func startDroneTelemetryStreaming(fleetId, droneId string) {
	fmt.Printf("Starting telemetry streaming for fleet: %s, drone: %s\n", fleetId, droneId)
	// Implement your telemetry streaming logic here
}

/*
Example client subscriptions that would trigger the listeners:

1. Client subscribes to: /topic/drone/TL2025001/log
   - Triggers: startDroneLogStreaming("TL2025001")
   - Params: {"droneId": "TL2025001"}

2. Client subscribes to: /topic/drone/TL2025002/log
   - Triggers: startDroneLogStreaming("TL2025002")
   - Params: {"droneId": "TL2025002"}

3. Client subscribes to: /topic/drone/TL2025001/status
   - Triggers: startDroneStatusStreaming("TL2025001")
   - Params: {"droneId": "TL2025001"}

4. Client subscribes to: /topic/mission/MISSION001/updates
   - Triggers: startMissionUpdateStreaming("MISSION001")
   - Params: {"missionId": "MISSION001"}

5. Client subscribes to: /topic/fleet/FLEET01/drone/TL2025001/telemetry
   - Triggers: startDroneTelemetryStreaming("FLEET01", "TL2025001")
   - Params: {"fleetId": "FLEET01", "droneId": "TL2025001"}
*/
