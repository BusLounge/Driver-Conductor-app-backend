package database

import (
	"database/sql"
	"fmt"
)

// District represents a district lookup record.
type District struct {
	ID       string `json:"id"`
	District string `json:"district"`
}

// DistrictRepository handles database operations for districts table.
type DistrictRepository struct {
	db DB
}

// NewDistrictRepository creates a new district repository.
func NewDistrictRepository(db DB) *DistrictRepository {
	return &DistrictRepository{db: db}
}

// GetAll retrieves all districts (active-only if is_active column exists).
func (r *DistrictRepository) GetAll() ([]District, error) {
	query := `
		SELECT id::text AS id, district
		FROM districts
		ORDER BY district
	`
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query districts: %w", err)
	}
	defer rows.Close()

	districts := make([]District, 0)
	for rows.Next() {
		var district District
		if err := rows.Scan(&district.ID, &district.District); err != nil {
			return nil, fmt.Errorf("failed to scan district row: %w", err)
		}

		districts = append(districts, district)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate district rows: %w", err)
	}

	return districts, nil
}

// GetByID retrieves one district by ID (requires id or district_id column).
func (r *DistrictRepository) GetByID(id string) (*District, error) {
	query := `
		SELECT id::text AS id, district
		FROM districts
		WHERE id::text = $1
		LIMIT 1
	`
	var district District
	err := r.db.QueryRow(query, id).Scan(&district.ID, &district.District)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query district by id: %w", err)
	}

	return &district, nil
}
