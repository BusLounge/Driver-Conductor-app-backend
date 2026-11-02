package database

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/smarttransit/sms-auth-backend/internal/models"
)

// BusMaintenanceLogRepository handles database operations for bus_maintenance_logs
type BusMaintenanceLogRepository struct {
	db DB
}

// NewBusMaintenanceLogRepository creates a new BusMaintenanceLogRepository
func NewBusMaintenanceLogRepository(db DB) *BusMaintenanceLogRepository {
	return &BusMaintenanceLogRepository{db: db}
}

// Create inserts a new maintenance log
func (r *BusMaintenanceLogRepository) Create(log *models.BusMaintenanceLog) error {
	if log.ID == "" {
		log.ID = uuid.New().String()
	}

	query := `
		INSERT INTO bus_maintenance_logs (
			id, bus_id, staff_id, bus_owner_id,
			date, maintenance_type, description, total_cost,
			vendor_name, odometer_reading, next_service_due_km, next_service_due_date, notes
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7, $8,
			$9, $10, $11, $12, $13
		)
		RETURNING created_at, updated_at
	`

	return r.db.QueryRow(
		query,
		log.ID, log.BusID, log.StaffID, log.BusOwnerID,
		log.Date, log.MaintenanceType, log.Description, log.TotalCost,
		log.VendorName, log.OdometerReading, log.NextServiceDueKm, log.NextServiceDueDate, log.Notes,
	).Scan(&log.CreatedAt, &log.UpdatedAt)
}

// GetByID retrieves a maintenance log by ID with joined bus and staff information
func (r *BusMaintenanceLogRepository) GetByID(id string) (*models.BusMaintenanceLog, error) {
	query := `
		SELECT
			ml.id, ml.bus_id, ml.staff_id, ml.bus_owner_id,
			ml.date::text, ml.maintenance_type, ml.description, ml.total_cost,
			ml.vendor_name, ml.odometer_reading, ml.next_service_due_km, ml.next_service_due_date::text, ml.notes,
			ml.created_at, ml.updated_at,
			b.bus_number,
			TRIM(COALESCE(bs.first_name, '') || ' ' || COALESCE(bs.last_name, '')) AS staff_name
		FROM bus_maintenance_logs ml
		LEFT JOIN buses b ON b.id = ml.bus_id
		LEFT JOIN bus_staff bs ON bs.id = ml.staff_id
		WHERE ml.id = $1
	`

	log := &models.BusMaintenanceLog{}
	var vendorName sql.NullString
	var odometer sql.NullInt64
	var nextDueKm sql.NullInt64
	var nextDueDate sql.NullString
	var notes sql.NullString
	var busNumber sql.NullString
	var staffName sql.NullString

	err := r.db.QueryRow(query, id).Scan(
		&log.ID, &log.BusID, &log.StaffID, &log.BusOwnerID,
		&log.Date, &log.MaintenanceType, &log.Description, &log.TotalCost,
		&vendorName, &odometer, &nextDueKm, &nextDueDate, &notes,
		&log.CreatedAt, &log.UpdatedAt,
		&busNumber, &staffName,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("maintenance log not found")
		}
		return nil, err
	}

	if vendorName.Valid {
		log.VendorName = &vendorName.String
	}
	if odometer.Valid {
		odom := int(odometer.Int64)
		log.OdometerReading = &odom
	}
	if nextDueKm.Valid {
		km := int(nextDueKm.Int64)
		log.NextServiceDueKm = &km
	}
	if nextDueDate.Valid {
		log.NextServiceDueDate = &nextDueDate.String
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

// GetByStaffID retrieves maintenance logs for a given staff member with optional filters
func (r *BusMaintenanceLogRepository) GetByStaffID(staffID string, busID string, fromDate string, toDate string) ([]models.BusMaintenanceLog, error) {
	var conditions []string
	var args []interface{}
	argIdx := 1

	conditions = append(conditions, fmt.Sprintf("ml.staff_id = $%d", argIdx))
	args = append(args, staffID)
	argIdx++

	if strings.TrimSpace(busID) != "" {
		conditions = append(conditions, fmt.Sprintf("ml.bus_id = $%d", argIdx))
		args = append(args, busID)
		argIdx++
	}

	if strings.TrimSpace(fromDate) != "" {
		conditions = append(conditions, fmt.Sprintf("ml.date >= $%d", argIdx))
		args = append(args, fromDate)
		argIdx++
	}

	if strings.TrimSpace(toDate) != "" {
		conditions = append(conditions, fmt.Sprintf("ml.date <= $%d", argIdx))
		args = append(args, toDate)
		argIdx++
	}

	query := fmt.Sprintf(`
		SELECT
			ml.id, ml.bus_id, ml.staff_id, ml.bus_owner_id,
			ml.date::text, ml.maintenance_type, ml.description, ml.total_cost,
			ml.vendor_name, ml.odometer_reading, ml.next_service_due_km, ml.next_service_due_date::text, ml.notes,
			ml.created_at, ml.updated_at,
			b.bus_number,
			TRIM(COALESCE(bs.first_name, '') || ' ' || COALESCE(bs.last_name, '')) AS staff_name
		FROM bus_maintenance_logs ml
		LEFT JOIN buses b ON b.id = ml.bus_id
		LEFT JOIN bus_staff bs ON bs.id = ml.staff_id
		WHERE %s
		ORDER BY ml.date DESC, ml.created_at DESC
	`, strings.Join(conditions, " AND "))

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []models.BusMaintenanceLog
	for rows.Next() {
		var l models.BusMaintenanceLog
		var vendorName sql.NullString
		var odometer sql.NullInt64
		var nextDueKm sql.NullInt64
		var nextDueDate sql.NullString
		var notes sql.NullString
		var busNumber sql.NullString
		var staffName sql.NullString

		if err := rows.Scan(
			&l.ID, &l.BusID, &l.StaffID, &l.BusOwnerID,
			&l.Date, &l.MaintenanceType, &l.Description, &l.TotalCost,
			&vendorName, &odometer, &nextDueKm, &nextDueDate, &notes,
			&l.CreatedAt, &l.UpdatedAt,
			&busNumber, &staffName,
		); err != nil {
			return nil, err
		}

		if vendorName.Valid {
			l.VendorName = &vendorName.String
		}
		if odometer.Valid {
			odom := int(odometer.Int64)
			l.OdometerReading = &odom
		}
		if nextDueKm.Valid {
			km := int(nextDueKm.Int64)
			l.NextServiceDueKm = &km
		}
		if nextDueDate.Valid {
			l.NextServiceDueDate = &nextDueDate.String
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

// Delete removes a maintenance log by ID if it belongs to the requesting staff
func (r *BusMaintenanceLogRepository) Delete(id string, staffID string) error {
	query := `DELETE FROM bus_maintenance_logs WHERE id = $1 AND staff_id = $2`
	result, err := r.db.Exec(query, id, staffID)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("maintenance log not found or unauthorized")
	}
	return nil
}

// GetVehicleSummary returns aggregated statistics across fuel and maintenance for staff or bus
func (r *BusMaintenanceLogRepository) GetVehicleSummary(staffID string, busID string) (*models.VehicleSummaryResponse, error) {
	summary := &models.VehicleSummaryResponse{}

	// 1. Fuel stats
	var fuelQuery string
	var fuelArgs []interface{}
	if strings.TrimSpace(busID) != "" {
		fuelQuery = `
			SELECT
				COALESCE(SUM(liters), 0),
				COALESCE(SUM(total_cost), 0),
				COUNT(*),
				MAX(odometer_reading)
			FROM bus_fuel_logs
			WHERE staff_id = $1 AND bus_id = $2
		`
		fuelArgs = []interface{}{staffID, busID}
	} else {
		fuelQuery = `
			SELECT
				COALESCE(SUM(liters), 0),
				COALESCE(SUM(total_cost), 0),
				COUNT(*),
				MAX(odometer_reading)
			FROM bus_fuel_logs
			WHERE staff_id = $1
		`
		fuelArgs = []interface{}{staffID}
	}

	var fuelOdom sql.NullInt64
	err := r.db.QueryRow(fuelQuery, fuelArgs...).Scan(
		&summary.TotalFuelLiters,
		&summary.TotalFuelCost,
		&summary.FuelEntriesCount,
		&fuelOdom,
	)
	if err != nil {
		return nil, err
	}

	if summary.TotalFuelLiters > 0 {
		summary.AvgCostPerLiter = summary.TotalFuelCost / summary.TotalFuelLiters
	}

	// 2. Maintenance stats
	var maintQuery string
	var maintArgs []interface{}
	if strings.TrimSpace(busID) != "" {
		maintQuery = `
			SELECT
				COALESCE(SUM(total_cost), 0),
				COUNT(*),
				MAX(odometer_reading)
			FROM bus_maintenance_logs
			WHERE staff_id = $1 AND bus_id = $2
		`
		maintArgs = []interface{}{staffID, busID}
	} else {
		maintQuery = `
			SELECT
				COALESCE(SUM(total_cost), 0),
				COUNT(*),
				MAX(odometer_reading)
			FROM bus_maintenance_logs
			WHERE staff_id = $1
		`
		maintArgs = []interface{}{staffID}
	}

	var maintOdom sql.NullInt64
	err = r.db.QueryRow(maintQuery, maintArgs...).Scan(
		&summary.TotalMaintenanceCost,
		&summary.MaintenanceEntriesCount,
		&maintOdom,
	)
	if err != nil {
		return nil, err
	}

	// Determine latest odometer
	if fuelOdom.Valid && maintOdom.Valid {
		fVal := int(fuelOdom.Int64)
		mVal := int(maintOdom.Int64)
		if fVal > mVal {
			summary.LastOdometer = &fVal
		} else {
			summary.LastOdometer = &mVal
		}
	} else if fuelOdom.Valid {
		val := int(fuelOdom.Int64)
		summary.LastOdometer = &val
	} else if maintOdom.Valid {
		val := int(maintOdom.Int64)
		summary.LastOdometer = &val
	}

	return summary, nil
}

