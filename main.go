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
)

var (
	db        *sql.DB
	templates *template.Template
)

// Struct definitions
type User struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Password string `json:"-"`
	Role     string `json:"role"`
}

type Employee struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	CardUID   string `json:"card_uid"`
	IsPresent bool   `json:"is_present"`
}

type TimeRecord struct {
	ID         int       `json:"id"`
	EmployeeID int       `json:"employee_id"`
	ClockIn    time.Time `json:"clock_in"`
	ClockOut   time.Time `json:"clock_out"`
	IsComplete bool      `json:"is_complete"`
}

func initDB() error {
	log.Println("Initializing database...")
	dbDir := "db"
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return fmt.Errorf("failed to create database directory: %w", err)
	}

	dbPath := filepath.Join(dbDir, "timetrack.db")
	var err error
	db, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Create tables with RFID card support
	createTables := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		password TEXT NOT NULL,
		role TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS employees (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		card_uid TEXT UNIQUE NOT NULL,
		is_present BOOLEAN DEFAULT FALSE,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS time_records (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		employee_id INTEGER NOT NULL,
		clock_in DATETIME NOT NULL,
		clock_out DATETIME,
		is_completed BOOLEAN DEFAULT FALSE,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (employee_id) REFERENCES employees(id)
	);

	-- Indexes for better performance
	CREATE INDEX IF NOT EXISTS idx_card_uid ON employees(card_uid);
	CREATE INDEX IF NOT EXISTS idx_is_present ON employees(is_present);
	CREATE INDEX IF NOT EXISTS idx_employee_time ON time_records(employee_id, clock_in);`

	if _, err = db.Exec(createTables); err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	return nil
}

// RFID card handling functions
func handleRFIDScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var cardData struct {
		CardUID string `json:"card_uid"`
	}

	if err := json.NewDecoder(r.Body).Decode(&cardData); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Look up employee by card UID
	var employee Employee
	err := db.QueryRow("SELECT id, name, is_present FROM employees WHERE card_uid = ?", 
		cardData.CardUID).Scan(&employee.ID, &employee.Name, &employee.IsPresent)
	
	if err == sql.ErrNoRows {
		http.Error(w, "Unknown card", http.StatusNotFound)
		return
	} else if err != nil {
		log.Printf("Database error: %v", err)
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	// Start transaction for clock in/out
	tx, err := db.Begin()
	if err != nil {
		log.Printf("Transaction error: %v", err)
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	now := time.Now()
	response := map[string]interface{}{
		"employee_name": employee.Name,
	}

	if !employee.IsPresent {
		// Clock in
		_, err = tx.Exec(`
			INSERT INTO time_records (employee_id, clock_in)
			VALUES (?, ?)`, employee.ID, now)
		if err != nil {
			log.Printf("Clock-in error: %v", err)
			http.Error(w, "Server error", http.StatusInternalServerError)
			return
		}

		_, err = tx.Exec("UPDATE employees SET is_present = TRUE WHERE id = ?", employee.ID)
		response["action"] = "clock_in"
		response["message"] = fmt.Sprintf("Welcome %s! Clock-in time: %s", 
			employee.Name, now.Format("15:04:05"))
	} else {
		// Clock out
		var recordID int
		err = tx.QueryRow(`
			SELECT id FROM time_records 
			WHERE employee_id = ? AND is_completed = FALSE 
			ORDER BY clock_in DESC LIMIT 1`, employee.ID).Scan(&recordID)
		if err != nil {
			log.Printf("Record lookup error: %v", err)
			http.Error(w, "Server error", http.StatusInternalServerError)
			return
		}

		_, err = tx.Exec(`
			UPDATE time_records 
			SET clock_out = ?, is_completed = TRUE 
			WHERE id = ?`, now, recordID)
		if err != nil {
			log.Printf("Clock-out error: %v", err)
			http.Error(w, "Server error", http.StatusInternalServerError)
			return
		}

		_, err = tx.Exec("UPDATE employees SET is_present = FALSE WHERE id = ?", employee.ID)
		response["action"] = "clock_out"
		response["message"] = fmt.Sprintf("Goodbye %s! Clock-out time: %s", 
			employee.Name, now.Format("15:04:05"))
	}

	if err = tx.Commit(); err != nil {
		log.Printf("Transaction commit error: %v", err)
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Employee management handlers
func addEmployeeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := r.FormValue("name")
	cardUID := r.FormValue("card_uid")

	if name == "" || cardUID == "" {
		http.Error(w, "Name and Card UID are required", http.StatusBadRequest)
		return
	}

	_, err := db.Exec(`
		INSERT INTO employees (name, card_uid)
		VALUES (?, ?)`, name, cardUID)

	if err != nil {
		log.Printf("Error adding employee: %v", err)
		http.Error(w, "Error adding employee", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)
}

func getEmployeesHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT id, name, card_uid, is_present 
		FROM employees 
		ORDER BY name`)
	if err != nil {
		log.Printf("Error fetching employees: %v", err)
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var employees []Employee
	for rows.Next() {
		var emp Employee
		if err := rows.Scan(&emp.ID, &emp.Name, &emp.CardUID, &emp.IsPresent); err != nil {
			log.Printf("Error scanning employee: %v", err)
			continue
		}
		employees = append(employees, emp)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(employees)
}

func main() {
	if err := initDB(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Set up routes
	http.HandleFunc("/api/rfid/scan", handleRFIDScan)
	http.HandleFunc("/api/employees", getEmployeesHandler)
	http.HandleFunc("/admin/employees/add", addEmployeeHandler)

	// Start server
	log.Println("Starting server on :8080...")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
