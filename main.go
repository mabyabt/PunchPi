// Add these additional functions to your code:

// Basic Auth Middleware implementation
func basicAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        user, pass, ok := r.BasicAuth()
        if !ok || user != "admin" || pass != "password" { // Change these credentials
            w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
            http.Error(w, "Unauthorized", http.StatusUnauthorized)
            return
        }
        next.ServeHTTP(w, r)
    }
}

// Template handlers
func dashboardHandler(w http.ResponseWriter, r *http.Request) {
    data := struct {
        Title string
        Time  time.Time
    }{
        Title: "Dashboard",
        Time:  time.Now(),
    }
    err := templates.ExecuteTemplate(w, "dashboard.html", data)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
    }
}

func employeesHandler(w http.ResponseWriter, r *http.Request) {
    data := struct {
        Title string
    }{
        Title: "Employees",
    }
    err := templates.ExecuteTemplate(w, "employees.html", data)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
    }
}

func reportsHandler(w http.ResponseWriter, r *http.Request) {
    data := struct {
        Title string
    }{
        Title: "Reports",
    }
    err := templates.ExecuteTemplate(w, "reports.html", data)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
    }
}

func clockInOutHandler(w http.ResponseWriter, r *http.Request) {
    data := struct {
        Title string
    }{
        Title: "Clock In/Out",
    }
    err := templates.ExecuteTemplate(w, "clock.html", data)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
    }
}

// Modified RFID reader initialization with retry
func initRFIDReader(portName string, callback func(string)) (*RFIDReader, error) {
    var reader *RFIDReader
    var err error
    maxRetries := 5
    
    for i := 0; i < maxRetries; i++ {
        reader, err = NewRFIDReader(portName, callback)
        if err == nil {
            return reader, nil
        }
        log.Printf("Failed to initialize RFID reader (attempt %d/%d): %v", i+1, maxRetries, err)
        time.Sleep(time.Second * 2)
    }
    return nil, fmt.Errorf("failed to initialize RFID reader after %d attempts: %v", maxRetries, err)
}

// Modified main function with better error handling
func main() {
    // Initialize logger
    initLogger()
    logger.Println("Starting time tracking application...")

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

    // Initialize RFID reader with retry
    callback := func(cardID string) {
        scan := CardScanEvent{
            DeviceID: "JT308-001",
            CardUID:  cardID,
            Time:     time.Now(),
        }

        employee, err := processCardScan(scan)
        if err != nil {
            log.Printf("Error processing card scan: %v", err)
            return
        }
        log.Printf("Successfully processed scan for employee: %s", employee.Name)
    }

    // Detect OS and set appropriate port name
    portName := "COM3" // Default for Windows
    if os.Getenv("GOOS") == "linux" {
        portName = "/dev/ttyUSB0"
    } else if os.Getenv("GOOS") == "darwin" {
        portName = "/dev/tty.usbserial-*"
    }

    // Override port name if specified in environment
    if envPort := os.Getenv("RFID_PORT"); envPort != "" {
        portName = envPort
    }

    reader, err := initRFIDReader(portName, callback)
    if err != nil {
        log.Fatalf("Failed to initialize RFID reader: %v", err)
    }
    defer reader.Close()

    // Start RFID reader in a goroutine with recovery
    go func() {
        defer func() {
            if r := recover(); r != nil {
                log.Printf("RFID reader panic recovered: %v", r)
            }
        }()
        reader.Read()
    }()

    // Create router
    r := mux.NewRouter()

    // Static file server
    r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

    // Routes
    r.HandleFunc("/", basicAuthMiddleware(dashboardHandler))
    r.HandleFunc("/dashboard", basicAuthMiddleware(dashboardHandler))
    r.HandleFunc("/employees", basicAuthMiddleware(employeesHandler))
    r.HandleFunc("/reports", basicAuthMiddleware(reportsHandler))
    r.HandleFunc("/clock", clockInOutHandler)

    // API routes
    api := r.PathPrefix("/api").Subrouter()
    api.HandleFunc("/employees", basicAuthMiddleware(apiGetEmployees)).Methods("GET")
    api.HandleFunc("/time-records", basicAuthMiddleware(apiGetTimeRecords)).Methods("GET")

    // Server configuration with timeouts
    srv := &http.Server{
        Handler:      r,
        Addr:         ":8080",
        WriteTimeout: 15 * time.Second,
        ReadTimeout:  15 * time.Second,
    }

    // Start server
    log.Println("Starting server on :8080...")
    log.Fatal(srv.ListenAndServe())
}
