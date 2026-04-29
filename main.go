package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

var platform string
var version = "1.0.0"

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	configDir := getConfigDir()
	os.MkdirAll(configDir, 0755)

	configPath := filepath.Join(configDir, "atavus-agent.json")
	logPath := filepath.Join(configDir, "atavus-agent.log")

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		log.SetOutput(logFile)
		defer logFile.Close()
	}

	log.Println("=== Atavus Agent Starting ===")

	cfg := LoadConfig(configPath)

	if len(os.Args) < 2 {
		runSetupMenu(cfg, configPath)
		return
	}

	switch os.Args[1] {
	case "pair":
		pairInteractive(cfg, configPath)
	case "connect":
		if cfg.AuthToken == "" {
			fmt.Println("❌ Not paired. Run 'atavus-agent pair' first.")
			os.Exit(1)
		}
		if len(os.Args) > 2 && os.Args[2] == "--autostart" {
			// Running from startup — no stdout output
			runAgentSilent(cfg, configPath)
		} else {
			runAgent(cfg, configPath)
		}
	case "status":
		showStatus(cfg)
	case "disconnect":
		disconnect(cfg, configPath)
	case "uninstall":
		uninstall(cfg, configPath)
	case "version":
		fmt.Println("Atavus Agent v" + version)
		fmt.Println("Platform:", detectPlatform())
	case "startup":
		addToStartup()
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Printf("Unknown: %s\n", os.Args[1])
		printUsage()
	}
}

func runSetupMenu(cfg *Config, configPath string) {
	fmt.Println(`
╔════════════════════════════════════════╗
║           Atavus AI Agent             ║
║         v` + version + ` (` + detectPlatform() + `)             ║
╚════════════════════════════════════════╝`)

	if cfg.AuthToken != "" {
		fmt.Printf("\n✅ Already paired as: %s\n", cfg.DeviceName)
		fmt.Printf("   Device ID: %s\n\n", cfg.DeviceID)
		fmt.Println("Starting connection in 3 seconds...")
		time.Sleep(3 * time.Second)
		runAgent(cfg, configPath)
		return
	}

	fmt.Println(`
This agent connects your computer to Atavus AI so your
AI assistants can manage files and folders on this PC.

To get started:`)
	fmt.Println()
	fmt.Println("  1. Open https://atavus.ai/devices in your browser")
	fmt.Println("  2. Click 'Connect a PC' and enter a name")
	fmt.Println("  3. Get a 6-digit pairing code")
	fmt.Println()
	fmt.Println("Enter pairing code below, or type 'help' for options.")
	fmt.Println()

	for {
		fmt.Print("▶ Code (6 digits): ")
		var code string
		fmt.Scanln(&code)
		code = strings.TrimSpace(code)

		switch code {
		case "exit", "quit":
			fmt.Println("Goodbye.")
			return
		case "help", "?":
			fmt.Println()
			fmt.Println("  Type a 6-digit code → pair this computer")
			fmt.Println("  Type 'new'         → show how to generate a code")
			fmt.Println("  Type 'exit'        → quit")
			fmt.Println()
			continue
		case "new":
			fmt.Println()
			fmt.Println("  To generate a new pairing code:")
			fmt.Println("  1. Open https://atavus.ai/devices in your browser")
			fmt.Println("  2. Click 'Connect a PC' → enter a name → click 'Generate Pairing Code'")
			fmt.Println("  3. Type the 6-digit code below within 5 minutes")
			fmt.Println()
			continue
		}

		if len(code) != 6 {
			fmt.Println("❌ Must be exactly 6 digits. Type 'help' for options.")
			continue
		}

		serverURL := cfg.ServerURL
		if serverURL == "" {
			serverURL = "https://atavus.ai"
		}

		deviceName := cfg.DeviceName
		if deviceName == "" {
			hostname, _ := os.Hostname()
			deviceName = hostname
		}

		fmt.Print("\n🔐 Pairing with server...")
		result, err := pairDevice(serverURL, code, deviceName, detectPlatform())
		if err != nil {
			fmt.Printf("\n❌ Pairing failed: %v\n", err)
			fmt.Println()
			fmt.Println("  🔄 Go to https://atavus.ai/devices and click 'Connect a PC'")
			fmt.Println("     to get a fresh 6-digit code. It expires in 5 minutes.")
			fmt.Println()
			continue
		}

		cfg.AuthToken = result.AuthToken
		cfg.DeviceID = result.DeviceID
		cfg.DeviceName = deviceName
		cfg.Save(configPath)

		fmt.Println(" ✅")
		fmt.Printf("\n✅ Paired successfully! Device: %s\n\n", deviceName)
		fmt.Println("Connecting in 3 seconds...")
		time.Sleep(3 * time.Second)
		runAgent(cfg, configPath)
		return
	}
}

func printUsage() {
	fmt.Println(`Atavus Agent - Connect your PC to Atavus AI

USAGE:
  atavus-agent pair              Start pairing flow
  atavus-agent connect           Run agent (foreground)
  atavus-agent status            Show connection status
  atavus-agent disconnect        Clear pairing
  atavus-agent uninstall         Remove agent + config
  atavus-agent startup           Add to OS autostart
  atavus-agent version           Show version
`)
}

func runAgent(cfg *Config, configPath string) {
	log.Println("Starting agent...")

	wsURL := cfg.ServerURL
	if wsURL == "" {
		wsURL = "https://atavus.ai"
	}
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
	wsURL = strings.Replace(wsURL, "https://", "wss://", 1)
	// Always append the WebSocket endpoint path
	wsURL = strings.TrimRight(wsURL, "/") + "/api/v1/device-manage/ws"

	sandbox := NewSandbox()
	client := NewWSClient(wsURL, cfg.AuthToken, sandbox)
	client.deviceID = cfg.DeviceID
	client.deviceName = cfg.DeviceName
	client.configPath = configPath
	client.onAuthFail = func() {
		fmt.Println("Press Enter to restart pairing...")
		fmt.Scanln()
		runSetupMenu(cfg, configPath)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down...")
		client.Stop()
		fmt.Println("\nAtavus Agent stopped.")
		os.Exit(0)
	}()

	fmt.Println("✅ Atavus Agent connected. Press Ctrl+C to stop.")
	err := client.Connect()
	if err != nil {
		log.Printf("Connection error: %v", err)
		fmt.Printf("❌ Connection failed: %v\n", err)
		os.Exit(1)
	}
}

func runAgentSilent(cfg *Config, configPath string) {
	log.Println("Starting agent (autostart mode)...")
	wsURL := cfg.ServerURL
	if wsURL == "" {
		wsURL = "wss://atavus.ai/api/v1/device-manage/ws"
	}
	sandbox := NewSandbox()
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
	wsURL = strings.Replace(wsURL, "https://", "wss://", 1)
	wsURL = strings.TrimRight(wsURL, "/") + "/api/v1/device-manage/ws"
	client := NewWSClient(wsURL, cfg.AuthToken, sandbox)
	client.deviceID = cfg.DeviceID
	client.deviceName = cfg.DeviceName
	client.configPath = configPath
	client.onAuthFail = func() {
		fmt.Println("Press Enter to restart pairing...")
		fmt.Scanln()
		runSetupMenu(cfg, configPath)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		client.Stop()
		os.Exit(0)
	}()

	err := client.Connect()
	if err != nil {
		log.Printf("Connection error: %v", err)
	}
}

func pairInteractive(cfg *Config, configPath string) {
	fmt.Println("═══ Atavus AI - Device Pairing ═══")
	fmt.Println()
	fmt.Println("1. Go to https://atavus.ai/devices in your browser")
	fmt.Println("2. Click 'Connect a PC' and enter a name for this computer")
	fmt.Println("3. A 6-digit code will appear")
	fmt.Println()
	fmt.Print("Enter the 6-digit pairing code: ")

	var code string
	fmt.Scanln(&code)
	code = strings.TrimSpace(code)

	if len(code) != 6 {
		fmt.Println("❌ Code must be exactly 6 digits")
		os.Exit(1)
	}

	serverURL := cfg.ServerURL
	if serverURL == "" {
		serverURL = "https://atavus.ai"
	}

	deviceName := cfg.DeviceName
	if deviceName == "" {
		hostname, _ := os.Hostname()
		deviceName = hostname
	}

	result, err := pairDevice(serverURL, code, deviceName, detectPlatform())
	if err != nil {
		fmt.Printf("❌ Pairing failed: %v\n", err)
		os.Exit(1)
	}

	cfg.AuthToken = result.AuthToken
	cfg.DeviceID = result.DeviceID
	cfg.DeviceName = deviceName
	cfg.Save(configPath)

	fmt.Printf(`
✅ Paired successfully!

  Device: %s
  Server: %s

Next step: run 'atavus-agent connect'
`, deviceName, serverURL)
}

func showStatus(cfg *Config) {
	if cfg.AuthToken == "" {
		fmt.Println("❌ Not paired. Run 'atavus-agent pair'.")
		return
	}
	fmt.Println("═══ Atavus Agent Status ═══")
	fmt.Printf("✅ Paired\n")
	fmt.Printf("  Device:  %s\n", cfg.DeviceName)
	fmt.Printf("  ID:      %s\n", cfg.DeviceID)
	fmt.Printf("  Server:  %s\n", cfg.ServerURL)
	if cfg.ServerURL == "" {
		fmt.Println("  Server:  https://atavus.ai")
	}
	fmt.Println()
	fmt.Println("Run 'atavus-agent connect' to connect.")
}

func disconnect(cfg *Config, configPath string) {
	if cfg.AuthToken == "" {
		fmt.Println("Not paired.")
		return
	}
	notifyDisconnect(cfg)
	cfg.AuthToken = ""
	cfg.DeviceID = ""
	cfg.DeviceName = ""
	cfg.Save(configPath)
	fmt.Println("✅ Disconnected.")
}

func uninstall(cfg *Config, configPath string) {
	disconnect(cfg, configPath)
	os.Remove(configPath)
	os.Remove(filepath.Join(getConfigDir(), "atavus-agent.log"))
	removeFromStartup()
	fmt.Println("✅ Uninstalled.")
}

func addToStartup() {
	exe, err := os.Executable()
	if err != nil {
		fmt.Println("❌ Can't determine executable path")
		return
	}

	if detectPlatform() == "windows" {
		addWindowsStartup(exe)
	} else {
		addMacOSLaunchAgent(exe)
	}
	fmt.Println("✅ Added to startup.")
}

func removeFromStartup() {
	if detectPlatform() == "windows" {
		removeWindowsStartup()
	} else {
		removeMacOSLaunchAgent()
	}
}

func getConfigDir() string {
	if detectPlatform() == "windows" {
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming")
		}
		return filepath.Join(appData, "Atavus")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".atavus")
}

func detectPlatform() string {
	return platform
}

// ── Platform-specific startup ──

func addWindowsStartup(exe string) {
	cmd := fmt.Sprintf(`reg add "HKCU\Software\Microsoft\Windows\CurrentVersion\Run" /v "AtavusAgent" /t REG_SZ /d "\"%s\" connect --autostart" /f`, exe)
	runCmd(cmd)
}

func removeWindowsStartup() {
	runCmd(`reg delete "HKCU\Software\Microsoft\Windows\CurrentVersion\Run" /v "AtavusAgent" /f 2>nul`)
}

func addMacOSLaunchAgent(exe string) {
	home, _ := os.UserHomeDir()
	plistDir := filepath.Join(home, "Library", "LaunchAgents")
	os.MkdirAll(plistDir, 0755)

	plistContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>ai.atavus.agent</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>connect</string>
        <string>--autostart</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/tmp/atavus-agent.log</string>
    <key>StandardErrorPath</key>
    <string>/tmp/atavus-agent.log</string>
</dict>
</plist>`, exe)

	plistPath := filepath.Join(plistDir, "ai.atavus.agent.plist")
	os.WriteFile(plistPath, []byte(plistContent), 0644)
	runCmd("launchctl load " + plistPath)
}

func removeMacOSLaunchAgent() {
	home, _ := os.UserHomeDir()
	plistPath := filepath.Join(home, "Library", "LaunchAgents", "ai.atavus.agent.plist")
	runCmd("launchctl unload " + plistPath + " 2>/dev/null")
	os.Remove(plistPath)
}

func runCmd(cmd string) {
	var args []string
	if detectPlatform() == "windows" {
		args = []string{"cmd", "/c", cmd}
	} else {
		args = []string{"sh", "-c", cmd}
	}
	proc, err := os.StartProcess(args[0], args, &os.ProcAttr{
		Files: []*os.File{nil, nil, nil},
	})
	if err != nil {
		log.Printf("runCmd error: %v", err)
		return
	}
	proc.Wait()
}

// ── HTTP helpers for pairing ──

type PairingResult struct {
	AuthToken string `json:"auth_token"`
	DeviceID  string `json:"device_id"`
}

func pairDevice(serverURL, code, deviceName, platform string) (*PairingResult, error) {
	payload, _ := json.Marshal(map[string]string{
		"pairing_code": code,
		"device_name":  deviceName,
		"device_type":  platform,
	})

	req, err := http.NewRequest("POST", serverURL+"/api/v1/device-manage/link", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("request error: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("server error (%d): %s", resp.StatusCode, string(body))
	}

	var result PairingResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	if result.AuthToken == "" || result.DeviceID == "" {
		return nil, fmt.Errorf("invalid server response")
	}

	return &result, nil
}

func notifyDisconnect(cfg *Config) {
	if cfg.ServerURL == "" || cfg.DeviceID == "" {
		return
	}

	payload, _ := json.Marshal(map[string]string{
		"auth_token": cfg.AuthToken,
	})

	req, err := http.NewRequest("DELETE", cfg.ServerURL+"/api/v1/device-manage/"+cfg.DeviceID, bytes.NewReader(payload))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err == nil {
		resp.Body.Close()
	}
}
