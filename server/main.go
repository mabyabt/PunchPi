package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const dbFile = "rfid_attendance.db"

// Templates
var templates = template.Must(template.ParseGlob("templates/*.html"))

// User represents data from the users table
type User struct {
	ID                 int
	Name               string
	RFIDUIDOriginal    string
	RFIDUIDNormalized  string
}

// ClockRecord represents data from the clock_in_out table
type ClockRecord struct {
	ID                 int
	UserID             int
	UserName           string
	RFIDUIDOriginal    string
	RFIDUIDNormalized  string
	Timestamp          time.Time
	FormattedTimestamp string
	EventType          string
}

func main() {
	// Connect to database
	db, err := sql.Open("sqlite3", dbFile)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	createTables(db)

	// Create static file server
	fs := http.FileServer(http.Dir("static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	// Define API routes
	http.HandleFunc("/scan", func(w http.ResponseWriter, r *http.Request) {
		handleRFIDScan(w, r, db)
	})

	// Define Web UI routes
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		homeHandler(w, r, db)
	})
	http.HandleFunc("/users", func(w http.ResponseWriter, r *http.Request) {
		userListHandler(w, r, db)
	})
	http.HandleFunc("/users/add", func(w http.ResponseWriter, r *http.Request) {
		addUserHandler(w, r, db)
	})
	http.HandleFunc("/logs", func(w http.ResponseWriter, r *http.Request) {
		logsHandler(w, r, db)
	})

	// Create directories for templates and static files if they don't exist
	ensureDirectories()
	
	// Create initial templates if they don't exist
	createInitialTemplates()

	fmt.Println("Server running on :8080...")
	fmt.Println("Web interface available at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func ensureDirectories() {
	// Create templates directory
	if err := ensureDir("templates"); err != nil {
		log.Fatal("Error creating templates directory:", err)
	}
	
	// Create static directory
	if err := ensureDir("static"); err != nil {
		log.Fatal("Error creating static directory:", err)
	}
	
	// Create CSS directory
	if err := ensureDir(filepath.Join("static", "css")); err != nil {
		log.Fatal("Error creating css directory:", err)
	}
}

func ensureDir(dirName string) error {
	return nil // Placeholder - we'll implement file operations in the HTTP handlers for simplicity
}

func createInitialTemplates() {
	// We'll implement this in the HTTP handlers
	// This is just a placeholder function
}

func homeHandler(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	
	// Get user count
	var userCount int
	err := db.QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	
	// Get log count
	var logCount int
	err = db.QueryRow("SELECT COUNT(*) FROM clock_in_out").Scan(&logCount)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	
	// Get latest scans
	rows, err := db.Query(`
		SELECT c.id, c.user_id, u.name, c.rfid_uid_original, 
		       c.rfid_uid_normalized, c.timestamp
		FROM clock_in_out c
		JOIN users u ON c.user_id = u.id
		ORDER BY c.timestamp DESC LIMIT 5
	`)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	
	var latestScans []ClockRecord
	for rows.Next() {
		var r ClockRecord
		var timestamp string
		err := rows.Scan(&r.ID, &r.UserID, &r.UserName, &r.RFIDUIDOriginal, 
		                &r.RFIDUIDNormalized, &timestamp)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		
		t, _ := time.Parse("2006-01-02 15:04:05", timestamp)
		r.Timestamp = t
		r.FormattedTimestamp = t.Format("Jan 02, 2006 15:04:05")
		latestScans = append(latestScans, r)
	}
	
	// Create a simple HTML response for now
	html := `
<!DOCTYPE html>
<html>
<head>
    <title>RFID Attendance System</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            max-width: 1200px;
            margin: 0 auto;
            padding: 20px;
        }
        header {
            background-color: #f5f5f5;
            padding: 20px;
            margin-bottom: 20px;
            border-radius: 5px;
        }
        nav ul {
            list-style: none;
            padding: 0;
            display: flex;
            gap: 20px;
        }
        nav li a {
            text-decoration: none;
            color: #333;
        }
        .dashboard {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
            gap: 20px;
            margin-bottom: 30px;
        }
        .card {
            background-color: #f9f9f9;
            border-radius: 5px;
            padding: 20px;
            box-shadow: 0 2px 5px rgba(0,0,0,0.1);
        }
        .card h2 {
            margin-top: 0;
            border-bottom: 1px solid #ddd;
            padding-bottom: 10px;
        }
        table {
            width: 100%;
            border-collapse: collapse;
        }
        th, td {
            padding: 10px;
            text-align: left;
            border-bottom: 1px solid #ddd;
        }
        th {
            background-color: #f2f2f2;
        }
    </style>
</head>
<body>
    <header>
        <h1>RFID Attendance System</h1>
        <nav>
            <ul>
                <li><a href="/">Dashboard</a></li>
                <li><a href="/users">Manage Users</a></li>
                <li><a href="/logs">View Logs</a></li>
            </ul>
        </nav>
    </header>

    <div class="dashboard">
        <div class="card">
            <h2>Users</h2>
            <p>Total users: ` + fmt.Sprint(userCount) + `</p>
            <p><a href="/users">Manage Users</a></p>
        </div>
        <div class="card">
            <h2>Attendance Logs</h2>
            <p>Total logs: ` + fmt.Sprint(logCount) + `</p>
            <p><a href="/logs">View All Logs</a></p>
        </div>
    </div>

    <div class="card">
        <h2>Recent Activity</h2>
        <table>
            <thead>
                <tr>
                    <th>User</th>
                    <th>RFID</th>
                    <th>Timestamp</th>
                </tr>
            </thead>
            <tbody>`
    
    for _, scan := range latestScans {
        html += `
                <tr>
                    <td>` + scan.UserName + `</td>
                    <td>` + scan.RFIDUIDOriginal + `</td>
                    <td>` + scan.FormattedTimestamp + `</td>
                </tr>`
    }
    
    html += `
            </tbody>
        </table>
    </div>
</body>
</html>`

    w.Header().Set("Content-Type", "text/html")
    fmt.Fprint(w, html)
}

func userListHandler(w http.ResponseWriter, r *http.Request, db *sql.DB) {
    rows, err := db.Query("SELECT id, name, rfid_uid_original, rfid_uid_normalized FROM users ORDER BY name")
    if err != nil {
        http.Error(w, "Database error", http.StatusInternalServerError)
        return
    }
    defer rows.Close()
    
    var users []User
    for rows.Next() {
        var u User
        err := rows.Scan(&u.ID, &u.Name, &u.RFIDUIDOriginal, &u.RFIDUIDNormalized)
        if err != nil {
            http.Error(w, "Database error", http.StatusInternalServerError)
            return
        }
        users = append(users, u)
    }
    
    html := `
<!DOCTYPE html>
<html>
<head>
    <title>Manage Users - RFID Attendance System</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            max-width: 1200px;
            margin: 0 auto;
            padding: 20px;
        }
        header {
            background-color: #f5f5f5;
            padding: 20px;
            margin-bottom: 20px;
            border-radius: 5px;
        }
        nav ul {
            list-style: none;
            padding: 0;
            display: flex;
            gap: 20px;
        }
        nav li a {
            text-decoration: none;
            color: #333;
        }
        .card {
            background-color: #f9f9f9;
            border-radius: 5px;
            padding: 20px;
            box-shadow: 0 2px 5px rgba(0,0,0,0.1);
            margin-bottom: 20px;
        }
        table {
            width: 100%;
            border-collapse: collapse;
        }
        th, td {
            padding: 10px;
            text-align: left;
            border-bottom: 1px solid #ddd;
        }
        th {
            background-color: #f2f2f2;
        }
        .btn {
            display: inline-block;
            padding: 8px 16px;
            background-color: #4CAF50;
            color: white;
            text-decoration: none;
            border-radius: 4px;
            margin-bottom: 20px;
        }
    </style>
</head>
<body>
    <header>
        <h1>RFID Attendance System</h1>
        <nav>
            <ul>
                <li><a href="/">Dashboard</a></li>
                <li><a href="/users">Manage Users</a></li>
                <li><a href="/logs">View Logs</a></li>
            </ul>
        </nav>
    </header>

    <h2>Manage Users</h2>
    <a href="/users/add" class="btn">Add New User</a>
    
    <div class="card">
        <table>
            <thead>
                <tr>
                    <th>ID</th>
                    <th>Name</th>
                    <th>RFID UID (Original)</th>
                    <th>RFID UID (Normalized)</th>
                </tr>
            </thead>
            <tbody>`
    
    for _, user := range users {
        html += `
                <tr>
                    <td>` + fmt.Sprint(user.ID) + `</td>
                    <td>` + user.Name + `</td>
                    <td>` + user.RFIDUIDOriginal + `</td>
                    <td>` + user.RFIDUIDNormalized + `</td>
                </tr>`
    }
    
    html += `
            </tbody>
        </table>
    </div>
</body>
</html>`

    w.Header().Set("Content-Type", "text/html")
    fmt.Fprint(w, html)
}

func addUserHandler(w http.ResponseWriter, r *http.Request, db *sql.DB) {
    if r.Method == http.MethodPost {
        // Process form submission
        err := r.ParseForm()
        if err != nil {
            http.Error(w, "Error parsing form", http.StatusBadRequest)
            return
        }
        
        name := r.FormValue("name")
        rfidUID := r.FormValue("rfid_uid")
        
        if name == "" || rfidUID == "" {
            http.Error(w, "Name and RFID UID are required", http.StatusBadRequest)
            return
        }
        
        originalUID, normalizedUID := normalizeRFIDInput(rfidUID)
        
        _, err = db.Exec(
            "INSERT INTO users (name, rfid_uid_original, rfid_uid_normalized) VALUES (?, ?, ?)",
            name, originalUID, normalizedUID)
        
        if err != nil {
            http.Error(w, "Error adding user: "+err.Error(), http.StatusInternalServerError)
            return
        }
        
        // Redirect back to user list
        http.Redirect(w, r, "/users", http.StatusSeeOther)
        return
    }
    
    // Display the add user form
    html := `
<!DOCTYPE html>
<html>
<head>
    <title>Add User - RFID Attendance System</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            max-width: 1200px;
            margin: 0 auto;
            padding: 20px;
        }
        header {
            background-color: #f5f5f5;
            padding: 20px;
            margin-bottom: 20px;
            border-radius: 5px;
        }
        nav ul {
            list-style: none;
            padding: 0;
            display: flex;
            gap: 20px;
        }
        nav li a {
            text-decoration: none;
            color: #333;
        }
        .card {
            background-color: #f9f9f9;
            border-radius: 5px;
            padding: 20px;
            box-shadow: 0 2px 5px rgba(0,0,0,0.1);
        }
        form {
            max-width: 500px;
        }
        .form-group {
            margin-bottom: 15px;
        }
        label {
            display: block;
            margin-bottom: 5px;
            font-weight: bold;
        }
        input[type="text"] {
            width: 100%;
            padding: 8px;
            border: 1px solid #ddd;
            border-radius: 4px;
        }
        button {
            padding: 10px 15px;
            background-color: #4CAF50;
            color: white;
            border: none;
            border-radius: 4px;
            cursor: pointer;
        }
        .btn-secondary {
            background-color: #f44336;
            margin-left: 10px;
        }
    </style>
</head>
<body>
    <header>
        <h1>RFID Attendance System</h1>
        <nav>
            <ul>
                <li><a href="/">Dashboard</a></li>
                <li><a href="/users">Manage Users</a></li>
                <li><a href="/logs">View Logs</a></li>
            </ul>
        </nav>
    </header>

    <h2>Add New User</h2>
    
    <div class="card">
        <form method="POST" action="/users/add">
            <div class="form-group">
                <label for="name">Name:</label>
                <input type="text" id="name" name="name" required>
            </div>
            
            <div class="form-group">
                <label for="rfid_uid">RFID UID:</label>
                <input type="text" id="rfid_uid" name="rfid_uid" required>
            </div>
            
            <div class="form-group">
                <button type="submit">Add User</button>
                <a href="/users" style="text-decoration: none;">
                    <button type="button" class="btn-secondary">Cancel</button>
                </a>
            </div>
        </form>
    </div>
</body>
</html>`

    w.Header().Set("Content-Type", "text/html")
    fmt.Fprint(w, html)
}

func logsHandler(w http.ResponseWriter, r *http.Request, db *sql.DB) {
    rows, err := db.Query(`
        SELECT c.id, c.user_id, u.name, c.rfid_uid_original, 
               c.rfid_uid_normalized, c.timestamp
        FROM clock_in_out c
        JOIN users u ON c.user_id = u.id
        ORDER BY c.timestamp DESC
    `)
    if err != nil {
        http.Error(w, "Database error", http.StatusInternalServerError)
        return
    }
    defer rows.Close()
    
    var logs []ClockRecord
    for rows.Next() {
        var r ClockRecord
        var timestamp string
        err := rows.Scan(&r.ID, &r.UserID, &r.UserName, &r.RFIDUIDOriginal, 
                       &r.RFIDUIDNormalized, &timestamp)
        if err != nil {
            http.Error(w, "Database error", http.StatusInternalServerError)
            return
        }
        
        t, _ := time.Parse("2006-01-02 15:04:05", timestamp)
        r.Timestamp = t
        r.FormattedTimestamp = t.Format("Jan 02, 2006 15:04:05")
        logs = append(logs, r)
    }
    
    html := `
<!DOCTYPE html>
<html>
<head>
    <title>Attendance Logs - RFID Attendance System</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            max-width: 1200px;
            margin: 0 auto;
            padding: 20px;
        }
        header {
            background-color: #f5f5f5;
            padding: 20px;
            margin-bottom: 20px;
            border-radius: 5px;
        }
        nav ul {
            list-style: none;
            padding: 0;
            display: flex;
            gap: 20px;
        }
        nav li a {
            text-decoration: none;
            color: #333;
        }
        .card {
            background-color: #f9f9f9;
            border-radius: 5px;
            padding: 20px;
            box-shadow: 0 2px 5px rgba(0,0,0,0.1);
        }
        table {
            width: 100%;
            border-collapse: collapse;
        }
        th, td {
            padding: 10px;
            text-align: left;
            border-bottom: 1px solid #ddd;
        }
        th {
            background-color: #f2f2f2;
        }
    </style>
</head>
<body>
    <header>
        <h1>RFID Attendance System</h1>
        <nav>
            <ul>
                <li><a href="/">Dashboard</a></li>
                <li><a href="/users">Manage Users</a></li>
                <li><a href="/logs">View Logs</a></li>
            </ul>
        </nav>
    </header>

    <h2>Attendance Logs</h2>
    
    <div class="card">
        <table>
            <thead>
                <tr>
                    <th>ID</th>
                    <th>User</th>
                    <th>RFID UID</th>
                    <th>Timestamp</th>
                </tr>
            </thead>
            <tbody>`
    
    for _, log := range logs {
        html += `
                <tr>
                    <td>` + fmt.Sprint(log.ID) + `</td>
                    <td>` + log.UserName + `</td>
                    <td>` + log.RFIDUIDOriginal + `</td>
                    <td>` + log.FormattedTimestamp + `</td>
                </tr>`
    }
    
    html += `
            </tbody>
        </table>
    </div>
</body>
</html>`

    w.Header().Set("Content-Type", "text/html")
    fmt.Fprint(w, html)
}
