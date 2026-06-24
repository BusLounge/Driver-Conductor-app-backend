package database

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/smarttransit/sms-auth-backend/internal/models"
)

// LoungeOwnerDistrictRepository handles lounge_owner_districts table operations.
type LoungeOwnerDistrictRepository struct {
	db DB
}

// NewLoungeOwnerDistrictRepository creates a new repository instance.
func NewLoungeOwnerDistrictRepository(db DB) *LoungeOwnerDistrictRepository {
	return &LoungeOwnerDistrictRepository{db: db}
}

// Create inserts a new lounge owner district record and returns the created row.
func (r *LoungeOwnerDistrictRepository) Create(record *models.LoungeOwnerDistrict) (*models.LoungeOwnerDistrict, error) {
	query := `
		INSERT INTO lounge_owner_districts (
			id,
			owner_id,
			district_id,
			owner_name,
			business_name
		)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, owner_id, district_id, owner_name, business_name
	`

	created := &models.LoungeOwnerDistrict{}
	err := r.db.QueryRow(
		query,
		record.ID,
		record.OwnerID,
		record.DistrictID,
		record.OwnerName,
		record.BusinessName,
	).Scan(
		&created.ID,
		&created.OwnerID,
		&created.DistrictID,
		&created.OwnerName,
		&created.BusinessName,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create lounge owner district: %w", err)
	}

	return created, nil
}

// GetByDistrictID returns all records for a district UUID.
func (r *LoungeOwnerDistrictRepository) GetByDistrictID(districtID uuid.UUID) ([]models.LoungeOwnerDistrict, error) {
	query := `
		SELECT id, owner_id, district_id, owner_name, business_name
		FROM lounge_owner_districts
		WHERE district_id = $1
		ORDER BY owner_name ASC, business_name ASC
	`

	rows, err := r.db.Query(query, districtID)
	if err != nil {
		return nil, fmt.Errorf("failed to query lounge owner districts: %w", err)
	}
	defer rows.Close()

	records := make([]models.LoungeOwnerDistrict, 0)
	for rows.Next() {
		var record models.LoungeOwnerDistrict
		if err := rows.Scan(
			&record.ID,
			&record.OwnerID,
			&record.DistrictID,
			&record.OwnerName,
			&record.BusinessName,
		); err != nil {
			return nil, fmt.Errorf("failed to scan lounge owner district row: %w", err)
		}
		records = append(records, record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate lounge owner district rows: %w", err)
	}

	return records, nil
}

// ExistsByOwnerAndDistrict checks whether the owner_id + district_id pair already exists.
func (r *LoungeOwnerDistrictRepository) ExistsByOwnerAndDistrict(ownerID uuid.UUID, districtID uuid.UUID) (bool, error) {
	query := `
		SELECT EXISTS (
			SELECT 1
			FROM lounge_owner_districts
			WHERE owner_id = $1 AND district_id = $2
		)
	`

	var exists bool
	err := r.db.QueryRow(query, ownerID, districtID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check lounge owner district pair existence: %w", err)
	}

	return exists, nil
}
