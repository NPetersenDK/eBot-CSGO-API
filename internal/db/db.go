package db

import (
	"database/sql"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// Open connects to the shared eBot MySQL database and verifies the connection.
func Open(dsn string) (*sql.DB, error) {
	d, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	d.SetConnMaxLifetime(5 * time.Minute)
	d.SetMaxOpenConns(10)
	d.SetMaxIdleConns(5)

	if err := d.Ping(); err != nil {
		d.Close()
		return nil, err
	}
	return d, nil
}
