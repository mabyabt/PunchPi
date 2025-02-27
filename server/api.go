package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type ScanRequest struct {
	UID string `json:"uid"`
}

func handleRFIDScan(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	var req ScanRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil || req.UID == "" {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	// Use only the original UID for searching
	originalUID := req.UID

	// Look up user by original UID only
	var userName string
	var userId int
	err = db.QueryRow(`
		SELECT id, name FROM users WHERE rfid_uid_original = ?`,
		originalUID).Scan(&userId, &userName)

	if err == sql.ErrNoRows {
		http.Error(w, "Unknown RFID card", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Determine clock-in or clock-out
	var lastTimestamp time.Time
	err = db.QueryRow(`
		SELECT timestamp FROM clock_in_out WHERE user_id = ? ORDER BY timestamp DESC LIMIT 1`,
		userId).Scan(&lastTimestamp)

	eventType := "Clock-In"
	if err == nil && time.Since(lastTimestamp) < 12*time.Hour {
		eventType = "Clock-Out"
	}

	_, err = db.Exec(
		"INSERT INTO clock_in_out (rfid_uid_original, user_id, timestamp) VALUES (?, ?, datetime('now'))",
		originalUID, userId)

	if err != nil {
		http.Error(w, "Failed to record scan", http.StatusInternalServerError)
		return
	}

	response := fmt.Sprintf("%s: %s", eventType, userName)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(response))
}
