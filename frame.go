package stomp

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// Frame represents a STOMP frame
type Frame struct {
	Command string
	Headers map[string]string
	Body    []byte
}

// NewFrame creates a new STOMP frame
func NewFrame(command string) *Frame {
	return &Frame{
		Command: command,
		Headers: make(map[string]string),
		Body:    nil,
	}
}

// ParseFrame parses a STOMP frame from raw bytes
func ParseFrame(data []byte) (*Frame, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty frame data")
	}

	// Remove trailing null byte if present
	if data[len(data)-1] == 0 {
		data = data[:len(data)-1]
	}

	// Split into lines
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("no lines in frame")
	}

	// First line is the command
	command := strings.TrimSpace(lines[0])
	if command == "" {
		return nil, fmt.Errorf("empty command")
	}

	frame := &Frame{
		Command: command,
		Headers: make(map[string]string),
	}

	// Parse headers
	headerEndIndex := 1
	for i := 1; i < len(lines); i++ {
		line := lines[i]
		if line == "" {
			// Empty line indicates end of headers
			headerEndIndex = i + 1
			break
		}

		// Parse header line
		colonIndex := strings.Index(line, ":")
		if colonIndex == -1 {
			return nil, fmt.Errorf("invalid header line: %s", line)
		}

		key := strings.TrimSpace(line[:colonIndex])
		value := ""
		if colonIndex+1 < len(line) {
			value = line[colonIndex+1:] // Don't trim value to preserve spaces
		}

		// Handle header escaping for STOMP 1.1+
		key = unescapeHeaderValue(key)
		value = unescapeHeaderValue(value)

		frame.Headers[key] = value
	}

	// Parse body
	if headerEndIndex < len(lines) {
		body := strings.Join(lines[headerEndIndex:], "\n")
		frame.Body = []byte(body)
	}

	// Handle content-length header
	if contentLengthStr, exists := frame.Headers["content-length"]; exists {
		contentLength, err := strconv.Atoi(contentLengthStr)
		if err != nil {
			return nil, fmt.Errorf("invalid content-length: %s", contentLengthStr)
		}

		// Recalculate body based on content-length
		bodyStart := headerEndIndex
		if bodyStart < len(lines) {
			fullBody := strings.Join(lines[bodyStart:], "\n")
			if len(fullBody) >= contentLength {
				frame.Body = []byte(fullBody[:contentLength])
			} else {
				return nil, fmt.Errorf("body length %d is less than content-length %d", len(fullBody), contentLength)
			}
		}
	}

	return frame, nil
}

// Marshal serializes the frame to bytes
func (f *Frame) Marshal() []byte {
	var buffer bytes.Buffer

	// Write command
	buffer.WriteString(f.Command)
	buffer.WriteString("\n")

	// Write headers
	for key, value := range f.Headers {
		// Escape header key and value for STOMP 1.1+
		escapedKey := escapeHeaderValue(key)
		escapedValue := escapeHeaderValue(value)

		buffer.WriteString(escapedKey)
		buffer.WriteString(":")
		buffer.WriteString(escapedValue)
		buffer.WriteString("\n")
	}

	// Add content-length header if body exists and not already set
	if len(f.Body) > 0 {
		if _, exists := f.Headers["content-length"]; !exists {
			buffer.WriteString("content-length:")
			buffer.WriteString(strconv.Itoa(len(f.Body)))
			buffer.WriteString("\n")
		}
	}

	// Empty line to separate headers from body
	buffer.WriteString("\n")

	// Write body
	if len(f.Body) > 0 {
		buffer.Write(f.Body)
	}

	// Null terminator
	buffer.WriteByte(0)

	return buffer.Bytes()
}

// SetHeader sets a header value
func (f *Frame) SetHeader(key, value string) {
	f.Headers[key] = value
}

// GetHeader gets a header value
func (f *Frame) GetHeader(key string) (string, bool) {
	value, exists := f.Headers[key]
	return value, exists
}

// SetBody sets the frame body
func (f *Frame) SetBody(body []byte) {
	f.Body = body
}

// GetBody gets the frame body
func (f *Frame) GetBody() []byte {
	return f.Body
}

// String returns a string representation of the frame
func (f *Frame) String() string {
	var buffer bytes.Buffer

	buffer.WriteString(fmt.Sprintf("Command: %s\n", f.Command))
	buffer.WriteString("Headers:\n")
	for key, value := range f.Headers {
		buffer.WriteString(fmt.Sprintf("  %s: %s\n", key, value))
	}

	if len(f.Body) > 0 {
		buffer.WriteString(fmt.Sprintf("Body (%d bytes): %s\n", len(f.Body), string(f.Body)))
	} else {
		buffer.WriteString("Body: (empty)\n")
	}

	return buffer.String()
}

// escapeHeaderValue escapes header values according to STOMP 1.1+ specification
func escapeHeaderValue(value string) string {
	// STOMP 1.1+ header escaping:
	// \r -> \\r
	// \n -> \\n
	// \c -> \\c
	// \\ -> \\\\
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\r", "\\r")
	value = strings.ReplaceAll(value, "\n", "\\n")
	value = strings.ReplaceAll(value, ":", "\\c")
	return value
}

// unescapeHeaderValue unescapes header values according to STOMP 1.1+ specification
func unescapeHeaderValue(value string) string {
	// Reverse of escapeHeaderValue
	value = strings.ReplaceAll(value, "\\c", ":")
	value = strings.ReplaceAll(value, "\\n", "\n")
	value = strings.ReplaceAll(value, "\\r", "\r")
	value = strings.ReplaceAll(value, "\\\\", "\\")
	return value
}

// Validate validates the frame according to STOMP specification
func (f *Frame) Validate() error {
	if f.Command == "" {
		return fmt.Errorf("frame command cannot be empty")
	}

	// Validate required headers for specific commands
	switch f.Command {
	case "CONNECT", "STOMP":
		// No required headers for CONNECT/STOMP
	case "SEND":
		if _, exists := f.Headers["destination"]; !exists {
			return fmt.Errorf("SEND frame missing destination header")
		}
	case "SUBSCRIBE":
		if _, exists := f.Headers["destination"]; !exists {
			return fmt.Errorf("SUBSCRIBE frame missing destination header")
		}
	case "UNSUBSCRIBE":
		// Must have either id or destination
		if _, hasID := f.Headers["id"]; !hasID {
			if _, hasDest := f.Headers["destination"]; !hasDest {
				return fmt.Errorf("UNSUBSCRIBE frame missing both id and destination headers")
			}
		}
	case "ACK", "NACK":
		// Must have message-id or id (depending on version)
		if _, hasMessageID := f.Headers["message-id"]; !hasMessageID {
			if _, hasID := f.Headers["id"]; !hasID {
				return fmt.Errorf("%s frame missing message identifier", f.Command)
			}
		}
	case "BEGIN", "COMMIT", "ABORT":
		if _, exists := f.Headers["transaction"]; !exists {
			return fmt.Errorf("%s frame missing transaction header", f.Command)
		}
	}

	return nil
}
