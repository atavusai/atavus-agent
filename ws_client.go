package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

// WSClient manages the WebSocket connection to the Atavus backend
type WSClient struct {
	serverURL  string
	authToken  string
	deviceID   string
	deviceName string
	sandbox    *Sandbox
	conn       *websocket.Conn
	done       chan struct{}
	configPath string // path to config file (for clearing credentials on auth fail)
	onAuthFail func()  // callback when auth fails (stale token)
}

// NewWSClient creates a new WebSocket client
func NewWSClient(serverURL, authToken string, sandbox *Sandbox) *WSClient {
	return &WSClient{
		serverURL: serverURL,
		authToken: authToken,
		sandbox:   sandbox,
		done:      make(chan struct{}),
	}
}

// WsMessage represents a message from/to the backend
type WsMessage struct {
	Type   string          `json:"type"`
	ID     string          `json:"id,omitempty"`
	Action string          `json:"action,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Data   json.RawMessage `json:"data,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// ExecuteParams holds parameters for an execute command
type ExecuteParams struct {
	Action string          `json:"action"`
	Params json.RawMessage `json:"params"`
}

// Connect establishes the WebSocket connection and handles messages
func (c *WSClient) Connect() error {
	log.Printf("Connecting to %s...", c.serverURL)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	// Token is sent in the auth message, not as query param
	conn, httpResp, err := dialer.Dial(c.serverURL, nil)
	if err != nil {
		if httpResp != nil {
			return fmt.Errorf("dial error: server returned HTTP %d - %w", httpResp.StatusCode, err)
		}
		return fmt.Errorf("dial error (no response): %w", err)
	}
	c.conn = conn
	log.Println("Connected to Atavus")

	// Send auth message — flat JSON, backend reads token at top level
	authPayload, _ := json.Marshal(map[string]string{
		"type":        "auth",
		"token":       c.authToken,
		"device_id":   c.deviceID,
		"device_name": c.deviceName,
	})
	err = c.conn.WriteMessage(websocket.TextMessage, authPayload)
	if err != nil {
		return fmt.Errorf("auth send error: %w", err)
	}
	log.Println("Auth message sent")

	// Start heartbeat
	go c.heartbeat()

	// Read messages
	go c.readLoop()

	return nil
}

// Stop closes the connection
func (c *WSClient) Stop() {
	select {
	case <-c.done:
		return
	default:
		close(c.done)
	}
	if c.conn != nil {
		c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		c.conn.Close()
	}
}

// send writes a message to the WebSocket
func (c *WSClient) send(msg WsMessage) {
	if c.conn == nil {
		return
	}
	data, _ := json.Marshal(msg)
	err := c.conn.WriteMessage(websocket.TextMessage, data)
	if err != nil {
		log.Printf("Send error: %v", err)
	}
}

// heartbeat sends pings every 30 seconds
func (c *WSClient) heartbeat() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			ping := WsMessage{Type: "heartbeat"}
			c.send(ping)
			log.Println("Heartbeat sent")
		}
	}
}

// readLoop reads and processes incoming messages
func (c *WSClient) readLoop() {
	defer func() {
		log.Println("Read loop ended")
		c.reconnect()
	}()

	for {
		select {
		case <-c.done:
			return
		default:
			_, message, err := c.conn.ReadMessage()
			if err != nil {
				log.Printf("Read error: %v", err)
				return
			}

			var msg WsMessage
			if err := json.Unmarshal(message, &msg); err != nil {
				log.Printf("Parse error: %v", err)
				continue
			}

			c.handleMessage(msg)
		}
	}
}

// handleMessage routes incoming messages to the right handler
func (c *WSClient) handleMessage(msg WsMessage) {
	log.Printf("Received: type=%s action=%s id=%s", msg.Type, msg.Action, msg.ID)

	switch msg.Type {
	case "execute":
		c.handleExecute(msg)
	case "auth_ok":
		log.Println("✅ Authentication verified by server")
	case "auth_error":
		log.Printf("❌ Authentication failed: %s", msg.Error)
		// Device was deleted from dashboard — clear credentials and stop
		c.handleAuthFailure()
	case "disconnect":
		log.Println("Server requested disconnect")
		c.Stop()
	case "ping":
		c.send(WsMessage{Type: "pong"})
	case "error":
		log.Printf("Server error: %s", msg.Error)
	default:
		log.Printf("Unknown message type: %s", msg.Type)
	}
}

// handleAuthFailure clears stored credentials when auth fails (device deleted)
func (c *WSClient) handleAuthFailure() {
	c.Stop()
	// Clear saved token so reconnect won't reuse stale credentials
	if c.configPath != "" {
		cfg := DefaultConfig()
		cfg.Save(c.configPath)
		log.Println("Cleared saved credentials (device was removed from dashboard)")
	}
	fmt.Println()
	fmt.Println("╔════════════════════════════════════════╗")
	fmt.Println("║     Device removed from dashboard      ║")
	fmt.Println("╚════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("This device was unlinked from your Atavus account.")
	fmt.Println("Go to https://atavus.ai/devices to generate a new pairing code.")
	fmt.Println()
	if c.onAuthFail != nil {
		c.onAuthFail()
	}
	os.Exit(1)
}

// handleExecute processes an execute command from the server
func (c *WSClient) handleExecute(msg WsMessage) {
	startTime := time.Now()
	result := WsMessage{
		Type: "result",
		ID:   msg.ID,
	}

	var response interface{}
	var execErr string

	switch msg.Action {
	case "list_files":
		var params struct {
			Path string `json:"path"`
		}
		json.Unmarshal(msg.Params, &params)
		if params.Path == "" {
			params.Path = c.sandbox.GetSafeDirectory()
		}
		response, execErr = handleListFiles(params.Path, c.sandbox)

	case "read_file":
		var params struct {
			Path string `json:"path"`
		}
		json.Unmarshal(msg.Params, &params)
		response, execErr = handleReadFile(params.Path, c.sandbox)

	case "write_file":
		var params struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		json.Unmarshal(msg.Params, &params)
		response, execErr = handleWriteFile(params.Path, params.Content, c.sandbox)

	case "move_file":
		var params struct {
			Source      string `json:"source"`
			Destination string `json:"destination"`
		}
		json.Unmarshal(msg.Params, &params)
		response, execErr = handleMoveFile(params.Source, params.Destination, c.sandbox)

	case "copy_file":
		var params struct {
			Source      string `json:"source"`
			Destination string `json:"destination"`
		}
		json.Unmarshal(msg.Params, &params)
		response, execErr = handleCopyFile(params.Source, params.Destination, c.sandbox)

	case "delete_file":
		var params struct {
			Path string `json:"path"`
		}
		json.Unmarshal(msg.Params, &params)
		response, execErr = handleDeleteFile(params.Path, c.sandbox)

	case "create_folder":
		var params struct {
			Path string `json:"path"`
		}
		json.Unmarshal(msg.Params, &params)
		response, execErr = handleCreateFolder(params.Path, c.sandbox)

	case "search_files":
		var params struct {
			Pattern string `json:"pattern"`
			Root    string `json:"root"`
		}
		json.Unmarshal(msg.Params, &params)
		if params.Root == "" {
			params.Root = c.sandbox.GetSafeDirectory()
		}
		response, execErr = handleSearchFiles(params.Pattern, params.Root, c.sandbox)

	case "empty_trash":
		response, execErr = handleEmptyTrash()

	case "get_system_info":
		response, execErr = handleGetSystemInfo()

	case "device_info":
		response = map[string]string{
			"device_id":   c.deviceID,
			"device_name": c.deviceName,
			"platform":    detectPlatform(),
		}

	default:
		execErr = fmt.Sprintf("unknown action: %s", msg.Action)
	}

	// Build response
	durationMs := time.Since(startTime).Milliseconds()
	result.Data = mustMarshal(map[string]interface{}{
		"duration_ms": durationMs,
		"action":      msg.Action,
	})

	if execErr != "" {
		result.Type = "error"
		result.Error = execErr
	} else {
		result.Data = mustMarshal(response)
	}

	c.send(result)
	log.Printf("Executed %s in %dms", msg.Action, durationMs)
}

// reconnect attempts to reconnect with exponential backoff
func (c *WSClient) reconnect() {
	backoff := 5 * time.Second
	maxBackoff := 60 * time.Second

	for {
		select {
		case <-c.done:
			return
		default:
			log.Printf("Reconnecting in %.0f seconds...", backoff.Seconds())
			time.Sleep(backoff)

			err := c.Connect()
			if err == nil {
				log.Println("Reconnected successfully")
				return
			}

			log.Printf("Reconnect failed: %v", err)

			// Exponential backoff
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

func mustMarshal(v interface{}) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}
