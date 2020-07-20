package main

import (
	"database/sql"
	"log"
	"os"
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

	/* ACTUALLY MAKES THE CRASHING ISSUE WORSE LOL!!!
	db.SetMaxOpenConns(50)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetMaxOpenConns(4096)
	db.SetMaxIdleConns(256)
	db.SetConnMaxLifetime(5 * time.Minute)*/

	db.SetMaxOpenConns(2048)
	db.SetMaxIdleConns(128)
	db.SetConnMaxLifetime(0)

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
