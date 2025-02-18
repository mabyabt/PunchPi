package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
	_ "github.com/mattn/go-sqlite3"
)

type ScanRequest struct {
	CardUID string `json:"card_uid"` // RFID card's unique identifier
}

type Employee struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	CardUID   string `json:"card_uid"`
	IsPresent bool   `json:"is_present"`
}

type TimeRecord struct {
	ID          int       `json:"id"`
	EmployeeID  int       `json:"employee_id"`
	ClockIn     time.Time `json:"clock_in"`
	ClockOut    time.Time `json:"clock_out,omitempty"`
	TotalHours  float64   `json:"total_hours,omitempty"`
	IsCompleted bool      `json:"is_completed"`
}

var db *sql.DB

func initDB() error {
	dbDir := "db"
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return err
	}

	var err error
	db, err = sql.Open("sqlite3", filepath.Join(dbDir, "timetrack.db"))
	if err != nil {
		return err
	}

	// Create tables with enhanced structure
	createTables := `
	CREATE TABLE IF NOT EXISTS employees (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		card_uid TEXT UNIQUE NOT NULL,
		is_present BOOLEAN DEFAULT FALSE,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS time_records (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		employee_id INTEGER NOT NULL,
		clock_in DATETIME NOT NULL,
		clock_out DATETIME,
		total_hours REAL,
		is_completed BOOLEAN DEFAULT FALSE,
		FOREIGN KEY (employee_id) REFERENCES employees(id)
	);

	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		password TEXT NOT NULL,
		role TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	-- Index for faster card UID lookups
	CREATE INDEX IF NOT EXISTS idx_card_uid ON employees(card_uid);
	`

	_, err = db.Exec(createTables)
	return err
}

func main() {
	if err := initDB(); err != nil {
		log.Fatal("Failed to initialize database:", err)
	}
	defer db.Close()

	// API endpoints
	http.HandleFunc("/api/scan", handleRFIDScan)
	http.HandleFunc("/api/employees", handleEmployees)
	http.HandleFunc("/api/time-records", handleTimeRecords)
	
	// Web interface endpoints
	http.HandleFunc("/admin", adminPanel)
	http.HandleFunc("/employee", employeePanel)
	http.HandleFunc("/login", loginHandler)

	log.Println("Time Tracking Server running on port 8080...")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}

func handleRFIDScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ScanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Begin transaction
	tx, err := db.Begin()
	if err != nil {
		log.Printf("Transaction error: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// Get employee by card UID
	var employee Employee
	err = tx.QueryRow(`
		SELECT id, name, is_present 
		FROM employees 
		WHERE card_uid = ?`, req.CardUID).Scan(&employee.ID, &employee.Name, &employee.IsPresent)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Unregistered card", http.StatusUnauthorized)
			return
		}
		log.Printf("Database error: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	currentTime := time.Now()

	if !employee.IsPresent {
		// Clock in
		_, err = tx.Exec(`
			INSERT INTO time_records (employee_id, clock_in) 
			VALUES (?, ?)`, employee.ID, currentTime)
		if err != nil {
			log.Printf("Clock-in error: %v", err)
			http.Error(w, "Failed to record clock-in", http.StatusInternalServerError)
			return
		}

		// Update employee status
		_, err = tx.Exec(`
			UPDATE employees 
			SET is_present = TRUE 
			WHERE id = ?`, employee.ID)
	} else {
		// Clock out: Find the open time record
		var recordID int
		var clockIn time.Time
		err = tx.QueryRow(`
			SELECT id, clock_in 
			FROM time_records 
			WHERE employee_id = ? AND is_completed = FALSE`, employee.ID).Scan(&recordID, &clockIn)
		if err != nil {
			log.Printf("Record lookup error: %v", err)
			http.Error(w, "Failed to find open time record", http.StatusInternalServerError)
			return
		}

		// Calculate hours worked
		hoursWorked := currentTime.Sub(clockIn).Hours()

		// Update the time record
		_, err = tx.Exec(`
			UPDATE time_records 
			SET clock_out = ?, total_hours = ?, is_completed = TRUE 
			WHERE id = ?`, currentTime, hoursWorked, recordID)
		if err != nil {
			log.Printf("Clock-out error: %v", err)
			http.Error(w, "Failed to record clock-out", http.StatusInternalServerError)
			return
		}

		// Update employee status
		_, err = tx.Exec(`
			UPDATE employees 
			SET is_present = FALSE 
			WHERE id = ?`, employee.ID)
	}

	if err != nil {
		log.Printf("Status update error: %v", err)
		http.Error(w, "Failed to update employee status", http.StatusInternalServerError)
		return
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		log.Printf("Commit error: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Prepare response
	response := map[string]interface{}{
		"employee_name": employee.Name,
		"action":        map[bool]string{true: "clock-out", false: "clock-in"}[employee.IsPresent],
		"timestamp":     currentTime.Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleTimeRecords(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	employeeID := r.URL.Query().Get("employee_id")
	startDate := r.URL.Query().Get("start_date")
	endDate := r.URL.Query().Get("end_date")

	query := `
		SELECT t.id, t.employee_id, e.name, t.clock_in, t.clock_out, t.total_hours, t.is_completed
		FROM time_records t
		JOIN employees e ON t.employee_id = e.id
		WHERE 1=1
	`
	var args []interface{}

	if employeeID != "" {
		query += " AND t.employee_id = ?"
		args = append(args, employeeID)
	}
	if startDate != "" {
		query += " AND DATE(t.clock_in) >= DATE(?)"
		args = append(args, startDate)
	}
	if endDate != "" {
		query += " AND DATE(t.clock_in) <= DATE(?)"
		args = append(args, endDate)
	}

	query += " ORDER BY t.clock_in DESC"

	rows, err := db.Query(query, args...)
	if err != nil {
		log.Printf("Query error: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var records []map[string]interface{}
	for rows.Next() {
		var record struct {
			ID         int
			EmployeeID int
			Name       string
			ClockIn    time.Time
			ClockOut   sql.NullTime
			TotalHours sql.NullFloat64
			IsCompleted bool
		}
		
		if err := rows.Scan(&record.ID, &record.EmployeeID, &record.Name, 
			&record.ClockIn, &record.ClockOut, &record.TotalHours, &record.IsCompleted); err != nil {
			log.Printf("Scan error: %v", err)
			continue
		}

		recordMap := map[string]interface{}{
			"id":          record.ID,
			"employee_id": record.EmployeeID,
			"name":        record.Name,
			"clock_in":    record.ClockIn.Format(time.RFC3339),
			"is_completed": record.IsCompleted,
		}

		if record.ClockOut.Valid {
			recordMap["clock_out"] = record.ClockOut.Time.Format(time.RFC3339)
		}
		if record.TotalHours.Valid {
			recordMap["total_hours"] = record.TotalHours.Float64
		}

		records = append(records, recordMap)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(records)
}

func handleEmployees(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rows, err := db.Query(`
			SELECT id, name, card_uid, is_present 
			FROM employees 
			ORDER BY name`)
		if err != nil {
			log.Printf("Query error: %v", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var employees []Employee
		for rows.Next() {
			var emp Employee
			if err := rows.Scan(&emp.ID, &emp.Name, &emp.CardUID, &emp.IsPresent); err != nil {
				log.Printf("Scan error: %v", err)
				continue
			}
			employees = append(employees, emp)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(employees)

	case http.MethodPost:
		var emp Employee
		if err := json.NewDecoder(r.Body).Decode(&emp); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		result, err := db.Exec(`
			INSERT INTO employees (name, card_uid) 
			VALUES (?, ?)`, emp.Name, emp.CardUID)
		if err != nil {
			log.Printf("Insert error: %v", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		id, _ := result.LastInsertId()
		emp.ID = int(id)
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(emp)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
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
