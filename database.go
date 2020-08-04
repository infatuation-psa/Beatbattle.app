package main

import (
	"database/sql"
	"log"
	"os"
)

// dbConn ...
func dbInit() (*sql.DB, *sql.DB) {
	dbDriver := "mysql"
	dbName := os.Getenv("MYSQL_DB")
	dbUser := os.Getenv("MYSQL_USER")
	dbPass := os.Getenv("MYSQL_PASS")

	readServer := os.Getenv("MYSQL_READ")
	writeServer := os.Getenv("MYSQL_WRITE")

	dbWrite, err := sql.Open(dbDriver, dbUser+":"+dbPass+"@"+readServer+"/"+dbName+"?parseTime=true")
	if err != nil {
		log.Fatal(err)
	}

	dbRead, err := sql.Open(dbDriver, dbUser+":"+dbPass+"@"+writeServer+"/"+dbName+"?parseTime=true")
	if err != nil {
		log.Fatal(err)
	}

	// db.t2.small max connections is 150
	dbWrite.SetMaxOpenConns(150)
	dbRead.SetMaxOpenConns(150)

	/*
		db.SetMaxIdleConns(16)
		db.SetConnMaxLifetime(30 * time.Second)*/

	return dbRead, dbWrite
}

// RowExists ...
func RowExists(sqlStmt string, args ...interface{}) bool {
	var empty int
	err := dbRead.QueryRow(sqlStmt, args...).Scan(&empty)
	if err != nil {
		if err != sql.ErrNoRows {
			// Real error
			log.Print(err)
		}

		return false
	}

	return true
}
