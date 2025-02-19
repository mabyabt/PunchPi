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

// Define the RFIDDevice struct
type RFIDDevice struct {
	ID       string    `json:"id"`
	LastSeen time.Time `json:"last_seen"`
	Status   string    `json:"status"`
}

// Define the CardScanEvent struct
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

// Initialize the database
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

// Basic Authentication Middleware
func basicAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Hardcoded credentials for demonstration
		username := "admin"
		password := "password"

		// Get credentials from request
		user, pass, ok := r.BasicAuth()
		if !ok || user != username || pass != password {
			// If credentials are missing or incorrect, send a 401 Unauthorized response
			w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// If credentials are valid, call the next handler
		next.ServeHTTP(w, r)
	}
}

// Handle WebSocket connections from RFID devices
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
		employee, err := processCardScan(scanEvent)
		if err != nil {
			log.Printf("Error processing card scan: %v", err)
			conn.WriteJSON(map[string]string{"status": "error", "message": err.Error()})
		} else {
			// Send success response with employee information
			conn.WriteJSON(map[string]interface{}{
				"status":   "success",
				"employee": employee,
			})
		}
	}
}

// Process card scans
func processCardScan(scan CardScanEvent) (*Employee, error) {
	// Look up employee
	var employee Employee
	err := db.QueryRow(`
		SELECT id, name, is_present, last_clock_in, last_clock_out 
		FROM employees 
		WHERE card_uid = ?`, scan.CardUID).Scan(
		&employee.ID, &employee.Name, &employee.IsPresent,
		&employee.LastClockIn, &employee.LastClockOut,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("unknown card: %s", scan.CardUID)
	} else if err != nil {
		return nil, fmt.Errorf("database error: %v", err)
	}

	// Start transaction
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("transaction error: %v", err)
	}
	defer tx.Rollback()

	if !employee.IsPresent {
		// Clock in
		_, err = tx.Exec(`
			INSERT INTO time_records (employee_id, clock_in)
			VALUES (?, ?)`, employee.ID, scan.Time)
		if err != nil {
			return nil, fmt.Errorf("clock-in error: %v", err)
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
			return nil, fmt.Errorf("record lookup error: %v", err)
		}

		_, err = tx.Exec(`
			UPDATE time_records 
			SET clock_out = ?,
				total_hours = ROUND(CAST((JULIANDAY(?) - JULIANDAY(clock_in)) * 24 AS REAL), 2)
			WHERE id = ?`, scan.Time, scan.Time, recordID)
		if err != nil {
			return nil, fmt.Errorf("clock-out error: %v", err)
		}

		_, err = tx.Exec(`
			UPDATE employees 
			SET is_present = FALSE, last_clock_out = ? 
			WHERE id = ?`, scan.Time, employee.ID)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit error: %v", err)
	}

	return &employee, nil
}

// API endpoint to get employees
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

// API endpoint to get time records
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

// Dashboard handler
func dashboardHandler(w http.ResponseWriter, r *http.Request) {
	err := templates.ExecuteTemplate(w, "dashboard.html", nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Employees handler
func employeesHandler(w http.ResponseWriter, r *http.Request) {
	err := templates.ExecuteTemplate(w, "employees.html", nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Reports handler
func reportsHandler(w http.ResponseWriter, r *http.Request) {
	err := templates.ExecuteTemplate(w, "reports.html", nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Clock in/out handler
func clockInOutHandler(w http.ResponseWriter, r *http.Request) {
	err := templates.ExecuteTemplate(w, "clock.html", nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
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

	// Web interface routes (protected)
	r.HandleFunc("/", basicAuthMiddleware(dashboardHandler))
	r.HandleFunc("/dashboard", basicAuthMiddleware(dashboardHandler))
	r.HandleFunc("/employees", basicAuthMiddleware(employeesHandler))
	r.HandleFunc("/reports", basicAuthMiddleware(reportsHandler))

	// API routes (protected)
	api := r.PathPrefix("/api").Subrouter()
	api.HandleFunc("/employees", basicAuthMiddleware(apiGetEmployees)).Methods("GET")
	api.HandleFunc("/time-records", basicAuthMiddleware(apiGetTimeRecords)).Methods("GET")

	// WebSocket endpoint for RFID devices (protected)
	r.HandleFunc("/ws/device", basicAuthMiddleware(handleDeviceWebSocket))

	// Clock in/out route (public)
	r.HandleFunc("/clock", clockInOutHandler)

	// Start server
	log.Println("Starting server on :8080...")
	log.Fatal(http.ListenAndServe(":8080", r))
}
