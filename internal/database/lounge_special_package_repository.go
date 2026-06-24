package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/smarttransit/sms-auth-backend/internal/models"
)

// LoungeSpecialPackageRepository handles DB operations for lounge special packages
type LoungeSpecialPackageRepository struct {
	db DB
}

// NewLoungeSpecialPackageRepository creates a new repository
func NewLoungeSpecialPackageRepository(db DB) *LoungeSpecialPackageRepository {
	return &LoungeSpecialPackageRepository{db: db}
}

// jsonOrNull returns a *json.RawMessage suitable for a nullable JSONB column.
// nil / empty input → sql.NullString{Valid: false}, so the DB gets NULL.
func jsonOrNull(raw json.RawMessage) interface{} {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return []byte(raw)
}

// scanPackageRow scans a full row (all columns) into a LoungeSpecialPackage.
// Column order must match the SELECT list used in every query below.
func scanPackageRow(row interface {
	Scan(dest ...interface{}) error
}, pkg *models.LoungeSpecialPackage) error {
	var (
		breakfastType    []byte
		lunchType        []byte
		eveningSnackType []byte
		dinnerType       []byte
		places          []byte
		transportModeStr sql.NullString
	)

	err := row.Scan(
		&pkg.ID,
		&pkg.LoungeID,
		&pkg.PackageName,
		&pkg.ImageURL,
		&pkg.PackageType,
		&pkg.Description,
		&pkg.Price,
		&pkg.IsActive,
		&pkg.CreatedAt,
		&pkg.UpdatedAt,
		&pkg.Pax,
		&pkg.TransportStatus,
		&transportModeStr,
		&pkg.MealStatus,
		&pkg.BreakfastStatus,
		&breakfastType,
		&pkg.LunchStatus,
		&lunchType,
		&pkg.EveningSnackStatus,
		&eveningSnackType,
		&pkg.DinnerStatus,
		&dinnerType,
		&places,
	)
	if err != nil {
		return err
	}

	// Nullable string → pointer
	if transportModeStr.Valid {
		pkg.TransportMode = &transportModeStr.String
	}

	// JSONB bytes → json.RawMessage
	if len(breakfastType) > 0 {
		pkg.BreakfastType = json.RawMessage(breakfastType)
	}
	if len(lunchType) > 0 {
		pkg.LunchType = json.RawMessage(lunchType)
	}
	if len(eveningSnackType) > 0 {
		pkg.EveningSnackType = json.RawMessage(eveningSnackType)
	}
	if len(dinnerType) > 0 {
		pkg.DinnerType = json.RawMessage(dinnerType)
	}
	if len(places) > 0 {
		pkg.Places = json.RawMessage(places)
	}

	return nil
}

// fullSelectCols is the shared SELECT column list (order must match scanPackageRow).
const fullSelectCols = `
	id, lounge_id, package_name, image_url, package_type,
	description, price, is_active, created_at, updated_at,
	pax, transport_status, transport_mode,
	"meal-status", "breakfast-status", "breakfast-type",
	"lunch-status", "lunch-type",
	"evening-snack-status", "evening-snack-type",
	"dinner-status", "dinner-type",
	places`

// CreateSpecialPackage inserts a new special package into the database
func (r *LoungeSpecialPackageRepository) CreateSpecialPackage(pkg *models.LoungeSpecialPackage) error {
	pkg.ID = uuid.New()
	pkg.CreatedAt = time.Now()
	pkg.UpdatedAt = time.Now()
	pkg.IsActive = true

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

	_, err := r.db.Exec(
		query,
		pkg.ID,
		pkg.LoungeID,
		pkg.PackageName,
		pkg.ImageURL,
		string(pkg.PackageType),
		pkg.Description,
		pkg.Price,
		pkg.IsActive,
		pkg.CreatedAt,
		pkg.UpdatedAt,
		pkg.Pax,
		pkg.TransportStatus,
		pkg.TransportMode,
		pkg.MealStatus,
		pkg.BreakfastStatus,
		jsonOrNull(pkg.BreakfastType),
		pkg.LunchStatus,
		jsonOrNull(pkg.LunchType),
		pkg.EveningSnackStatus,
		jsonOrNull(pkg.EveningSnackType),
		pkg.DinnerStatus,
		jsonOrNull(pkg.DinnerType),
		jsonOrNull(pkg.Places),
	)
	if err != nil {
		return fmt.Errorf("create special package: %w", err)
	}

	log.Printf("INFO: Created special package %s for lounge %s", pkg.ID, pkg.LoungeID)
	return nil
}

// GetSpecialPackagesByLoungeID retrieves all active special packages for a lounge
func (r *LoungeSpecialPackageRepository) GetSpecialPackagesByLoungeID(loungeID uuid.UUID) ([]models.LoungeSpecialPackage, error) {
	query := `
		SELECT` + fullSelectCols + `
		FROM lounge_special_packages
		WHERE lounge_id = $1 AND is_active = TRUE
		ORDER BY
			CASE package_type
				WHEN 'platinum' THEN 1
				WHEN 'gold'     THEN 2
				WHEN 'standard' THEN 3
				ELSE 4
			END,
			created_at DESC`

	rows, err := r.db.Query(query, loungeID)
	if err != nil {
		return nil, fmt.Errorf("query special packages: %w", err)
	}
	defer rows.Close()

	var packages []models.LoungeSpecialPackage
	for rows.Next() {
		var pkg models.LoungeSpecialPackage
		if err := scanPackageRow(rows, &pkg); err != nil {
			return nil, fmt.Errorf("scan special package: %w", err)
		}
		packages = append(packages, pkg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return packages, nil
}

// GetSpecialPackageByID retrieves a single special package by ID
func (r *LoungeSpecialPackageRepository) GetSpecialPackageByID(pkgID uuid.UUID) (*models.LoungeSpecialPackage, error) {
	query := `
		SELECT` + fullSelectCols + `
		FROM lounge_special_packages
		WHERE id = $1`

	var pkg models.LoungeSpecialPackage
	row := r.db.QueryRow(query, pkgID)
	if err := scanPackageRow(row, &pkg); err != nil {
		return nil, fmt.Errorf("get special package by id: %w", err)
	}
	return &pkg, nil
}

// UpdateSpecialPackage updates an existing special package
func (r *LoungeSpecialPackageRepository) UpdateSpecialPackage(pkg *models.LoungeSpecialPackage) error {
	pkg.UpdatedAt = time.Now()

	query := `
		UPDATE lounge_special_packages
		SET package_name           = $1,
		    image_url               = $2,
		    package_type            = $3,
		    description             = $4,
		    price                   = $5,
		    pax                     = $6,
		    transport_status        = $7,
		    transport_mode          = $8,
		    "meal-status"           = $9,
		    "breakfast-status"      = $10,
		    "breakfast-type"        = $11,
		    "lunch-status"          = $12,
		    "lunch-type"            = $13,
		    "evening-snack-status" = $14,
		    "evening-snack-type"   = $15,
		    "dinner-status"         = $16,
		    "dinner-type"           = $17,
		    places                  = $18,
		    updated_at              = $19
		WHERE id = $20`

	result, err := r.db.Exec(
		query,
		pkg.PackageName,
		pkg.ImageURL,
		string(pkg.PackageType),
		pkg.Description,
		pkg.Price,
		pkg.Pax,
		pkg.TransportStatus,
		pkg.TransportMode,
		pkg.MealStatus,
		pkg.BreakfastStatus,
		jsonOrNull(pkg.BreakfastType),
		pkg.LunchStatus,
		jsonOrNull(pkg.LunchType),
		pkg.EveningSnackStatus,
		jsonOrNull(pkg.EveningSnackType),
		pkg.DinnerStatus,
		jsonOrNull(pkg.DinnerType),
		jsonOrNull(pkg.Places),
		pkg.UpdatedAt,
		pkg.ID,
	)
	if err != nil {
		return fmt.Errorf("update special package: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("special package not found: %s", pkg.ID)
	}
	return nil
}

// DeleteSpecialPackage soft-deletes a special package (sets is_active = false)
func (r *LoungeSpecialPackageRepository) DeleteSpecialPackage(pkgID uuid.UUID) error {
	query := `
		UPDATE lounge_special_packages
		SET is_active = FALSE, updated_at = $1
		WHERE id = $2`

	result, err := r.db.Exec(query, time.Now(), pkgID)
	if err != nil {
		return fmt.Errorf("delete special package: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("special package not found: %s", pkgID)
	}
	return nil
}
