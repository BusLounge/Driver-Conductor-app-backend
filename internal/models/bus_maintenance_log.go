package models

import (
	"errors"
	"strings"
	"time"
)

// BusMaintenanceLog represents a record in the bus_maintenance_logs table
type BusMaintenanceLog struct {
	ID                 string     `json:"id" db:"id"`
	BusID              string     `json:"bus_id" db:"bus_id"`
	StaffID            string     `json:"staff_id" db:"staff_id"`
	BusOwnerID         string     `json:"bus_owner_id" db:"bus_owner_id"`
	Date               string     `json:"date" db:"date"`
	MaintenanceType    string     `json:"maintenance_type" db:"maintenance_type"`
	Description        string     `json:"description" db:"description"`
	TotalCost          float64    `json:"total_cost" db:"total_cost"`
	VendorName         *string    `json:"vendor_name,omitempty" db:"vendor_name"`
	OdometerReading    *int       `json:"odometer_reading,omitempty" db:"odometer_reading"`
	NextServiceDueKm   *int       `json:"next_service_due_km,omitempty" db:"next_service_due_km"`
	NextServiceDueDate *string    `json:"next_service_due_date,omitempty" db:"next_service_due_date"`
	Notes              *string    `json:"notes,omitempty" db:"notes"`
	CreatedAt          time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at" db:"updated_at"`

	// Enriched display fields (from joins)
	BusNumber *string `json:"bus_number,omitempty" db:"bus_number"`
	StaffName *string `json:"staff_name,omitempty" db:"staff_name"`
}

// CreateBusMaintenanceLogRequest defines payload for logging maintenance
type CreateBusMaintenanceLogRequest struct {
	BusID              string  `json:"bus_id" binding:"required"`
	Date               string  `json:"date" binding:"required"` // Format: YYYY-MM-DD
	MaintenanceType    string  `json:"maintenance_type" binding:"required"`
	Description        string  `json:"description" binding:"required"`
	TotalCost          float64 `json:"total_cost" binding:"required,gte=0"`
	VendorName         *string `json:"vendor_name,omitempty"`
	OdometerReading    *int    `json:"odometer_reading,omitempty"`
	NextServiceDueKm   *int    `json:"next_service_due_km,omitempty"`
	NextServiceDueDate *string `json:"next_service_due_date,omitempty"` // Format: YYYY-MM-DD
	Notes              *string `json:"notes,omitempty"`
}

// Validate validates CreateBusMaintenanceLogRequest
func (r *CreateBusMaintenanceLogRequest) Validate() error {
	if strings.TrimSpace(r.BusID) == "" {
		return errors.New("bus_id is required")
	}
	if strings.TrimSpace(r.Date) == "" {
		return errors.New("date is required")
	}
	if strings.TrimSpace(r.Description) == "" {
		return errors.New("description is required")
	}
	if r.TotalCost < 0 {
		return errors.New("total_cost must be non-negative")
	}
	mType := strings.ToLower(strings.TrimSpace(r.MaintenanceType))
	validTypes := map[string]bool{
		"oil_change":       true,
		"tire_replacement": true,
		"brake_service":    true,
		"engine_service":   true,
		"battery":          true,
		"body_repair":      true,
		"general":          true,
		"other":            true,
	}
	if !validTypes[mType] {
		return errors.New("invalid maintenance_type: must be oil_change, tire_replacement, brake_service, engine_service, battery, body_repair, general, or other")
	}
	return nil
}

