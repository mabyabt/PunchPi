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

	_ "github.com/mattn/go-sqlite3"
)

var (
	db        *sql.DB
	templates *template.Template
)

// User type definition
type User struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Password string `json:"-"` // "-" means this field won't be included in JSON
	Role     string `json:"role"`
}

// Initialize templates
func initTemplates() error {
	log.Println("Initializing templates...")
	
	// Get absolute path to templates directory
	templatesDir, err := filepath.Abs("templates")
	if err != nil {
		return fmt.Errorf("error getting template directory path: %w", err)
	}

	// Create templates directory if it doesn't exist
	if err := os.MkdirAll(templatesDir, 0755); err != nil {
		return fmt.Errorf("failed to create templates directory: %w", err)
	}

	// Parse templates with error checking
	templates, err = template.ParseGlob(filepath.Join(templatesDir, "*.html"))
	if err != nil {
		return fmt.Errorf("error parsing templates: %w", err)
	}

	log.Printf("Templates loaded successfully from: %s", templatesDir)
	return nil
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
		return fmt.Errorf("failed to create database directory: %w", err)
	}
	log.Println("Database directory created/verified at ./db")

	dbPath := filepath.Join(dbDir, "timetrack.db")
	var err error
	db, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Ensure database is closed if initialization fails
	defer func() {
		if err != nil {
			db.Close()
		}
	}()

	// Test database connection
	if err := db.Ping(); err != nil {
		return fmt.Errorf("database connection test failed: %w", err)
	}
	log.Println("Database connection test successful")

	// Create tables
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

	-- Create indexes for frequently queried columns
	CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
	CREATE INDEX IF NOT EXISTS idx_employees_card_uid ON employees(card_uid);
	CREATE INDEX IF NOT EXISTS idx_time_records_employee_id ON time_records(employee_id);`

	if _, err = db.Exec(createTables); err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}
	log.Println("Database tables created successfully")

	// Check if admin user exists
	var count int
	if err = db.QueryRow("SELECT COUNT(*) FROM users WHERE username = 'admin'").Scan(&count); err != nil {
		return fmt.Errorf("failed to check admin user: %w", err)
	}

	// Create default admin user if it doesn't exist
	if count == 0 {
		hashedPassword := hashPassword("admin")
		if _, err = db.Exec(`
			INSERT INTO users (username, password, role) 
			VALUES (?, ?, ?)`,
			"admin", hashedPassword, "admin"); err != nil {
			return fmt.Errorf("failed to create admin user: %w", err)
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
	// Check for valid HTTP methods
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if r.Method == http.MethodPost {
		username := r.FormValue("username")
		password := r.FormValue("password")

		// Basic input validation
		if username == "" || password == "" {
			http.Error(w, "Username and password are required", http.StatusBadRequest)
			return
		}

		hashedPassword := hashPassword(password)

		var user User
		ctx := r.Context()
		err := db.QueryRowContext(ctx, `
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

		response := map[string]interface{}{
			"success": true,
			"role":    user.Role,
			"message": fmt.Sprintf("Welcome %s!", user.Username),
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Printf("Error encoding JSON response: %v", err)
			http.Error(w, "Server error", http.StatusInternalServerError)
			return
		}
		return
	}

	// Serve login page for GET requests
	if err := templates.ExecuteTemplate(w, "login.html", nil); err != nil {
		log.Printf("Template execution error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

func main() {
	// Initialize templates
	if err := initTemplates(); err != nil {
		log.Fatalf("Failed to initialize templates: %v", err)
	}

	// Initialize database
	if err := initDB(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Set up HTTP routes
	http.HandleFunc("/login", loginHandler)

	// Start the server
	log.Println("Starting server on :8080...")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
