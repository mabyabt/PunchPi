package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

// Define the Employee struct
type Employee struct {
	ID           int       `json:"id"`
	Name         string    `json:"name"`
	CardUID      string    `json:"card_uid"`
	IsPresent    bool      `json:"is_present"`
	LastClockIn  time.Time `json:"last_clock_in"`
	LastClockOut time.Time `json:"last_clock_out"`
}

// Define the TimeRecord struct
type TimeRecord struct {
	ID         int       `json:"id"`
	EmployeeID int       `json:"employee_id"`
	Name       string    `json:"name"`
	ClockIn    time.Time `json:"clock_in"`
	ClockOut   time.Time `json:"clock_out"`
	TotalHours float64   `json:"total_hours"`
}

// Define the initDB function
func initDB() error {
	var err error
	db, err = sql.Open("sqlite3", "./time_tracking.db")
	if err != nil {
		return err
	}

	// Create tables if they don't exist
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS employees (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			card_uid TEXT NOT NULL UNIQUE,
			is_present BOOLEAN NOT NULL DEFAULT FALSE,
			last_clock_in DATETIME,
			last_clock_out DATETIME
		);

		CREATE TABLE IF NOT EXISTS time_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			employee_id INTEGER NOT NULL,
			clock_in DATETIME NOT NULL,
			clock_out DATETIME,
			total_hours REAL,
			FOREIGN KEY (employee_id) REFERENCES employees (id)
		);
	`)
	return err
}

// Define the dashboardHandler function
func dashboardHandler(w http.ResponseWriter, r *http.Request) {
	// Render the dashboard template
	err := templates.ExecuteTemplate(w, "dashboard.html", nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Define the employeesHandler function
func employeesHandler(w http.ResponseWriter, r *http.Request) {
	// Render the employees template
	err := templates.ExecuteTemplate(w, "employees.html", nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Define the reportsHandler function
func reportsHandler(w http.ResponseWriter, r *http.Request) {
	// Render the reports template
	err := templates.ExecuteTemplate(w, "reports.html", nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Define the clockInOutHandler function
func clockInOutHandler(w http.ResponseWriter, r *http.Request) {
	// Render the clock in/out template
	err := templates.ExecuteTemplate(w, "clock.html", nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// ... (rest of your existing code)

func main() {
	// Initialize templates
	var err error
	templates, err = template.ParseGlob("templates/*.html")
	if err != nil {
		log.Fatalf("Failed to parse templates: %v", err)
	}

	// Initialize database
	if err := initDB(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Create router
	r := mux.NewRouter()

	// Web interface routes
	r.HandleFunc("/", dashboardHandler)
	r.HandleFunc("/dashboard", dashboardHandler)
	r.HandleFunc("/employees", employeesHandler)
	r.HandleFunc("/reports", reportsHandler)
	r.HandleFunc("/clock", clockInOutHandler)

	// API routes
	api := r.PathPrefix("/api").Subrouter()
	api.HandleFunc("/employees", apiGetEmployees).Methods("GET")
	api.HandleFunc("/time-records", apiGetTimeRecords).Methods("GET")

	// WebSocket endpoint for RFID devices
	r.HandleFunc("/ws/device", handleDeviceWebSocket)

	// Start server
	log.Println("Starting server on :8080...")
	log.Fatal(http.ListenAndServe(":8080", r))
}
