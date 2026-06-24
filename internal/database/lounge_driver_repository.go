package database

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/smarttransit/sms-auth-backend/internal/models"
)

// This handles the database operations for lounge driver
type LoungeDriverRepository struct {
	db *sqlx.DB
}

// create a new lounge driver repository
func NewLoungeDriverRepository(db *sqlx.DB) *LoungeDriverRepository {
	return &LoungeDriverRepository{db: db}
}

// add drivers to the lounge
func (r *LoungeDriverRepository) AddDriver(driver *models.LoungeDriver) (*models.LoungeDriver, error) {

	// setting auto-generated and default fields
	driver.ID = uuid.New()
	driver.CreatedAt = time.Now()
	driver.UpdatedAt = time.Now()

	if driver.Status == "" {
		driver.Status = models.DriverStatusActive
	}

	query := `
        INSERT INTO lounge_drivers (
            id, lounge_id, name, nic_number, contact_no,
            vehicle_no, vehicle_type, status, created_at, updated_at
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
        RETURNING id, created_at, updated_at
    `

	err := r.db.QueryRowx(query,
		driver.ID,
		driver.LoungeID,
		driver.Name,
		driver.NIC,
		driver.ContactNumber,
		driver.VehicleNumber,
		driver.VehicleType,
		driver.Status,
		driver.CreatedAt,
		driver.UpdatedAt,
	).Scan(&driver.ID, &driver.CreatedAt, &driver.UpdatedAt)

	if err != nil {
		log.Printf("ERROR: Failed to add driver for lounge %s: %v", driver.LoungeID, err)
		return nil, fmt.Errorf("failed to add driver: %w", err)
	}

	return driver, nil

}

// get drivers by the loungeID
func (r *LoungeDriverRepository) GetDriversByLoungeID(loungeID uuid.UUID) ([]models.LoungeDriver, error) {

	// creating a struct to hold the data
	var drivers []models.LoungeDriver

	query := `
			SELECT *
			FROM lounge_drivers
			WHERE lounge_id = $1
			ORDER BY created_at DESC`

	// querying the database inorder to extract the drivers to a specific lounge
	err := r.db.Select(&drivers, query, loungeID)
	// handling the NoRows error
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		log.Printf("ERROR: Failed to get drivers for the lounge %s: %v", loungeID, err)
		return nil, fmt.Errorf("failed to get drivers: %w", err)
	}

	return drivers, nil
}

// get lounge driver by driverID
func (r *LoungeDriverRepository) GetDriverByID(driverID uuid.UUID) (*models.LoungeDriver, error) {

	// creating the driver struct to hold data
	var driver models.LoungeDriver

	query :=
		`SELECT *
			  FROM lounge_drivers
			  WHERE id = $1`

	// querying the database to get the driver
	err := r.db.Get(&driver, query, driverID)
	if err != nil {
		log.Printf("ERROR: Failed to get driver %s: %v", driverID, err)
		return nil, fmt.Errorf("failed to get driver: %w", err)
	}
	if err == sql.ErrNoRows {
		return nil, nil
	}

	// returning the driver data
	return &driver, nil
}

// delete drivers by driverID
func (r *LoungeDriverRepository) DeleteDriver(driverID uuid.UUID) error {

	query :=
		`DELETE FROM lounge_drivers
			 WHERE id=$1`

	// executing the delete query
	result, err := r.db.Exec(query, driverID)

	if err != nil {
		log.Printf("ERROR: Failed to delete driver %s: %v", driverID, err)
		return fmt.Errorf("failed to delete driver: %w", err)
	}

	// checking if any row actually affected
	rowsAffected, _ := result.RowsAffected()

	if rowsAffected == 0 {
		return fmt.Errorf("driver not found: %s", driverID)
	}

	return nil

}

// update drivers using driverID
func (r *LoungeDriverRepository) UpdateDriver(driverID uuid.UUID, updates map[string]interface{}) error {

	// if no updates returning nil
	if len(updates) == 0 {
		return nil
	}

	// Build dynamic query
	query := "UPDATE lounge_drivers SET "
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
	args = append(args, driverID)

	// quarying the Database
	result, err := r.db.Exec(query, args...)
	if err != nil {
		log.Printf("ERROR: Failed to update driver %s: %v", driverID, err)
		return fmt.Errorf("failed to update driver: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("driver not found: %s", driverID)
	}

	return nil

}
