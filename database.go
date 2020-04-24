package main

import (
	"database/sql"
	"log"
	"os"
)

// dbConn ...
func dbConn() (db *sql.DB) {
	dbDriver := "mysql"
	dbUser := os.Getenv("MYSQL_USER")
	dbPass := os.Getenv("MYSQL_PASS")
	dbName := os.Getenv("MYSQL_DB")
	db, err := sql.Open(dbDriver, dbUser+":"+dbPass+"@/"+dbName+"?parseTime=true")
	if err != nil {
		// Real error
		log.Print(err)
	}
	return db
}

// RowExists ...
func RowExists(db *sql.DB, sqlStmt string, args ...interface{}) bool {
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
