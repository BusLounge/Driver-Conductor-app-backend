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
	_ "github.com/lib/pq"
)

func jsonOrNull(raw json.RawMessage) interface{} {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return []byte(raw)
}

func main() {
	db, err := sql.Open("postgres", "postgresql://postgres.pttatcukzpceljcrwehk:KQ95tJUYdFX251VR@aws-1-us-east-1.pooler.supabase.com:5432/postgres")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Check if lounge exists
	loungeID := "e528941b-3c29-4b4e-87e8-6c2ae4ea8c34"
	var exists bool
	err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM lounges WHERE id = $1)", loungeID).Scan(&exists)
	if err != nil {
		log.Fatalf("Error checking lounge existence: %v", err)
	}
	fmt.Printf("Lounge %s exists: %v\n", loungeID, exists)

	// Attempt insertion using the same query as CreateSpecialPackage
	query := `
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

	id := uuid.New()
	packageName := "Test Premium Package"
	var imageUrl *string
	packageType := "platinum"
	description := "Test description"
	price := "1500.00"
	isActive := true
	createdAt := time.Now()
	updatedAt := time.Now()
	var pax int64 = 2
	transportStatus := true
	
	// Test pointer
	transportModeStr := "three-wheeler"
	transportMode := &transportModeStr
	
	mealStatus := true
	breakfastStatus := true
	breakfastType := json.RawMessage(`{"type": "English"}`)
	lunchStatus := false
	var lunchType json.RawMessage
	eveningSnackStatus := false
	var eveningSnackType json.RawMessage
	dinnerStatus := false
	var dinnerType json.RawMessage
	places := json.RawMessage(`["Temple", "Lake"]`)

	_, err = db.Exec(
		query,
		id,
		loungeID,
		packageName,
		imageUrl,
		packageType,
		description,
		price,
		isActive,
		createdAt,
		updatedAt,
		pax,
		transportStatus,
		transportMode, // Passing *string
		mealStatus,
		breakfastStatus,
		jsonOrNull(breakfastType),
		lunchStatus,
		jsonOrNull(lunchType),
		eveningSnackStatus,
		jsonOrNull(eveningSnackType),
		dinnerStatus,
		jsonOrNull(dinnerType),
		jsonOrNull(places),
	)
	if err != nil {
		fmt.Printf("EXECUTION ERROR: %v\n", err)
	} else {
		fmt.Println("INSERTION SUCCESSFUL!")
		// Clean up
		_, _ = db.Exec("DELETE FROM lounge_special_packages WHERE id = $1", id)
	}
}
