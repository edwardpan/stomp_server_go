package stomp

import (
	"context"
	"fmt"
)

// handleSend processes SEND frame
func (c *Connection) handleSend(frame *Frame) error {
	destination := frame.Headers["destination"]
	if destination == "" {
		return fmt.Errorf("SEND frame missing destination header")
	}

	// Check if this is part of a transaction
	transactionID := frame.Headers["transaction"]
	if transactionID != "" {
		c.mutex.RLock()
		tx, exists := c.transactions[transactionID]
		c.mutex.RUnlock()

		if !exists {
			return fmt.Errorf("transaction %s not found", transactionID)
		}

		// Add to transaction instead of sending immediately
		tx.Messages = append(tx.Messages, frame)
		return nil
	}

	// Send immediately if not in transaction
	return c.sendMessage(frame)
}

// sendMessage actually sends the message to subscribers
func (c *Connection) sendMessage(frame *Frame) error {
	destination := frame.Headers["destination"]

	// Check if there's a custom handler for this destination
	c.server.mutex.RLock()
	handlers := make(map[string]*MessageHandlerListener)
	for k, v := range c.server.handlers {
		handlers[k] = v
	}
	c.server.mutex.RUnlock()

	for _, handler := range handlers {
		if params := c.extractParameters(handler, destination); params != nil {
			// // Call the callback asynchronously
			// go func(l *MessageHandlerListener, frame *Frame, p map[string]string) {
			// 	defer func() {
			// 		if r := recover(); r != nil {
			// 			g.Log().Errorf(gctx.GetInitCtx(), "Message Handler listener callback panic for pattern '%s': %v", l.Pattern, r)
			// 		}
			// 	}()
			// 	l.Handler(gctx.GetInitCtx(), c, frame)
			// }(handler, frame, params)

			return handler.Handler(context.Background(), c, frame, params)
		}
	}

	// Default behavior: send to all subscribers
	headers := make(map[string]string)
	for k, v := range frame.Headers {
		if k != "destination" && k != "transaction" {
			headers[k] = v
		}
	}

	return c.server.SendToDestination(destination, frame.Body, headers)
}

// extractParameters extracts parameters from a topic using the listener's regex
func (c *Connection) extractParameters(listener *MessageHandlerListener, topic string) map[string]string {
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

// handleSubscribe processes SUBSCRIBE frame
func (c *Connection) handleSubscribe(frame *Frame) error {
	destination := frame.Headers["destination"]
	if destination == "" {
		return fmt.Errorf("SUBSCRIBE frame missing destination header")
	}

	// Get subscription ID
	subID := frame.Headers["id"]
	if subID == "" {
		// For STOMP 1.0, id is optional, generate one
		if c.version == "1.0" {
			subID = fmt.Sprintf("sub-%s-%d", c.ID, len(c.subscriptions))
		} else {
			return fmt.Errorf("SUBSCRIBE frame missing id header (required for STOMP %s)", c.version)
		}
	}

	// Check if subscription ID already exists
	c.mutex.RLock()
	_, exists := c.subscriptions[subID]
	c.mutex.RUnlock()

	if exists {
		return fmt.Errorf("subscription with id %s already exists", subID)
	}

	// Get ack mode
	ackMode := frame.Headers["ack"]
	if ackMode == "" {
		ackMode = "auto" // Default
	}

	// Validate ack mode
	validAckModes := map[string]bool{
		"auto":              true,
		"client":            true,
		"client-individual": true,
	}

	if !validAckModes[ackMode] {
		return fmt.Errorf("invalid ack mode: %s", ackMode)
	}

	// Create subscription
	sub := &Subscription{
		ID:          subID,
		Destination: destination,
		Ack:         ackMode,
		Selector:    frame.Headers["selector"],
	}

	// Add to connection subscriptions
	c.mutex.Lock()
	c.subscriptions[subID] = sub
	c.mutex.Unlock()

	// Add to server subscriptions
	c.server.addSubscription(destination, c.ID, sub)

	// Trigger subscription listeners after successful subscription
	c.server.triggerSubscriptionListeners(c.ctx, destination)

	// Send receipt if requested
	if receiptID := frame.Headers["receipt"]; receiptID != "" {
		receiptFrame := &Frame{
			Command: "RECEIPT",
			Headers: map[string]string{
				"receipt-id": receiptID,
			},
		}
		return c.SendFrame(receiptFrame)
	}

	return nil
}

// handleUnsubscribe processes UNSUBSCRIBE frame
func (c *Connection) handleUnsubscribe(frame *Frame) error {
	// For STOMP 1.1+, id is required. For 1.0, destination can be used
	subID := frame.Headers["id"]
	destination := frame.Headers["destination"]

	if subID == "" && destination == "" {
		return fmt.Errorf("UNSUBSCRIBE frame missing both id and destination headers")
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	var subToRemove *Subscription
	var keyToRemove string

	if subID != "" {
		// Find by subscription ID
		if sub, exists := c.subscriptions[subID]; exists {
			subToRemove = sub
			keyToRemove = subID
		}
	} else {
		// Find by destination (STOMP 1.0 compatibility)
		for id, sub := range c.subscriptions {
			if sub.Destination == destination {
				subToRemove = sub
				keyToRemove = id
				break
			}
		}
	}

	if subToRemove == nil {
		return fmt.Errorf("subscription not found")
	}

	// Remove from connection subscriptions
	delete(c.subscriptions, keyToRemove)

	// Remove from server subscriptions
	c.server.removeSubscription(subToRemove.Destination, c.ID)

	// Send receipt if requested
	if receiptID := frame.Headers["receipt"]; receiptID != "" {
		receiptFrame := &Frame{
			Command: "RECEIPT",
			Headers: map[string]string{
				"receipt-id": receiptID,
			},
		}
		return c.SendFrame(receiptFrame)
	}

	return nil
}

// handleAck processes ACK frame
func (c *Connection) handleAck(frame *Frame) error {
	// ACK handling depends on STOMP version
	var messageID string

	if c.version == "1.2" {
		// STOMP 1.2 uses 'id' header
		messageID = frame.Headers["id"]
	} else {
		// STOMP 1.0/1.1 uses 'message-id' header
		messageID = frame.Headers["message-id"]
	}

	if messageID == "" {
		return fmt.Errorf("ACK frame missing message identifier")
	}

	// In a real implementation, you would track pending messages
	// and mark them as acknowledged here
	// For now, we just log the acknowledgment
	fmt.Printf("Message %s acknowledged by connection %s\n", messageID, c.ID)

	return nil
}

// handleNack processes NACK frame (STOMP 1.1+)
func (c *Connection) handleNack(frame *Frame) error {
	if c.version == "1.0" {
		return fmt.Errorf("NACK not supported in STOMP 1.0")
	}

	var messageID string

	if c.version == "1.2" {
		// STOMP 1.2 uses 'id' header
		messageID = frame.Headers["id"]
	} else {
		// STOMP 1.1 uses 'message-id' header
		messageID = frame.Headers["message-id"]
	}

	if messageID == "" {
		return fmt.Errorf("NACK frame missing message identifier")
	}

	// In a real implementation, you would handle message rejection here
	// For now, we just log the negative acknowledgment
	fmt.Printf("Message %s rejected by connection %s\n", messageID, c.ID)

	return nil
}

// handleBegin processes BEGIN frame
func (c *Connection) handleBegin(frame *Frame) error {
	transactionID := frame.Headers["transaction"]
	if transactionID == "" {
		return fmt.Errorf("BEGIN frame missing transaction header")
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Check if transaction already exists
	if _, exists := c.transactions[transactionID]; exists {
		return fmt.Errorf("transaction %s already exists", transactionID)
	}

	// Create new transaction
	c.transactions[transactionID] = &Transaction{
		ID:       transactionID,
		Messages: make([]*Frame, 0),
	}

	// Send receipt if requested
	if receiptID := frame.Headers["receipt"]; receiptID != "" {
		receiptFrame := &Frame{
			Command: "RECEIPT",
			Headers: map[string]string{
				"receipt-id": receiptID,
			},
		}
		return c.SendFrame(receiptFrame)
	}

	return nil
}

// handleCommit processes COMMIT frame
func (c *Connection) handleCommit(frame *Frame) error {
	transactionID := frame.Headers["transaction"]
	if transactionID == "" {
		return fmt.Errorf("COMMIT frame missing transaction header")
	}

	c.mutex.Lock()
	tx, exists := c.transactions[transactionID]
	if exists {
		delete(c.transactions, transactionID)
	}
	c.mutex.Unlock()

	if !exists {
		return fmt.Errorf("transaction %s not found", transactionID)
	}

	// Send all messages in the transaction
	for _, msg := range tx.Messages {
		if err := c.sendMessage(msg); err != nil {
			return fmt.Errorf("failed to send message in transaction: %v", err)
		}
	}

	// Send receipt if requested
	if receiptID := frame.Headers["receipt"]; receiptID != "" {
		receiptFrame := &Frame{
			Command: "RECEIPT",
			Headers: map[string]string{
				"receipt-id": receiptID,
			},
		}
		return c.SendFrame(receiptFrame)
	}

	return nil
}

// handleAbort processes ABORT frame
func (c *Connection) handleAbort(frame *Frame) error {
	transactionID := frame.Headers["transaction"]
	if transactionID == "" {
		return fmt.Errorf("ABORT frame missing transaction header")
	}

	c.mutex.Lock()
	_, exists := c.transactions[transactionID]
	if exists {
		delete(c.transactions, transactionID)
	}
	c.mutex.Unlock()

	if !exists {
		return fmt.Errorf("transaction %s not found", transactionID)
	}

	// Transaction is simply discarded

	// Send receipt if requested
	if receiptID := frame.Headers["receipt"]; receiptID != "" {
		receiptFrame := &Frame{
			Command: "RECEIPT",
			Headers: map[string]string{
				"receipt-id": receiptID,
			},
		}
		return c.SendFrame(receiptFrame)
	}

	return nil
}

// handleDisconnect processes DISCONNECT frame
func (c *Connection) handleDisconnect(frame *Frame) error {
	// Send receipt if requested
	if receiptID := frame.Headers["receipt"]; receiptID != "" {
		receiptFrame := &Frame{
			Command: "RECEIPT",
			Headers: map[string]string{
				"receipt-id": receiptID,
			},
		}
		c.SendFrame(receiptFrame)
	}

	// Close the connection
	c.close(context.Background())
	return nil
}
