package database

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	// "github.com/pelletier/go-toml/query"
	"github.com/smarttransit/sms-auth-backend/internal/models"
)

// Lounge Transport Location Repository handles database operations of lounge transport locations
type LoungeTransportLocationRepository struct {
	db *sqlx.DB
}

// NewLoungeTransportLocationRepository creates new lounge transport location repository
func NewLoungeTransportLocationRepository(db *sqlx.DB) *LoungeTransportLocationRepository {
	return &LoungeTransportLocationRepository{db: db}
}

// Add Transport Locations for lounge
func (r *LoungeTransportLocationRepository) AddTransportLocationToLounge(TransportLocation models.LoungeTransportLocation) (*models.LoungeTransportLocation, error) {

	// setting auto-generated and default fields
	TransportLocation.ID = uuid.New()
	TransportLocation.CreatedAt = time.Now()
	TransportLocation.UpdatedAt = time.Now()

	// updating the status for the location default is active
	if TransportLocation.Status == "" {
		TransportLocation.Status = models.LoungeTransportLocationStatusActive
	}

	// setting up the query to run
	query := `INSERT INTO lounge_transport_locations (
			 id, lounge_id, location, latitude, longitude, status, created_at, updated_at, est_duration, distance)
			 VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			 RETURNING id, created_at, updated_at`

	err := r.db.QueryRowx(query,
		TransportLocation.ID,
		TransportLocation.LoungeID,
		TransportLocation.Location,
		TransportLocation.Latitude,
		TransportLocation.Longitude,
		TransportLocation.Status,
		TransportLocation.CreatedAt,
		TransportLocation.UpdatedAt,
		TransportLocation.EstDuration,
		TransportLocation.Distance,
	).Scan(&TransportLocation.ID, &TransportLocation.CreatedAt, &TransportLocation.UpdatedAt)

	if err != nil {
		log.Printf("ERROR: Failed to add location for lounge %s: %v", TransportLocation.LoungeID, err)
		return nil, fmt.Errorf("failed to add location: %w", err)
	}

	// if okay sending the created transport location data
	return &TransportLocation, nil
}

func (r *LoungeTransportLocationRepository) GetLoungeTransportLocationByID(locationID uuid.UUID) (*models.LoungeTransportLocation, error) {

	// creating a var to store the location
	var location models.LoungeTransportLocation

	query := `SELECT
				id,
				lounge_id,
				location,
				latitude,
				longitude,
				status,
				created_at,
				updated_at,
				COALESCE(est_duration, 0) AS est_duration,
				COALESCE(distance, 0) AS distance
			  FROM lounge_transport_locations
			  WHERE id = $1`

	// executing the data
	err := r.db.Get(&location, query, locationID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		log.Printf("ERROR: Failed to get transport location for id %s: %v", locationID, err)
		return nil, fmt.Errorf("failed to get location: %w", err)
	}

	return &location, nil
}

func (r *LoungeTransportLocationRepository) GetLoungeTransportLocationsByLoungeID(loungeID uuid.UUID) ([]models.LoungeTransportLocation, error) {

	// creating a struct to hold the data
	var loungeTransportLocations []models.LoungeTransportLocation

	query := `SELECT
				id,
				lounge_id,
				location,
				latitude,
				longitude,
				status,
				created_at,
				updated_at,
				COALESCE(est_duration, 0) AS est_duration,
				COALESCE(distance, 0) AS distance
			  FROM lounge_transport_locations 
			  WHERE lounge_id = $1
			  ORDER BY created_at DESC`

	// querying the database inorder to extract the lounge transport locations to a specific lounge
	err := r.db.Select(&loungeTransportLocations, query, loungeID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		log.Printf("ERROR: Failed to get transport locations for the lounge %s: %v", loungeID, err)
		return nil, fmt.Errorf("failed to get locations: %w", err)
	}

	// if no error occured pass the data got
	return loungeTransportLocations, nil
}

func (r *LoungeTransportLocationRepository) DeleteLoungeTransportLocationByID(locationID uuid.UUID) error {

	query := `DELETE FROM lounge_transport_locations
			  WHERE id = $1`

	// executing delete query
	result, err := r.db.Exec(query, locationID)

	if err != nil {
		log.Printf("ERROR: Failed to delete location %s: %v", locationID, err)
		return fmt.Errorf("failed to delete location: %w", err)
	}

	// checking for rows affected
	rowsAffected, _ := result.RowsAffected()

	if rowsAffected == 0 {
		return fmt.Errorf("location not found: %s", locationID)
	}

	return nil
}

func (r *LoungeTransportLocationRepository) UpdateLoungeTransportLocationByID(locationID uuid.UUID, updates map[string]interface{}) error {

	// if no updates returning nil
	if len(updates) == 0 {
		return nil
	}

	// Build dynamic query
	query := "UPDATE lounge_transport_locations SET "
	var args []interface{}
	var placeholders []string
	argIndex := 1

	// iterating through the values to append the key , value pairs
	for column, value := range updates {
		placeholders = append(placeholders, fmt.Sprintf("%s = $%d", column, argIndex))
		args = append(args, value)
		argIndex++
	}

	query += strings.Join(placeholders, ", ")
	query += fmt.Sprintf(", updated_at = NOW() WHERE id = $%d", argIndex)
	args = append(args, locationID)

	// quarying the Database
	result, err := r.db.Exec(query, args...)
	if err != nil {
		log.Printf("ERROR: Failed to update transport location %s: %v", locationID, err)
		return fmt.Errorf("failed to update location: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("location not found: %s", locationID)
	}

	return nil
}
