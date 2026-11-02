package models

import (
	"errors"
	"strings"
	"time"
)

// BusFuelLog represents a record in the bus_fuel_logs table
type BusFuelLog struct {
	ID              string     `json:"id" db:"id"`
	BusID           string     `json:"bus_id" db:"bus_id"`
	StaffID         string     `json:"staff_id" db:"staff_id"`
	BusOwnerID      string     `json:"bus_owner_id" db:"bus_owner_id"`
	Date            string     `json:"date" db:"date"`
	FuelType        string     `json:"fuel_type" db:"fuel_type"`
	Liters          float64    `json:"liters" db:"liters"`
	CostPerLiter    *float64   `json:"cost_per_liter,omitempty" db:"cost_per_liter"`
	TotalCost       float64    `json:"total_cost" db:"total_cost"`
	OdometerReading *int       `json:"odometer_reading,omitempty" db:"odometer_reading"`
	StationName     *string    `json:"station_name,omitempty" db:"station_name"`
	Notes           *string    `json:"notes,omitempty" db:"notes"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`

	// Enriched display fields (from joins)
	BusNumber *string `json:"bus_number,omitempty" db:"bus_number"`
	StaffName *string `json:"staff_name,omitempty" db:"staff_name"`
}

// CreateBusFuelLogRequest defines payload for logging fuel refill
type CreateBusFuelLogRequest struct {
	BusID           string   `json:"bus_id" binding:"required"`
	Date            string   `json:"date" binding:"required"` // Format: YYYY-MM-DD
	FuelType        string   `json:"fuel_type" binding:"required"`
	Liters          float64  `json:"liters" binding:"required,gt=0"`
	CostPerLiter    *float64 `json:"cost_per_liter,omitempty"`
	TotalCost       float64  `json:"total_cost" binding:"required,gte=0"`
	OdometerReading *int     `json:"odometer_reading,omitempty"`
	StationName     *string  `json:"station_name,omitempty"`
	Notes           *string  `json:"notes,omitempty"`
}

// Validate validates CreateBusFuelLogRequest
func (r *CreateBusFuelLogRequest) Validate() error {
	if strings.TrimSpace(r.BusID) == "" {
		return errors.New("bus_id is required")
	}
	if strings.TrimSpace(r.Date) == "" {
		return errors.New("date is required")
	}
	if r.Liters <= 0 {
		return errors.New("liters must be greater than zero")
	}
	if r.TotalCost < 0 {
		return errors.New("total_cost must be non-negative")
	}
	fuelType := strings.ToLower(strings.TrimSpace(r.FuelType))
	if fuelType != "diesel" && fuelType != "petrol" && fuelType != "cng" && fuelType != "electric" {
		return errors.New("fuel_type must be diesel, petrol, cng, or electric")
	}
	return nil
}

// VehicleSummaryResponse represents aggregated stats for fuel & maintenance
type VehicleSummaryResponse struct {
	TotalFuelLiters         float64  `json:"total_fuel_liters"`
	TotalFuelCost           float64  `json:"total_fuel_cost"`
	TotalMaintenanceCost    float64  `json:"total_maintenance_cost"`
	FuelEntriesCount        int      `json:"fuel_entries_count"`
	MaintenanceEntriesCount int      `json:"maintenance_entries_count"`
	AvgCostPerLiter         float64  `json:"avg_cost_per_liter"`
	LastOdometer            *int     `json:"last_odometer,omitempty"`
}

