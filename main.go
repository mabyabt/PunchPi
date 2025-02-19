package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
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
	ID           int       `json:"id"`
	Name         string    `json:"name"`
	CardUID      string    `json:"card_uid"`
	IsPresent    bool      `json:"is_present"`
	LastClockIn  time.Time `json:"last_clock_in"`
	LastClockOut time.Time `json:"last_clock_out"`
}

type TimeRecord struct {
	ID         int       `json:"id"`
	EmployeeID int       `json:"employee_id"`
	Name       string    `json:"name"`
	ClockIn    time.Time `json:"clock_in"`
	ClockOut   time.Time `json:"clock_out"`
	TotalHours float64   `json:"total_hours"`
}

type PageData struct {
	User         *User
	Employees    []Employee
	TimeRecords  []TimeRecord
	Message      string
	CurrentTime  time.Time
	TotalPresent int
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

	// Create tables
	createTables := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		password TEXT NOT NULL,
		role TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS employees (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		card_uid TEXT UNIQUE NOT NULL,
		is_present BOOLEAN DEFAULT FALSE,
		last_clock_in DATETIME,
		last_clock_out DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS time_records (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		employee_id INTEGER NOT NULL,
		clock_in DATETIME NOT NULL,
		clock_out DATETIME,
		total_hours REAL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (employee_id) REFERENCES employees(id)
	);

	CREATE INDEX IF NOT EXISTS idx_card_uid ON employees(card_uid);
	CREATE INDEX IF NOT EXISTS idx_employee_time ON time_records(employee_id, clock_in);`

	if _, err = db.Exec(createTables); err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	// Create default admin if not exists
	var count int
	if err = db.QueryRow("SELECT COUNT(*) FROM users WHERE username = 'admin'").Scan(&count); err != nil {
		return fmt.Errorf("failed to check admin user: %w", err)
	}

	if count == 0 {
		hashedPassword := hashPassword("admin")
		if _, err = db.Exec(`
			INSERT INTO users (username, password, role) 
			VALUES (?, ?, ?)`,
			"admin", hashedPassword, "admin"); err != nil {
			return fmt.Errorf("failed to create admin user: %w", err)
		}
		log.Println("Default admin user created (admin/admin)")
	}

	return nil
}

func hashPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return hex.EncodeToString(hash[:])
}

// Handlers
func dashboardHandler(w http.ResponseWriter, r *http.Request) {
	data := PageData{
		CurrentTime: time.Now(),
	}

	// Get present employees
	rows, err := db.Query(`
		SELECT id, name, card_uid, is_present, last_clock_in, last_clock_out 
		FROM employees 
		ORDER BY name`)
	if err != nil {
		log.Printf("Error fetching employees: %v", err)
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var emp Employee
		if err := rows.Scan(&emp.ID, &emp.Name, &emp.CardUID, &emp.IsPresent,
			&emp.LastClockIn, &emp.LastClockOut); err != nil {
			log.Printf("Error scanning employee: %v", err)
			continue
		}
		data.Employees = append(data.Employees, emp)
		if emp.IsPresent {
			data.TotalPresent++
		}
	}

	// Get recent time records
	timeRows, err := db.Query(`
		SELECT tr.id, tr.employee_id, e.name, tr.clock_in, tr.clock_out,
			ROUND(CAST((JULIANDAY(tr.clock_out) - JULIANDAY(tr.clock_in)) * 24 AS REAL), 2) as total_hours
		FROM time_records tr
		JOIN employees e ON tr.employee_id = e.id
		WHERE tr.clock_out IS NOT NULL
		ORDER BY tr.clock_in DESC LIMIT 10`)
	if err != nil {
		log.Printf("Error fetching time records: %v", err)
	} else {
		defer timeRows.Close()
		for timeRows.Next() {
			var record TimeRecord
			if err := timeRows.Scan(&record.ID, &record.EmployeeID, &record.Name,
				&record.ClockIn, &record.ClockOut, &record.TotalHours); err != nil {
				log.Printf("Error scanning time record: %v", err)
				continue
			}
			data.TimeRecords = append(data.TimeRecords, record)
		}
	}

	if err := templates.ExecuteTemplate(w, "dashboard.html", data); err != nil {
		log.Printf("Template error: %v", err)
		http.Error(w, "Server error", http.StatusInternalServerError)
	}
}

func employeesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		// Add new employee
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

		http.Redirect(w, r, "/employees", http.StatusSeeOther)
		return
	}

	// Get all employees
	rows, err := db.Query(`
		SELECT id, name, card_uid, is_present, last_clock_in, last_clock_out 
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
		if err := rows.Scan(&emp.ID, &emp.Name, &emp.CardUID, &emp.IsPresent,
			&emp.LastClockIn, &emp.LastClockOut); err != nil {
			log.Printf("Error scanning employee: %v", err)
			continue
		}
		employees = append(employees, emp)
	}

	data := PageData{
		Employees: employees,
	}

	if err := templates.ExecuteTemplate(w, "employees.html", data); err != nil {
		log.Printf("Template error: %v", err)
		http.Error(w, "Server error", http.StatusInternalServerError)
	}
}

func reportsHandler(w http.ResponseWriter, r *http.Request) {
	// Get date range from query params or use default (current month)
	now := time.Now()
	startDate := r.URL.Query().Get("start")
	endDate := r.URL.Query().Get("end")

	var start, end time.Time
	var err error

	if startDate != "" {
		start, err = time.Parse("2006-01-02", startDate)
		if err != nil {
			start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		}
	} else {
		start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	}

	if endDate != "" {
		end, err = time.Parse("2006-01-02", endDate)
		if err != nil {
			end = now
		}
	} else {
		end = now
	}

	// Get time records for the period
	rows, err := db.Query(`
		SELECT tr.id, tr.employee_id, e.name, tr.clock_in, tr.clock_out,
			ROUND(CAST((JULIANDAY(tr.clock_out) - JULIANDAY(tr.clock_in)) * 24 AS REAL), 2) as total_hours
		FROM time_records tr
		JOIN employees e ON tr.employee_id = e.id
		WHERE tr.clock_in BETWEEN ? AND ?
		ORDER BY tr.clock_in DESC`, start, end)
	if err != nil {
		log.Printf("Error fetching time records: %v", err)
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var records []TimeRecord
	for rows.Next() {
		var record TimeRecord
		if err := rows.Scan(&record.ID, &record.EmployeeID, &record.Name,
			&record.ClockIn, &record.ClockOut, &record.TotalHours); err != nil {
			log.Printf("Error scanning time record: %v", err)
			continue
		}
		records = append(records, record)
	}

	data := PageData{
		TimeRecords: records,
	}

	if err := templates.ExecuteTemplate(w, "reports.html", data); err != nil {
		log.Printf("Template error: %v", err)
		http.Error(w, "Server error", http.StatusInternalServerError)
	}
}

func clockInOutHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cardUID := r.FormValue("card_uid")
	if cardUID == "" {
		http.Error(w, "Card UID is required", http.StatusBadRequest)
		return
	}

	// Look up employee
	var employee Employee
	err := db.QueryRow(`
		SELECT id, name, is_present 
		FROM employees 
		WHERE card_uid = ?`, cardUID).Scan(&employee.ID, &employee.Name, &employee.IsPresent)
	
	if err == sql.ErrNoRows {
		http.Error(w, "Unknown card", http.StatusNotFound)
		return
	} else if err != nil {
		log.Printf("Database error: %v", err)
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	// Start transaction
	tx, err := db.Begin()
	if err != nil {
		log.Printf("Transaction error: %v", err)
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	now := time.Now()

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

		_, err = tx.Exec(`
			UPDATE employees 
			SET is_present = TRUE, last_clock_in = ? 
			WHERE id = ?`, now, employee.ID)
	} else {
		// Clock out
		var recordID int
		err = tx.QueryRow(`
			SELECT id FROM time_records 
			WHERE employee_id = ? AND clock_out IS NULL 
			ORDER BY clock_in DESC LIMIT 1`, employee.ID).Scan(&recordID)
		if err != nil {
			log.Printf("Record lookup error: %v", err)
			http.Error(w, "Server error", http.StatusInternalServerError)
			return
		}

		_, err = tx.Exec(`
			UPDATE time_records 
			SET clock_out = ?,
				total_hours = ROUND(CAST((JULIANDAY(?) - JULIANDAY(clock_in)) * 24 AS REAL), 2)
			WHERE id = ?`, now, now, recordID)
		if err != nil {
			log.Printf("Clock-out error: %v", err)
			http.Error(w, "Server error", http.StatusInternalServerError)
			return
		}

		_, err = tx.Exec(`
			UPDATE employees 
			SET is_present = FALSE, last_clock_out = ? 
			WHERE id = ?`, now, employee.ID)
	}

	if err = tx.Commit(); err != nil {
		log.Printf("Transaction commit error: %v", err)
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
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

	// Set up routes
	http.HandleFunc("/", dashboardHandler)
	http.HandleFunc("/dashboard", dashboardHandler)
	http.HandleFunc("/employees", employeesHandler)
	http.HandleFunc("/reports", reportsHandler)
	http.HandleFunc("/clock", clockInOutHandler)

	// Start server
	log.Println("Starting server on :8080...")
