package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/mattn/go-sqlite3"
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
	ClockIn    time.Time `json:"clock_in"`
	ClockOut   time.Time `json:"clock_out"`
	TotalHours float64   `json:"total_hours"`
}

// Define the CardScanEvent struct
type CardScanEvent struct {
	DeviceID string    `json:"device_id"`
	CardUID  string    `json:"card_uid"`
	Time     time.Time `json:"time"`
}

var (
	db     *sql.DB
	logger *log.Logger
)

// Initialize logger
func initLogger() {
	// Create logs directory if it doesn't exist
	if err := os.MkdirAll("logs", 0755); err != nil {
		log.Fatal("Failed to create logs directory:", err)
	}

	// Open log file
	file, err := os.OpenFile("logs/time_tracking.log",
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatal("Failed to open log file:", err)
	}

	// Initialize logger with timestamp
	logger = log.New(file, "", log.Ldate|log.Ltime)
}

// Initialize database
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

// Process card scans
func processCardScan(scan CardScanEvent) (*Employee, error) {
	logger.Printf("Processing card scan - Device: %s, Card UID: %s",
		scan.DeviceID, scan.CardUID)

	// Look up employee
	var employee Employee
	err := db.QueryRow(`
		SELECT id, name, card_uid, is_present, last_clock_in, last_clock_out 
		FROM employees 
		WHERE card_uid = ?`, scan.CardUID).Scan(
		&employee.ID, &employee.Name, &employee.CardUID, &employee.IsPresent,
		&employee.LastClockIn, &employee.LastClockOut,
	)

	if err == sql.ErrNoRows {
		logger.Printf("Unknown card UID: %s", scan.CardUID)
		return nil, fmt.Errorf("unknown card: %s", scan.CardUID)
	} else if err != nil {
		logger.Printf("Database error looking up employee: %v", err)
		return nil, fmt.Errorf("database error: %v", err)
	}

	logger.Printf("Found employee - ID: %d, Name: %s, Currently present: %v",
		employee.ID, employee.Name, employee.IsPresent)

	// Start transaction
	tx, err := db.Begin()
	if err != nil {
		logger.Printf("Failed to start transaction: %v", err)
		return nil, fmt.Errorf("transaction error: %v", err)
	}
	defer tx.Rollback()

	if !employee.IsPresent {
		// Clock in
		logger.Printf("Clocking in employee %s (ID: %d)", employee.Name, employee.ID)

		result, err := tx.Exec(`
			INSERT INTO time_records (employee_id, clock_in)
			VALUES (?, ?)`, employee.ID, scan.Time)
		if err != nil {
			logger.Printf("Clock-in error: %v", err)
			return nil, fmt.Errorf("clock-in error: %v", err)
		}

		recordID, _ := result.LastInsertId()
		logger.Printf("Created time record ID: %d", recordID)

		_, err = tx.Exec(`
			UPDATE employees 
			SET is_present = TRUE, last_clock_in = ? 
			WHERE id = ?`, scan.Time, employee.ID)
		if err != nil {
			logger.Printf("Failed to update employee status: %v", err)
			return nil, fmt.Errorf("employee update error: %v", err)
		}

		// Verify the clock-in
		var verifyTime time.Time
		err = tx.QueryRow(`
			SELECT clock_in 
			FROM time_records 
			WHERE id = ?`, recordID).Scan(&verifyTime)
		if err != nil {
			logger.Printf("Failed to verify clock-in: %v", err)
			return nil, fmt.Errorf("verification error: %v", err)
		}
		logger.Printf("Verified clock-in time: %v", verifyTime)

	} else {
		// Clock out
		logger.Printf("Clocking out employee %s (ID: %d)", employee.Name, employee.ID)

		var recordID int
		var clockInTime time.Time
		err = tx.QueryRow(`
			SELECT id, clock_in 
			FROM time_records 
			WHERE employee_id = ? AND clock_out IS NULL 
			ORDER BY clock_in DESC LIMIT 1`, employee.ID).Scan(&recordID, &clockInTime)
		if err != nil {
			logger.Printf("Failed to find open time record: %v", err)
			return nil, fmt.Errorf("record lookup error: %v", err)
		}

		logger.Printf("Found open time record ID: %d, Clock-in time: %v",
			recordID, clockInTime)

		_, err = tx.Exec(`
			UPDATE time_records 
			SET clock_out = ?,
				total_hours = ROUND(CAST((JULIANDAY(?) - JULIANDAY(clock_in)) * 24 AS REAL), 2)
			WHERE id = ?`, scan.Time, scan.Time, recordID)
		if err != nil {
			logger.Printf("Clock-out error: %v", err)
			return nil, fmt.Errorf("clock-out error: %v", err)
		}

		_, err = tx.Exec(`
			UPDATE employees 
			SET is_present = FALSE, last_clock_out = ? 
			WHERE id = ?`, scan.Time, employee.ID)
		if err != nil {
			logger.Printf("Failed to update employee status: %v", err)
			return nil, fmt.Errorf("employee update error: %v", err)
		}

		// Verify the clock-out
		var verifyTime time.Time
		var totalHours float64
		err = tx.QueryRow(`
			SELECT clock_out, total_hours 
			FROM time_records 
			WHERE id = ?`, recordID).Scan(&verifyTime, &totalHours)
		if err != nil {
			logger.Printf("Failed to verify clock-out: %v", err)
			return nil, fmt.Errorf("verification error: %v", err)
		}
		logger.Printf("Verified clock-out time: %v, Total hours: %.2f",
			verifyTime, totalHours)
	}

	if err := tx.Commit(); err != nil {
		logger.Printf("Failed to commit transaction: %v", err)
		return nil, fmt.Errorf("commit error: %v", err)
	}

	logger.Printf("Successfully processed card scan for %s", employee.Name)
	return &employee, nil
}

func main() {
	// Initialize logger
	initLogger()
	logger.Println("Starting time tracking application...")

	// Initialize database
	if err := initDB(); err != nil {
		log.Fatal("Failed to initialize database:", err)
	}
	defer db.Close()

	// Add a test employee if the table is empty
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM employees").Scan(&count)
	if err != nil {
		log.Fatal("Failed to check employees:", err)
	}

	if count == 0 {
		_, err = db.Exec(`
			INSERT INTO employees (name, card_uid, is_present)
			VALUES (?, ?, ?)`,
			"Test Employee", "1234567890", false)
		if err != nil {
			log.Fatal("Failed to insert test employee:", err)
		}
		logger.Println("Added test employee with card UID: 1234567890")
	}

	// Simulate a card scan for testing
	testScan := CardScanEvent{
		DeviceID: "TEST-DEVICE-001",
		CardUID:  "1234567890",
		Time:     time.Now(),
	}

	employee, err := processCardScan(testScan)
	if err != nil {
		log.Printf("Error processing test scan: %v", err)
	} else {
		log.Printf("Successfully processed test scan for employee: %s", employee.Name)
	}

	// Keep the program running
	select {}
}
