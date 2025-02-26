package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type ScanRequest struct {
	UID string `json:"uid"`
}

// normalizeRFIDInput takes an RFID UID string and returns both the original
// and a normalized version (converted to uppercase with spaces removed)
func normalizeRFIDInput(uid string) (string, string) {
	originalUID := uid
	normalizedUID := strings.ToUpper(strings.ReplaceAll(uid, " ", ""))
	return originalUID, normalizedUID
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

	originalUID, normalizedUID := normalizeRFIDInput(req.UID)

	// Look up user by UID
	var userName string
	var userId int
	err = db.QueryRow(`
		SELECT id, name FROM users WHERE rfid_uid_original = ? OR rfid_uid_normalized = ?`,
		originalUID, normalizedUID).Scan(&userId, &userName)

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
		"INSERT INTO clock_in_out (rfid_uid_original, rfid_uid_normalized, user_id, timestamp) VALUES (?, ?, ?, datetime('now'))",
		originalUID, normalizedUID, userId)

	if err != nil {
		http.Error(w, "Failed to record scan", http.StatusInternalServerError)
		return
	}

	response := fmt.Sprintf("%s: %s", eventType, userName)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(response))
}
