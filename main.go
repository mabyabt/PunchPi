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

// ... (previous struct definitions remain the same)

// New structs for RFID device communication
type RFIDDevice struct {
	ID       string    `json:"id"`
	LastSeen time.Time `json:"last_seen"`
	Status   string    `json:"status"`
}

type CardScanEvent struct {
	DeviceID string    `json:"device_id"`
	CardUID  string    `json:"card_uid"`
	Time     time.Time `json:"time"`
}

var (
	db        *sql.DB
	templates *template.Template
	upgrader  = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins for development
		},
	}
	activeDevices = make(map[string]*websocket.Conn)
)

// ... (previous initDB function remains the same)

// New function to handle RFID device WebSocket connections
func handleDeviceWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	deviceID := r.URL.Query().Get("device_id")
	if deviceID == "" {
		log.Println("Device ID not provided")
		return
	}

	activeDevices[deviceID] = conn
	defer delete(activeDevices, deviceID)

	log.Printf("Device connected: %s", deviceID)

	for {
		var scanEvent CardScanEvent
		err := conn.ReadJSON(&scanEvent)
		if err != nil {
			log.Printf("WebSocket read error: %v", err)
			break
		}

		scanEvent.DeviceID = deviceID
		scanEvent.Time = time.Now()

		// Process the card scan
		if err := processCardScan(scanEvent); err != nil {
			log.Printf("Error processing card scan: %v", err)
			conn.WriteJSON(map[string]string{"status": "error", "message": err.Error()})
		} else {
			conn.WriteJSON(map[string]string{"status": "success"})
		}
	}
}

// New function to process card scans
func processCardScan(scan CardScanEvent) error {
	// Look up employee
	var employee Employee
	err := db.QueryRow(`
		SELECT id, name, is_present 
		FROM employees 
		WHERE card_uid = ?`, scan.CardUID).Scan(&employee.ID, &employee.Name, &employee.IsPresent)
	
	if err == sql.ErrNoRows {
		return fmt.Errorf("unknown card: %s", scan.CardUID)
	} else if err != nil {
		return fmt.Errorf("database error: %v", err)
	}

	// Start transaction
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("transaction error: %v", err)
	}
	defer tx.Rollback()

	if !employee.IsPresent {
		// Clock in
		_, err = tx.Exec(`
			INSERT INTO time_records (employee_id, clock_in)
			VALUES (?, ?)`, employee.ID, scan.Time)
		if err != nil {
			return fmt.Errorf("clock-in error: %v", err)
		}

		_, err = tx.Exec(`
			UPDATE employees 
			SET is_present = TRUE, last_clock_in = ? 
			WHERE id = ?`, scan.Time, employee.ID)
	} else {
		// Clock out
		var recordID int
		err = tx.QueryRow(`
			SELECT id FROM time_records 
			WHERE employee_id = ? AND clock_out IS NULL 
			ORDER BY clock_in DESC LIMIT 1`, employee.ID).Scan(&recordID)
		if err != nil {
			return fmt.Errorf("record lookup error: %v", err)
		}

		_, err = tx.Exec(`
			UPDATE time_records 
			SET clock_out = ?,
				total_hours = ROUND(CAST((JULIANDAY(?) - JULIANDAY(clock_in)) * 24 AS REAL), 2)
			WHERE id = ?`, scan.Time, scan.Time, recordID)
		if err != nil {
			return fmt.Errorf("clock-out error: %v", err)
		}

		_, err = tx.Exec(`
			UPDATE employees 
			SET is_present = FALSE, last_clock_out = ? 
			WHERE id = ?`, scan.Time, employee.ID)
	}

	return tx.Commit()
}

// New API endpoints
func apiGetEmployees(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT id, name, card_uid, is_present, last_clock_in, last_clock_out 
		FROM employees 
		ORDER BY name`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var employees []Employee
	for rows.Next() {
		var emp Employee
		if err := rows.Scan(&emp.ID, &emp.Name, &emp.CardUID, &emp.IsPresent,
			&emp.LastClockIn, &emp.LastClockOut); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		employees = append(employees, emp)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(employees)
}

func apiGetTimeRecords(w http.ResponseWriter, r *http.Request) {
	startDate := r.URL.Query().Get("start")
	endDate := r.URL.Query().Get("end")
	employeeID := r.URL.Query().Get("employee_id")

	query := `
		SELECT tr.id, tr.employee_id, e.name, tr.clock_in, tr.clock_out,
			ROUND(CAST((JULIANDAY(tr.clock_out) - JULIANDAY(tr.clock_in)) * 24 AS REAL), 2) as total_hours
		FROM time_records tr
		JOIN employees e ON tr.employee_id = e.id
		WHERE 1=1`
	args := []interface{}{}

	if startDate != "" {
		query += " AND tr.clock_in >= ?"
		args = append(args, startDate)
	}
	if endDate != "" {
		query += " AND tr.clock_in <= ?"
		args = append(args, endDate)
	}
	if employeeID != "" {
		query += " AND tr.employee_id = ?"
		args = append(args, employeeID)
	}

	query += " ORDER BY tr.clock_in DESC"

	rows, err := db.Query(query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var records []TimeRecord
	for rows.Next() {
		var record TimeRecord
		if err := rows.Scan(&record.ID, &record.EmployeeID, &record.Name,
			&record.ClockIn, &record.ClockOut, &record.TotalHours); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		records = append(records, record)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(records)
}

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
