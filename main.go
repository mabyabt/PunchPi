package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

// ... [previous type definitions remain the same]

// Add User type
type User struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Password string `json:"-"` // "-" means this field won't be included in JSON
	Role     string `json:"role"`
}

// Hash password function
func hashPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return hex.EncodeToString(hash[:])
}

func initDB() error {
	log.Println("Initializing database...")
	dbDir := "db"
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		log.Printf("Failed to create database directory: %v", err)
		return err
	}
	log.Println("Database directory created/verified at ./db")

	var err error
	dbPath := filepath.Join(dbDir, "timetrack.db")
	db, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Printf("Failed to open database: %v", err)
		return err
	}
	log.Printf("Database connection established at %s", dbPath)

	// Test database connection
	if err := db.Ping(); err != nil {
		log.Printf("Database connection test failed: %v", err)
		return err
	}
	log.Println("Database connection test successful")

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
	);`

	_, err = db.Exec(createTables)
	if err != nil {
		log.Printf("Failed to create tables: %v", err)
		return err
	}
	log.Println("Database tables created successfully")

	// Check if admin user exists
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM users WHERE username = 'admin'").Scan(&count)
	if err != nil {
		log.Printf("Failed to check admin user: %v", err)
		return err
	}

	// Create default admin user if it doesn't exist
	if count == 0 {
		hashedPassword := hashPassword("admin")
		_, err = db.Exec(`
			INSERT INTO users (username, password, role) 
			VALUES (?, ?, ?)`,
			"admin", hashedPassword, "admin")
		if err != nil {
			log.Printf("Failed to create admin user: %v", err)
			return err
		}
		log.Println("Default admin user created successfully!")
		log.Println("Username: admin")
		log.Println("Password: admin")
		log.Println("Please change these credentials after first login!")
	}

	log.Println("Database initialization completed successfully")
	return nil
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		username := r.FormValue("username")
		password := r.FormValue("password")
		hashedPassword := hashPassword(password)

		var user User
		err := db.QueryRow(`
			SELECT id, username, role 
			FROM users 
			WHERE username = ? AND password = ?`,
			username, hashedPassword).Scan(&user.ID, &user.Username, &user.Role)

		if err != nil {
			if err == sql.ErrNoRows {
				log.Printf("Failed login attempt for username: %s", username)
				http.Error(w, "Invalid credentials", http.StatusUnauthorized)
				return
			}
			log.Printf("Database error during login: %v", err)
			http.Error(w, "Server error", http.StatusInternalServerError)
			return
		}

		// Here you would typically:
		// 1. Create a session
		// 2. Set a secure cookie
		// 3. Redirect to appropriate dashboard

		log.Printf("Successful login for user: %s with role: %s", user.Username, user.Role)

		response := map[string]interface{}{
			"success": true,
			"role":    user.Role,
			"message": fmt.Sprintf("Welcome %s!", user.Username),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	// Serve login page for GET requests
	if err := templates.ExecuteTemplate(w, "login.html", nil); err != nil {
		log.Printf("Template execution error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// ... [rest of the code remains the same]
