package main

import (
	"database/sql"
	"log"
)

func createTables(db *sql.DB) {
	queryUsers := `CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		rfid_uid_original TEXT NOT NULL UNIQUE,
		rfid_uid_normalized TEXT NOT NULL UNIQUE
	)`
	_, err := db.Exec(queryUsers)
	if err != nil {
		log.Fatal("Error creating users table:", err)
	}

	queryClockInOut := `CREATE TABLE IF NOT EXISTS clock_in_out (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		rfid_uid_original TEXT NOT NULL,
		rfid_uid_normalized TEXT NOT NULL,
		user_id INTEGER,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id)
	)`
	_, err = db.Exec(queryClockInOut)
	if err != nil {
		log.Fatal("Error creating clock_in_out table:", err)
	}
}
