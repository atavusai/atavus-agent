package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

func main() {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	// Test via Apache (wss://)
	url := "wss://atavus.ai/api/v1/device-manage/ws"
	fmt.Printf("Connecting to %s...\n", url)
	conn, resp, err := dialer.Dial(url, nil)
	if err != nil {
		fmt.Printf("Apache WebSocket failed: %v\n", err)
		if resp != nil {
			fmt.Printf("   HTTP status: %d\n", resp.StatusCode)
		}
	} else {
		fmt.Println("Apache WebSocket connected!")
		conn.Close()
	}

	// Test direct (ws://localhost)
	url2 := "ws://127.0.0.1:8000/api/v1/device-manage/ws"
	fmt.Printf("\nConnecting to %s...\n", url2)
	conn2, resp2, err2 := dialer.Dial(url2, nil)
	if err2 != nil {
		fmt.Printf("Direct WebSocket failed: %v\n", err2)
		if resp2 != nil {
			fmt.Printf("   HTTP status: %d\n", resp2.StatusCode)
		}
		os.Exit(1)
	} else {
		fmt.Println("Direct WebSocket connected!")
		// Send auth
		authMsg := map[string]string{"type": "auth", "token": "test"}
		data, _ := json.Marshal(authMsg)
		conn2.WriteMessage(websocket.TextMessage, data)
		conn2.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, msg, readErr := conn2.ReadMessage()
		if readErr != nil {
			fmt.Printf("   Read error: %v\n", readErr)
		} else {
			fmt.Printf("   Server response: %s\n", string(msg))
		}
		conn2.Close()
	}
}
