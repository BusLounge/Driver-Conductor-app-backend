package models

import (
	"time"

	"github.com/google/uuid"
)

// LoungeBookingDriverAssignmentStatus represents the status of driver assignment
type LoungeBookingDriverAssignmentStatus string

const (
	DriverAssignmentStatusPending   LoungeBookingDriverAssignmentStatus = "pending"
	DriverAssignmentStatusAssigned  LoungeBookingDriverAssignmentStatus = "assigned"
	DriverAssignmentStatusAccepted  LoungeBookingDriverAssignmentStatus = "accepted"
	DriverAssignmentStatusCompleted LoungeBookingDriverAssignmentStatus = "completed"
	DriverAssignmentStatusCancelled LoungeBookingDriverAssignmentStatus = "cancelled"
)

// LoungeBookingDriverAssignment represents a driver assignment for a booking
type LoungeBookingDriverAssignment struct {
	ID              uuid.UUID                           `json:"id" db:"id"`
	LoungeID        uuid.UUID                           `json:"lounge_id" db:"lounge_id"`
	DriverID        uuid.UUID                           `json:"driver_id" db:"driver_id"`
	LoungeBookingID uuid.UUID                           `json:"lounge_booking_id" db:"lounge_booking_id"`
	GuestName       string                              `json:"guest_name" db:"guest_name"`
	GuestContact    string                              `json:"guest_contact" db:"guest_contact"`
	DriverContact   string                              `json:"driver_contact" db:"driver_contact"`
	Status          LoungeBookingDriverAssignmentStatus `json:"status" db:"status"`
	CreatedAt       time.Time                           `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time                           `json:"updated_at" db:"updated_at"`
}

// CreateLoungeBookingDriverAssignmentRequest represents the request to create an assignment
type CreateLoungeBookingDriverAssignmentRequest struct {
	LoungeID        uuid.UUID `json:"lounge_id" binding:"required"`
	DriverID        uuid.UUID `json:"driver_id" binding:"required"`
	LoungeBookingID uuid.UUID `json:"lounge_booking_id" binding:"required"`
	GuestName       string    `json:"guest_name" binding:"required"`
	GuestContact    string    `json:"guest_contact" binding:"required"`
	DriverContact   string    `json:"driver_contact" binding:"required"`
}

// UpdateLoungeBookingDriverAssignmentRequest represents the request to update an assignment
type UpdateLoungeBookingDriverAssignmentRequest struct {
	Status        *LoungeBookingDriverAssignmentStatus `json:"status"`
	GuestName     *string                              `json:"guest_name"`
	GuestContact  *string                              `json:"guest_contact"`
	DriverContact *string                              `json:"driver_contact"`
}

// LoungeBookingDriverAssignmentResponse is the response model
type LoungeBookingDriverAssignmentResponse struct {
	ID              uuid.UUID                           `json:"id"`
	LoungeID        uuid.UUID                           `json:"lounge_id"`
	DriverID        uuid.UUID                           `json:"driver_id"`
	LoungeBookingID uuid.UUID                           `json:"lounge_booking_id"`
	GuestName       string                              `json:"guest_name"`
	GuestContact    string                              `json:"guest_contact"`
	DriverContact   string                              `json:"driver_contact"`
	Status          LoungeBookingDriverAssignmentStatus `json:"status"`
	CreatedAt       time.Time                           `json:"created_at"`
	UpdatedAt       time.Time                           `json:"updated_at"`
}
