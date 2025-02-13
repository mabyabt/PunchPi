// RFID/NFC Punch Clock System - Backend (Go)
// Handles RFID/NFC scans and logs them to MySQL

package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	_ "github.com/go-sql-driver/mysql"
	"github.com/tarm/serial"
)

type ScanRequest struct {
	CardID string `json:"card_id"`
}

type User struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

var db *sql.DB

func main() {
	var err error
	db, err = sql.Open("mysql", "user:password@tcp(localhost:3306)/rfid_system")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	http.HandleFunc("/scan", handleScan)
	http.HandleFunc("/logs", getLogs)
	http.HandleFunc("/admin", adminPanel)
	http.HandleFunc("/employee", employeePanel)
	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/manage_users", manageUsers)
	http.HandleFunc("/add_user", addUser)
	http.HandleFunc("/delete_user", deleteUser)
	http.HandleFunc("/update_user", updateUser)

	log.Println("RFID Server running on port 8080...")
	http.ListenAndServe(":8080", nil)
}

func handleScan(w http.ResponseWriter, r *http.Request) {
	var req ScanRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Check if card exists in the database
	var employeeID int
	err = db.QueryRow("SELECT id FROM employees WHERE card_id = ?", req.CardID).Scan(&employeeID)
	if err != nil {
		if err == sql.ErrNoRows {
			w.WriteHeader(http.StatusUnauthorized) // Ignore unregistered cards
			return
		}
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Log the clock-in/clock-out event
	_, err = db.Exec("INSERT INTO logs (employee_id, timestamp) VALUES (?, NOW())", employeeID)
	if err != nil {
		http.Error(w, "Failed to log event", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func getLogs(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT employees.name, logs.timestamp FROM logs JOIN employees ON logs.employee_id = employees.id ORDER BY logs.timestamp DESC")
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var logs []map[string]string
	for rows.Next() {
		var name, timestamp string
		rows.Scan(&name, &timestamp)
		logs = append(logs, map[string]string{"name": name, "timestamp": timestamp})
	}

	json.NewEncoder(w).Encode(logs)
}

func adminPanel(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "admin.html")
}

func employeePanel(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "employee.html")
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "login.html")
}

func manageUsers(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "manage_users.html")
}

func addUser(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "add_user.html")
}

func deleteUser(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "delete_user.html")
}

func updateUser(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "update_user.html")
}
