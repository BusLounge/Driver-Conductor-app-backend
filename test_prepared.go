//go:build ignore
// +build ignore

package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func jsonOrNull(raw json.RawMessage) interface{} {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return []byte(raw)
}

func main() {
	connectionURL := "postgresql://postgres.pttatcukzpceljcrwehk:KQ95tJUYdFX251VR@aws-1-us-east-1.pooler.supabase.com:5432/postgres?sslmode=require"

	pgxConfig, err := pgx.ParseConfig(connectionURL)
	if err != nil {
		log.Fatalf("failed to parse database URL: %v", err)
	}

	connStr := stdlib.RegisterConnConfig(pgxConfig)
	db, err := sql.Open("pgx", connStr)
	if err != nil {
		log.Fatalf("failed to connect: %v", err)
	}
	defer db.Close()

	queryNoCast := `
		INSERT INTO lounge_special_packages (
			id, lounge_id, package_name, image_url, package_type,
			description, price, is_active, created_at, updated_at,
			pax, transport_status, transport_mode,
			"meal-status", "breakfast-status", "breakfast-type",
			"lunch-status", "lunch-type",
			"evening-snack-status", "evening-snack-type",
			"dinner-status", "dinner-type",
			places
		) VALUES (
			$1,  $2,  $3,  $4,  $5,
			$6,  $7,  $8,  $9,  $10,
			$11, $12, $13,
			$14, $15, $16,
			$17, $18,
			$19, $20,
			$21, $22,
			$23
		)`

	stmt, err := db.Prepare(queryNoCast)
	if err != nil {
		log.Fatalf("PREPARATION ERROR: %v\n", err)
	}
	defer stmt.Close()

	id := uuid.New()
	loungeID := "e528941b-3c29-4b4e-87e8-6c2ae4ea8c34"
	transportMode := "three-wheeler"
	breakfastType := json.RawMessage(`["Continental"]`)

	_, err = stmt.Exec(
		id, loungeID, "Test Prepared Non-Nil JSON", nil, "standard", "Desc", "100.00", true, time.Now(), time.Now(),
		nil, true, transportMode,
		true, true, jsonOrNull(breakfastType),
		nil, nil, nil, nil, nil, nil, nil,
	)
	if err != nil {
		fmt.Printf("EXECUTION ERROR: %v\n", err)
	} else {
		fmt.Println("INSERTION SUCCESSFUL!")
		_, _ = db.Exec("DELETE FROM lounge_special_packages WHERE id = $1", id)
	}
}
