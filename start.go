package main

import (
	"log"
	"os/exec"
	"runtime"
	"time"
)

func main() {
	log.Println("🚀 Starting RFID Attendance System...")

	// Start the server in a Goroutine
	go startServer()

	// Wait a few seconds for the server to start before launching the client
	time.Sleep(2 * time.Second)

	// Start the client
	startClient()
}

// Function to start the server
func startServer() {
	log.Println("🖥️ Starting the server...")
	cmd := exec.Command("go", "run", "server/main.go", "server/database.go", "server/api.go")
	cmd.Stdout = log.Writer()
	cmd.Stderr = log.Writer()

	err := cmd.Run()
	if err != nil {
		log.Fatalf("❌ Server failed to start: %v", err)
	}
}

// Function to start the client
func startClient() {
	log.Println("💳 Starting the RFID client...")

	// Define the command based on OS
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", "go", "run", "client/main.go", "client/scanner.go")
	} else {
		cmd = exec.Command("go", "run", "client/main.go", "client/scanner.go")
	}

	cmd.Stdout = log.Writer()
	cmd.Stderr = log.Writer()

	err := cmd.Run()
	if err != nil {
		log.Fatalf("❌ Client failed to start: %v", err)
	}
}
