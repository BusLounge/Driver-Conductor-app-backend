package database

import (
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/smarttransit/sms-auth-backend/internal/models"
)

// LoungeRepository handles database operations for lounges
type LoungeRepository struct {
	db *sqlx.DB
}

// NewLoungeRepository creates a new lounge repository
func NewLoungeRepository(db *sqlx.DB) *LoungeRepository {
	return &LoungeRepository{db: db}
}

// CreateLounge creates a new lounge (Step 3 of registration)
func (r *LoungeRepository) CreateLounge(
	loungeOwnerID uuid.UUID,
	loungeName string,
	description *string,
	address string,
	district *uuid.UUID,
	state *string,
	postalCode *string,
	contactPhone string,
	latitude *string,
	longitude *string,
	capacity *int,
	price1Hour *string,
	price2Hours *string,
	price3Hours *string,
	priceUntilBus *string,
	amenities string,
	images string,
) (*models.Lounge, error) {
	
	lounge := &models.Lounge{
		ID:            uuid.New(),
		LoungeOwnerID: loungeOwnerID,
		Status:        models.LoungeStatusPending,
		IsOperational: true,
	}

	query := `
		INSERT INTO lounges (
			id, lounge_owner_id, lounge_name, description, address,
			district, state, postal_code, contact_phone, latitude, longitude, capacity,
			price_1_hour, price_2_hours, price_3_hours, price_until_bus,
			amenities, images,
			status, is_operational,
			created_at, updated_at
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
			$16, $17, $18, $19, $20, NOW(), NOW()
		)
		RETURNING id, created_at, updated_at
	`

	err := r.db.QueryRowx(
		query,
		lounge.ID,
		loungeOwnerID,
		loungeName,
		description,
		address,
		district,
		state,
		postalCode,
		contactPhone,
		latitude,
		longitude,
		capacity,
		price1Hour,
		price2Hours,
		price3Hours,
		priceUntilBus,
		amenities,
		images,
		lounge.Status,
		lounge.IsOperational,
	).Scan(&lounge.ID, &lounge.CreatedAt, &lounge.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create lounge: %w", err)
	}

	return lounge, nil
}

// GetLoungeByID retrieves a lounge by ID
func (r *LoungeRepository) GetLoungeByID(id uuid.UUID) (*models.Lounge, error) {
	var lounge models.Lounge
	query := `
		SELECT id, lounge_owner_id, lounge_name, description, address, district, state, country, 
		       postal_code, latitude, longitude, contact_phone, capacity, 
		       price_1_hour, price_2_hours, price_3_hours, price_until_bus, 
		       amenities, images, status, is_operational, average_rating, 
		       created_at, updated_at
		FROM lounges WHERE id = $1
	`
	err := r.db.Get(&lounge, query, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get lounge: %w", err)
	}
	return &lounge, nil
}

// GetLoungesByOwnerID retrieves all lounges for a specific owner
func (r *LoungeRepository) GetLoungesByOwnerID(ownerID uuid.UUID) ([]models.Lounge, error) {
	var lounges []models.Lounge
	query := `
		SELECT id, lounge_owner_id, lounge_name, description, address, district, state, country, 
		       postal_code, latitude, longitude, contact_phone, capacity, 
		       price_1_hour, price_2_hours, price_3_hours, price_until_bus, 
		       amenities, images, status, is_operational, average_rating, 
		       created_at, updated_at
		FROM lounges 
		WHERE lounge_owner_id = $1 
		ORDER BY created_at DESC
	`
	err := r.db.Select(&lounges, query, ownerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get lounges: %w", err)
	}
	return lounges, nil
}

// GetLoungesByOwnerAndDistrict retrieves lounges matching both owner ID and district UUID
func (r *LoungeRepository) GetLoungesByOwnerAndDistrict(ownerID uuid.UUID, districtID uuid.UUID) ([]models.Lounge, error) {
	var lounges []models.Lounge
	query := `
		SELECT id, lounge_owner_id, lounge_name, description, address, district, state, country,
		       postal_code, latitude, longitude, contact_phone, capacity,
		       price_1_hour, price_2_hours, price_3_hours, price_until_bus,
		       amenities, images, status, is_operational, average_rating,
		       created_at, updated_at
		FROM lounges
		WHERE lounge_owner_id = $1
		  AND district = $2
		ORDER BY created_at DESC
	`
	err := r.db.Select(&lounges, query, ownerID, districtID)
	if err != nil {
		return nil, fmt.Errorf("failed to get lounges by owner and district: %w", err)
	}
	return lounges, nil
}

// GetAllActiveLounges retrieves all active lounges (for public listing) - DEPRECATED: use GetActiveLoungesPaginated
func (r *LoungeRepository) GetAllActiveLounges() ([]models.Lounge, error) {
	var lounges []models.Lounge
	query := `
		SELECT id, lounge_owner_id, lounge_name, description, address, district, state, country, 
		       postal_code, latitude, longitude, contact_phone, capacity, 
		       price_1_hour, price_2_hours, price_3_hours, price_until_bus, 
		       amenities, images, status, is_operational, average_rating, 
		       created_at, updated_at
		FROM lounges 
		WHERE status = 'approved' AND is_operational = true
		ORDER BY lounge_name
		LIMIT 50
	`
	err := r.db.Select(&lounges, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get active lounges: %w", err)
	}
	return lounges, nil
}

// GetActiveLoungesPaginated retrieves active lounges with pagination
func (r *LoungeRepository) GetActiveLoungesPaginated(limit int, offset int) ([]models.Lounge, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20 // Default limit
	}
	if offset < 0 {
		offset = 0
	}

	var lounges []models.Lounge
	query := `
		SELECT id, lounge_owner_id, lounge_name, description, address, district, state, country, 
		       postal_code, latitude, longitude, contact_phone, capacity, 
		       price_1_hour, price_2_hours, price_3_hours, price_until_bus, 
		       amenities, images, status, is_operational, average_rating, 
		       created_at, updated_at
		FROM lounges 
		WHERE status = 'approved' AND is_operational = true
		ORDER BY lounge_name
		LIMIT $1 OFFSET $2
	`
	err := r.db.Select(&lounges, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get active lounges: %w", err)
	}

	// Get total count
	var total int64
	countQuery := `SELECT COUNT(*) FROM lounges WHERE status = 'approved' AND is_operational = true`
	err = r.db.Get(&total, countQuery)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count lounges: %w", err)
	}

	return lounges, total, nil
}

// GetAllLounges retrieves all lounges regardless of status or operational flag
func (r *LoungeRepository) GetAllLounges() ([]models.Lounge, error) {
	var lounges []models.Lounge
	query := `
		SELECT id, lounge_owner_id, lounge_name, description, address, district, state, country, 
			   postal_code, latitude, longitude, contact_phone, capacity, 
			   price_1_hour, price_2_hours, price_3_hours, price_until_bus, 
			   amenities, images, status, is_operational, average_rating, 
			   created_at, updated_at
		FROM lounges 
		ORDER BY lounge_name
	`

	err := r.db.Select(&lounges, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get lounges: %w", err)
	}
	return lounges, nil
}

// GetLoungesByStatus retrieves all lounges with a specific status
func (r *LoungeRepository) GetLoungesByStatus(status string) ([]models.Lounge, error) {
	var lounges []models.Lounge
	query := `
		SELECT id, lounge_owner_id, lounge_name, description, address, district, state, country, 
		       postal_code, latitude, longitude, contact_phone, capacity, 
		       price_1_hour, price_2_hours, price_3_hours, price_until_bus, 
		       amenities, images, status, is_operational, average_rating, 
		       created_at, updated_at
		FROM lounges WHERE status = $1 ORDER BY created_at DESC
	`
	err := r.db.Select(&lounges, query, status)
	if err != nil {
		return nil, fmt.Errorf("failed to get lounges by status: %w", err)
	}
	return lounges, nil
}

// SearchActiveLounges retrieves active lounges with optional state filter and limit
func (r *LoungeRepository) SearchActiveLounges(state string, limit int) ([]models.Lounge, error) {
	var lounges []models.Lounge
	var args []interface{}
	argNum := 1

	query := `
		SELECT id, lounge_owner_id, lounge_name, description, address, district, state, country, 
		       postal_code, latitude, longitude, contact_phone, capacity, 
		       price_1_hour, price_2_hours, price_3_hours, price_until_bus, 
		       amenities, images, status, is_operational, average_rating, 
		       created_at, updated_at
		FROM lounges WHERE status = 'approved' AND is_operational = true
	`

	if state != "" {
		query += fmt.Sprintf(" AND LOWER(state) = LOWER($%d)", argNum)
		args = append(args, state)
		argNum++
	}

	// Add random ordering when limit is specified, otherwise order by name
	if limit > 0 {
		query += " ORDER BY RANDOM()"
		query += fmt.Sprintf(" LIMIT $%d", argNum)
		args = append(args, limit)
	} else {
		query += " ORDER BY lounge_name"
	}

	err := r.db.Select(&lounges, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to search active lounges: %w", err)
	}
	return lounges, nil
}

// SearchLounges retrieves lounges with optional state filter and limit without status filtering
func (r *LoungeRepository) SearchLounges(state string, limit int) ([]models.Lounge, error) {
	var lounges []models.Lounge
	var args []interface{}
	argNum := 1

	query := `
		SELECT id, lounge_owner_id, lounge_name, description, address, district, state, country, 
			   postal_code, latitude, longitude, contact_phone, capacity, 
			   price_1_hour, price_2_hours, price_3_hours, price_until_bus, 
			   amenities, images, status, is_operational, average_rating, 
			   created_at, updated_at
		FROM lounges
	`

	if state != "" {
		query += fmt.Sprintf(" WHERE LOWER(state) = LOWER($%d)", argNum)
		args = append(args, state)
		argNum++
	}

	if limit > 0 {
		query += " ORDER BY RANDOM()"
		query += fmt.Sprintf(" LIMIT $%d", argNum)
		args = append(args, limit)
	} else {
		query += " ORDER BY lounge_name"
	}

	err := r.db.Select(&lounges, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to search lounges: %w", err)
	}
	return lounges, nil
}

// GetLoungesByStopID retrieves all active lounges that serve a specific stop
// A lounge serves a stop if the stop is either stop_before_id or stop_after_id in lounge_routes
func (r *LoungeRepository) GetLoungesByStopID(stopID uuid.UUID) ([]models.Lounge, error) {
	var lounges []models.Lounge
	query := `
		SELECT DISTINCT l.id, l.lounge_owner_id, l.lounge_name, l.description, l.address, l.district, l.state, l.country, 
		       l.postal_code, l.latitude, l.longitude, l.contact_phone, l.capacity, 
		       l.price_1_hour, l.price_2_hours, l.price_3_hours, l.price_until_bus, 
		       l.amenities, l.images, l.status, l.is_operational, l.average_rating, 
		       l.created_at, l.updated_at
		FROM lounges l
		INNER JOIN lounge_routes lr ON l.id = lr.lounge_id
		WHERE l.status = 'approved' 
		  AND l.is_operational = true
		  AND (lr.stop_before_id = $1 OR lr.stop_after_id = $1)
		ORDER BY l.lounge_name
		LIMIT 50
	`
	err := r.db.Select(&lounges, query, stopID)
	if err != nil {
		return nil, fmt.Errorf("failed to get lounges by stop: %w", err)
	}
	return lounges, nil
}

// GetLoungesByStopIDPaginated retrieves active lounges that serve a specific stop with pagination
func (r *LoungeRepository) GetLoungesByStopIDPaginated(stopID uuid.UUID, limit int, offset int) ([]models.Lounge, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	var lounges []models.Lounge
	query := `
		SELECT DISTINCT l.id, l.lounge_owner_id, l.lounge_name, l.description, l.address, l.district, l.state, l.country, 
		       l.postal_code, l.latitude, l.longitude, l.contact_phone, l.capacity, 
		       l.price_1_hour, l.price_2_hours, l.price_3_hours, l.price_until_bus, 
		       l.amenities, l.images, l.status, l.is_operational, l.average_rating, 
		       l.created_at, l.updated_at
		FROM lounges l
		INNER JOIN lounge_routes lr ON l.id = lr.lounge_id
		WHERE l.status = 'approved' 
		  AND l.is_operational = true
		  AND (lr.stop_before_id = $1 OR lr.stop_after_id = $1)
		ORDER BY l.lounge_name
		LIMIT $2 OFFSET $3
	`
	err := r.db.Select(&lounges, query, stopID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get lounges by stop: %w", err)
	}

	// Get total count
	var total int64
	countQuery := `
		SELECT COUNT(DISTINCT l.id) FROM lounges l
		INNER JOIN lounge_routes lr ON l.id = lr.lounge_id
		WHERE l.status = 'approved' 
		  AND l.is_operational = true
		  AND (lr.stop_before_id = $1 OR lr.stop_after_id = $1)
	`
	err = r.db.Get(&total, countQuery, stopID)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count lounges by stop: %w", err)
	}

	return lounges, total, nil
}

// GetLoungesByRouteID retrieves all active lounges that serve a specific route
func (r *LoungeRepository) GetLoungesByRouteID(routeID uuid.UUID) ([]models.Lounge, error) {
	var lounges []models.Lounge
	query := `
		SELECT DISTINCT l.id, l.lounge_owner_id, l.lounge_name, l.description, l.address, l.district, l.state, l.country, 
		       l.postal_code, l.latitude, l.longitude, l.contact_phone, l.capacity, 
		       l.price_1_hour, l.price_2_hours, l.price_3_hours, l.price_until_bus, 
		       l.amenities, l.images, l.status, l.is_operational, l.average_rating, 
		       l.created_at, l.updated_at
		FROM lounges l
		INNER JOIN lounge_routes lr ON l.id = lr.lounge_id
		WHERE l.status = 'approved' 
		  AND l.is_operational = true
		  AND lr.master_route_id = $1
		ORDER BY l.lounge_name
		LIMIT 50
	`
	err := r.db.Select(&lounges, query, routeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get lounges by route: %w", err)
	}
	return lounges, nil
}

// GetLoungesByRouteIDPaginated retrieves active lounges that serve a specific route with pagination
func (r *LoungeRepository) GetLoungesByRouteIDPaginated(routeID uuid.UUID, limit int, offset int) ([]models.Lounge, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	var lounges []models.Lounge
	query := `
		SELECT DISTINCT l.id, l.lounge_owner_id, l.lounge_name, l.description, l.address, l.district, l.state, l.country, 
		       l.postal_code, l.latitude, l.longitude, l.contact_phone, l.capacity, 
		       l.price_1_hour, l.price_2_hours, l.price_3_hours, l.price_until_bus, 
		       l.amenities, l.images, l.status, l.is_operational, l.average_rating, 
		       l.created_at, l.updated_at
		FROM lounges l
		INNER JOIN lounge_routes lr ON l.id = lr.lounge_id
		WHERE l.status = 'approved' 
		  AND l.is_operational = true
		  AND lr.master_route_id = $1
		ORDER BY l.lounge_name
		LIMIT $2 OFFSET $3
	`
	err := r.db.Select(&lounges, query, routeID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get lounges by route: %w", err)
	}

	// Get total count
	var total int64
	countQuery := `
		SELECT COUNT(DISTINCT l.id) FROM lounges l
		INNER JOIN lounge_routes lr ON l.id = lr.lounge_id
		WHERE l.status = 'approved' 
		  AND l.is_operational = true
		  AND lr.master_route_id = $1
	`
	err = r.db.Get(&total, countQuery, routeID)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count lounges by route: %w", err)
	}

	return lounges, total, nil
}

// GetLoungesNearStop retrieves lounges where passenger's stop is within N stops of the lounge's location
// The lounge is located between stop_before_id and stop_after_id on the route
// We check if the passenger's stop_order is within 'maxStopDistance' of either lounge stop
func (r *LoungeRepository) GetLoungesNearStop(masterRouteID uuid.UUID, passengerStopID uuid.UUID, maxStopDistance int) ([]models.Lounge, error) {
	var lounges []models.Lounge
	query := `
		WITH passenger_stop AS (
			-- Get the passenger's selected stop order
			SELECT stop_order 
			FROM master_route_stops 
			WHERE id = $2 AND master_route_id = $1
		),
		lounge_stops AS (
			-- Get lounges on this route with their stop orders
			SELECT 
				lr.lounge_id,
				mrs_before.stop_order as stop_before_order,
				mrs_after.stop_order as stop_after_order
			FROM lounge_routes lr
			LEFT JOIN master_route_stops mrs_before ON lr.stop_before_id = mrs_before.id
			LEFT JOIN master_route_stops mrs_after ON lr.stop_after_id = mrs_after.id
			WHERE lr.master_route_id = $1
		)
		SELECT DISTINCT l.id, l.lounge_owner_id, l.lounge_name, l.description, l.address, l.district, l.state, l.country, 
		       l.postal_code, l.latitude, l.longitude, l.contact_phone, l.capacity, 
		       l.price_1_hour, l.price_2_hours, l.price_3_hours, l.price_until_bus, 
		       l.amenities, l.images, l.status, l.is_operational, l.average_rating, 
		       l.created_at, l.updated_at
		FROM lounges l
		INNER JOIN lounge_stops ls ON l.id = ls.lounge_id
		CROSS JOIN passenger_stop ps
		WHERE l.status = 'approved' 
		  AND l.is_operational = true
		  AND (
		      -- Passenger stop is within N stops of stop_before
		      ABS(ps.stop_order - COALESCE(ls.stop_before_order, 0)) <= $3
		      OR
		      -- Passenger stop is within N stops of stop_after  
		      ABS(ps.stop_order - COALESCE(ls.stop_after_order, 0)) <= $3
		  )
		ORDER BY l.lounge_name
	`
	err := r.db.Select(&lounges, query, masterRouteID, passengerStopID, maxStopDistance)
	if err != nil {
		return nil, fmt.Errorf("failed to get lounges near stop: %w", err)
	}
	return lounges, nil
}

// GetDistinctStates retrieves all distinct states from active lounges
func (r *LoungeRepository) GetDistinctStates() ([]string, error) {
	var states []string
	query := `
		SELECT DISTINCT state FROM lounges 
		WHERE status = 'approved' AND is_operational = true AND state IS NOT NULL AND state != ''
		ORDER BY state
	`
	err := r.db.Select(&states, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get distinct states: %w", err)
	}
	return states, nil
}

// UpdateLounge updates lounge information
func (r *LoungeRepository) UpdateLounge(
	id uuid.UUID,
	loungeName string,
	description *string,
	address string,
	district *uuid.UUID,
	state *string,
	postalCode *string,
	contactPhone string,
	latitude *string,
	longitude *string,
	capacity *int,
	price1Hour *string,
	price2Hours *string,
	price3Hours *string,
	priceUntilBus *string,
	amenities string,
	images string,
) error {
	query := `
		UPDATE lounges 
		SET 
			lounge_name = $1,
			description = $2,
			address = $3,
			district = $4,
			state = $5,
			postal_code = $6,
			contact_phone = $7,
			latitude = $8,
			longitude = $9,
			capacity = $10,
			price_1_hour = $11,
			price_2_hours = $12,
			price_3_hours = $13,
			price_until_bus = $14,
			amenities = $15,
			images = $16,
			updated_at = NOW()
		WHERE id = $17
	`

	result, err := r.db.Exec(
		query,
		loungeName,
		description,
		address,
		district,
		state,
		postalCode,
		contactPhone,
		latitude,
		longitude,
		capacity,
		price1Hour,
		price2Hours,
		price3Hours,
		priceUntilBus,
		amenities,
		images,
		id,
	)

	if err != nil {
		return fmt.Errorf("failed to update lounge: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("lounge not found")
	}

	return nil
}

// UpdateLoungeStatus updates lounge status
func (r *LoungeRepository) UpdateLoungeStatus(id uuid.UUID, status string) error {
	query := `
		UPDATE lounges 
		SET 
			status = $1,
			updated_at = NOW()
		WHERE id = $2
	`

	_, err := r.db.Exec(query, status, id)
	if err != nil {
		return fmt.Errorf("failed to update lounge status: %w", err)
	}

	return nil
}

// DeleteLounge deletes a lounge
func (r *LoungeRepository) DeleteLounge(id uuid.UUID) error {
	query := `DELETE FROM lounges WHERE id = $1`
	_, err := r.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete lounge: %w", err)
	}
	return nil
}

// GetPendingLounges retrieves all lounges pending approval
func (r *LoungeRepository) GetPendingLounges(limit int, offset int) ([]models.Lounge, error) {
	var lounges []models.Lounge
	query := `
		SELECT * FROM lounges 
		WHERE status = 'pending'
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`
	err := r.db.Select(&lounges, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending lounges: %w", err)
	}
	return lounges, nil
}
