package main

import (
	"database/sql"
	"encoding/hex"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	_ "github.com/mattn/go-sqlite3"
	"go.bug.st/serial"
)

// All your existing structs remain the same
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
	ClockIn    time.Time `json:"clock_in"`
	ClockOut   time.Time `json:"clock_out"`
	TotalHours float64   `json:"total_hours"`
}

type CardScanEvent struct {
	DeviceID string    `json:"device_id"`
	CardUID  string    `json:"card_uid"`
	Time     time.Time `json:"time"`
}

// New RFID Reader struct
type RFIDReader struct {
	port     serial.Port
	logger   *log.Logger
	logFile  *os.File
	callback func(string)
}

var (
	db        *sql.DB
	logger    *log.Logger
	templates *template.Template
	upgrader  = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
)

// Initialize RFID Reader
func NewRFIDReader(portName string, callback func(string)) (*RFIDReader, error) {
	if err := os.MkdirAll("logs", 0755); err != nil {
		return nil, fmt.Errorf("failed to create logs directory: %v", err)
	}

	logFile, err := os.OpenFile(
		"logs/rfid_reader.log",
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0666,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %v", err)
	}

	mode := &serial.Mode{
		BaudRate: 9600,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	}

	port, err := serial.Open(portName, mode)
	if err != nil {
		logFile.Close()
		return nil, fmt.Errorf("failed to open serial port: %v", err)
	}

	port.SetReadTimeout(time.Millisecond * 100)

	return &RFIDReader{
		port:     port,
		logger:   log.New(logFile, "", log.Ldate|log.Ltime),
		logFile:  logFile,
		callback: callback,
	}, nil
}

func (r *RFIDReader) Close() {
	if r.port != nil {
		r.port.Close()
	}
	if r.logFile != nil {
		r.logFile.Close()
	}
}

func (r *RFIDReader) Read() {
	buffer := make([]byte, 64)
	cardData := make([]byte, 0, 64)
	isReading := false

	for {
		n, err := r.port.Read(buffer)
		if err != nil {
			r.logger.Printf("Error reading from port: %v", err)
			continue
		}

		if n > 0 {
			r.logger.Printf("Raw bytes received: %s", hex.Dump(buffer[:n]))

			for i := 0; i < n; i++ {
				b := buffer[i]

				if b == 0x02 {
					isReading = true
					cardData = cardData[:0]
					continue
				}

				if b == 0x03 {
					isReading = false
					if len(cardData) > 0 {
						r.processCardData(cardData)
					}
					continue
				}

				if isReading {
					cardData = append(cardData, b)
				}
			}
		}
	}
}

func (r *RFIDReader) processCardData(data []byte) {
	cardID := hex.EncodeToString(data)
	r.logger.Printf("Card ID (hex): %s", cardID)

	cleaned := make([]byte, 0, len(data))
	for _, b := range data {
		if (b >= '0' && b <= '9') || (b >= 'A' && b <= 'F') || (b >= 'a' && b <= 'f') {
			cleaned = append(cleaned, b)
		}
	}

	cleanedID := string(cleaned)
	r.logger.Printf("Cleaned Card ID: %s", cleanedID)

	if r.callback != nil {
		r.callback(cleanedID)
	}
}

// Your existing processCardScan function remains the same
func processCardScan(scan CardScanEvent) (*Employee, error) {
    // ... (your existing processCardScan code) ...
}

// Your existing initialization functions
func initDB() error {
    // ... (your existing initDB code) ...
}

func initLogger() {
    // ... (your existing initLogger code) ...
}

// Modified main function to include RFID reader
func main() {
	// Initialize logger
	initLogger()
	logger.Println("Starting time tracking application...")

	// Initialize database
	if err := initDB(); err != nil {
		log.Fatal("Failed to initialize database:", err)
	}
	defer db.Close()

	// Initialize RFID reader
	callback := func(cardID string) {
		scan := CardScanEvent{
			DeviceID: "JT308-001",
			CardUID:  cardID,
			Time:     time.Now(),
		}

		employee, err := processCardScan(scan)
		if err != nil {
			log.Printf("Error processing card scan: %v", err)
		} else {
			log.Printf("Successfully processed scan for employee: %s", employee.Name)
		}
	}

	// Replace "COM3" with your actual port name
	reader, err := NewRFIDReader("COM3", callback)
	if err != nil {
		log.Fatal(err)
	}
	defer reader.Close()

	// Start RFID reader in a goroutine
	go reader.Read()

	// Initialize web server
	r := mux.NewRouter()
	
	// Your existing routes
	r.HandleFunc("/", basicAuthMiddleware(dashboardHandler))
	r.HandleFunc("/dashboard", basicAuthMiddleware(dashboardHandler))
	r.HandleFunc("/employees", basicAuthMiddleware(employeesHandler))
	r.HandleFunc("/reports", basicAuthMiddleware(reportsHandler))
	r.HandleFunc("/clock", clockInOutHandler)

	// API routes
	api := r.PathPrefix("/api").Subrouter()
	api.HandleFunc("/employees", basicAuthMiddleware(apiGetEmployees)).Methods("GET")
	api.HandleFunc("/time-records", basicAuthMiddleware(apiGetTimeRecords)).Methods("GET")

	// Start web server
	log.Println("Starting server on :8080...")
	log.Fatal(http.ListenAndServe(":8080", r))
}
