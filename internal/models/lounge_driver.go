package models

import (
	"time"
	"github.com/google/uuid"
)


type LoungeDriver struct{

	// id for the driver auto generated
	ID uuid.UUID `db:"id" json:"id"`
	LoungeID uuid.UUID `db:"lounge_id" json:"lounge_id"`

	// driver personal details
	Name string `db:"name" json:"name"`
	NIC string `db:"nic_number" json:"nic_number"`
    ContactNumber string `db:"contact_no" json:"contact_no"`
	VehicleNumber string `db:"vehicle_no" json:"vehicle_no"`
	VehicleType DriverVehicleType `db:"vehicle_type" json:"vehicle_type"`

	// driver status
	Status DriverStatus `db:"status" json:"status"`

	// updated and created time stamps
	CreatedAt time.Time `db:"created_at"  json:"created_at"`
	UpdatedAt time.Time `db:"updated_at"  json:"updated_at"`


}


// vehicle type ENUM for the driver
type DriverVehicleType string 

const (
	DriverVehicleThreeWheeler DriverVehicleType = "three_wheeler"
	DriverVehicleCar DriverVehicleType = "car"
	DriverVehicleVan DriverVehicleType = "van"
)

// driver status ENUM
type DriverStatus string 

const (
	DriverStatusActive DriverStatus = "active"
	DriverStatusInactive DriverStatus = "inactive"
)

