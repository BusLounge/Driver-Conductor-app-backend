//go:build ignore
// +build ignore

package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/lib/pq"
)

func main() {
	db, err := sql.Open("postgres", "postgresql://postgres.pttatcukzpceljcrwehk:KQ95tJUYdFX251VR@aws-1-us-east-1.pooler.supabase.com:5432/postgres")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT column_name, data_type, is_nullable, column_default 
		FROM information_schema.columns 
		WHERE table_name = 'lounge_booking_driver_assignments'
		ORDER BY column_name
	`)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	fmt.Println("Columns in lounge_booking_driver_assignments:")
	for rows.Next() {
		var colName, dataType, isNullable string
		var colDefault sql.NullString
		if err := rows.Scan(&colName, &dataType, &isNullable, &colDefault); err != nil {
			log.Fatal(err)
		}
		defVal := "NULL"
		if colDefault.Valid {
			defVal = colDefault.String
		}
		fmt.Printf("- %s: %s (nullable: %s, default: %s)\n", colName, dataType, isNullable, defVal)
	}
}
