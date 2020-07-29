package main

import (
	"database/sql"
	"log"
	"os"
	"time"
)

// dbConn ...
func dbInit() (db *sql.DB) {
	dbDriver := "mysql"
	dbUser := os.Getenv("MYSQL_USER")
	dbPass := os.Getenv("MYSQL_PASS")
	dbName := os.Getenv("MYSQL_DB")

	db, err := sql.Open(dbDriver, dbUser+":"+dbPass+"@/"+dbName+"?parseTime=true")
	if err != nil {
		// Real error
		log.Print(err)
	}

	db.SetMaxOpenConns(256)
	db.SetMaxIdleConns(16)
	db.SetConnMaxLifetime(30 * time.Second)

	return db
}

// RowExists ...
func RowExists(sqlStmt string, args ...interface{}) bool {
	var empty int
	err := db.QueryRow(sqlStmt, args...).Scan(&empty)
	if err != nil {
		if err != sql.ErrNoRows {
			// Real error
			log.Print(err)
		}

		return false
	}

	return true
}
