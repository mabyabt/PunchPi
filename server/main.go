package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"

	_ "github.com/mattn/go-sqlite3"
)

const dbFile = "rfid_attendance.db"

func main() {
	// Connect to database
	db, err := sql.Open("sqlite3", dbFile)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	createTables(db)

	// Define API routes
	http.HandleFunc("/scan", func(w http.ResponseWriter, r *http.Request) {
		handleRFIDScan(w, r, db)
	})

	fmt.Println("Server running on :8080...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
