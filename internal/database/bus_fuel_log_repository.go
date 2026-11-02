package database

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/smarttransit/sms-auth-backend/internal/models"
)

// BusFuelLogRepository handles database operations for bus_fuel_logs
type BusFuelLogRepository struct {
	db DB
}

// NewBusFuelLogRepository creates a new BusFuelLogRepository
func NewBusFuelLogRepository(db DB) *BusFuelLogRepository {
	return &BusFuelLogRepository{db: db}
}

// Create inserts a new fuel log
func (r *BusFuelLogRepository) Create(log *models.BusFuelLog) error {
	if log.ID == "" {
		log.ID = uuid.New().String()
	}

	if log.CostPerLiter == nil && log.Liters > 0 {
		cpl := log.TotalCost / log.Liters
		log.CostPerLiter = &cpl
	}

	query := `
		INSERT INTO bus_fuel_logs (
			id, bus_id, staff_id, bus_owner_id,
			date, fuel_type, liters, cost_per_liter,
			total_cost, odometer_reading, station_name, notes
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7, $8,
			$9, $10, $11, $12
		)
		RETURNING created_at, updated_at
	`

	return r.db.QueryRow(
		query,
		log.ID, log.BusID, log.StaffID, log.BusOwnerID,
		log.Date, log.FuelType, log.Liters, log.CostPerLiter,
		log.TotalCost, log.OdometerReading, log.StationName, log.Notes,
	).Scan(&log.CreatedAt, &log.UpdatedAt)
}

// GetByID retrieves a fuel log by ID with joined bus and staff information
func (r *BusFuelLogRepository) GetByID(id string) (*models.BusFuelLog, error) {
	query := `
		SELECT
			fl.id, fl.bus_id, fl.staff_id, fl.bus_owner_id,
			fl.date::text, fl.fuel_type, fl.liters, fl.cost_per_liter,
			fl.total_cost, fl.odometer_reading, fl.station_name, fl.notes,
			fl.created_at, fl.updated_at,
			b.bus_number,
			TRIM(COALESCE(bs.first_name, '') || ' ' || COALESCE(bs.last_name, '')) AS staff_name
		FROM bus_fuel_logs fl
		LEFT JOIN buses b ON b.id = fl.bus_id
		LEFT JOIN bus_staff bs ON bs.id = fl.staff_id
		WHERE fl.id = $1
	`

	log := &models.BusFuelLog{}
	var costPerLiter sql.NullFloat64
	var odometer sql.NullInt64
	var stationName sql.NullString
	var notes sql.NullString
	var busNumber sql.NullString
	var staffName sql.NullString

	err := r.db.QueryRow(query, id).Scan(
		&log.ID, &log.BusID, &log.StaffID, &log.BusOwnerID,
		&log.Date, &log.FuelType, &log.Liters, &costPerLiter,
		&log.TotalCost, &odometer, &stationName, &notes,
		&log.CreatedAt, &log.UpdatedAt,
		&busNumber, &staffName,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("fuel log not found")
		}
		return nil, err
	}

	if costPerLiter.Valid {
		log.CostPerLiter = &costPerLiter.Float64
	}
	if odometer.Valid {
		odom := int(odometer.Int64)
		log.OdometerReading = &odom
	}
	if stationName.Valid {
		log.StationName = &stationName.String
	}
	if notes.Valid {
		log.Notes = &notes.String
	}
	if busNumber.Valid {
		log.BusNumber = &busNumber.String
	}
	if staffName.Valid && staffName.String != "" {
		log.StaffName = &staffName.String
	}

	return log, nil
}

// GetByStaffID retrieves fuel logs for a given staff member with optional filters
func (r *BusFuelLogRepository) GetByStaffID(staffID string, busID string, fromDate string, toDate string) ([]models.BusFuelLog, error) {
	var conditions []string
	var args []interface{}
	argIdx := 1

	conditions = append(conditions, fmt.Sprintf("fl.staff_id = $%d", argIdx))
	args = append(args, staffID)
	argIdx++

	if strings.TrimSpace(busID) != "" {
		conditions = append(conditions, fmt.Sprintf("fl.bus_id = $%d", argIdx))
		args = append(args, busID)
		argIdx++
	}

	if strings.TrimSpace(fromDate) != "" {
		conditions = append(conditions, fmt.Sprintf("fl.date >= $%d", argIdx))
		args = append(args, fromDate)
		argIdx++
	}

	if strings.TrimSpace(toDate) != "" {
		conditions = append(conditions, fmt.Sprintf("fl.date <= $%d", argIdx))
		args = append(args, toDate)
		argIdx++
	}

	query := fmt.Sprintf(`
		SELECT
			fl.id, fl.bus_id, fl.staff_id, fl.bus_owner_id,
			fl.date::text, fl.fuel_type, fl.liters, fl.cost_per_liter,
			fl.total_cost, fl.odometer_reading, fl.station_name, fl.notes,
			fl.created_at, fl.updated_at,
			b.bus_number,
			TRIM(COALESCE(bs.first_name, '') || ' ' || COALESCE(bs.last_name, '')) AS staff_name
		FROM bus_fuel_logs fl
		LEFT JOIN buses b ON b.id = fl.bus_id
		LEFT JOIN bus_staff bs ON bs.id = fl.staff_id
		WHERE %s
		ORDER BY fl.date DESC, fl.created_at DESC
	`, strings.Join(conditions, " AND "))

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []models.BusFuelLog
	for rows.Next() {
		var l models.BusFuelLog
		var costPerLiter sql.NullFloat64
		var odometer sql.NullInt64
		var stationName sql.NullString
		var notes sql.NullString
		var busNumber sql.NullString
		var staffName sql.NullString

		if err := rows.Scan(
			&l.ID, &l.BusID, &l.StaffID, &l.BusOwnerID,
			&l.Date, &l.FuelType, &l.Liters, &costPerLiter,
			&l.TotalCost, &odometer, &stationName, &notes,
			&l.CreatedAt, &l.UpdatedAt,
			&busNumber, &staffName,
		); err != nil {
			return nil, err
		}

		if costPerLiter.Valid {
			l.CostPerLiter = &costPerLiter.Float64
		}
		if odometer.Valid {
			odom := int(odometer.Int64)
			l.OdometerReading = &odom
		}
		if stationName.Valid {
			l.StationName = &stationName.String
		}
		if notes.Valid {
			l.Notes = &notes.String
		}
		if busNumber.Valid {
			l.BusNumber = &busNumber.String
		}
		if staffName.Valid && staffName.String != "" {
			l.StaffName = &staffName.String
		}

		logs = append(logs, l)
	}

	return logs, nil
}

// Delete removes a fuel log by ID if it belongs to the requesting staff
func (r *BusFuelLogRepository) Delete(id string, staffID string) error {
	query := `DELETE FROM bus_fuel_logs WHERE id = $1 AND staff_id = $2`
	result, err := r.db.Exec(query, id, staffID)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("fuel log not found or unauthorized")
	}
	return nil
}

