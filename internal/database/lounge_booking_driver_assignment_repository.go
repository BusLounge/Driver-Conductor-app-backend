package database

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/smarttransit/sms-auth-backend/internal/models"
)

// LoungeBookingDriverAssignmentRepository handles lounge booking driver assignment database operations
type LoungeBookingDriverAssignmentRepository struct {
	db *sqlx.DB
}

// NewLoungeBookingDriverAssignmentRepository creates a new driver assignment repository
func NewLoungeBookingDriverAssignmentRepository(db *sqlx.DB) *LoungeBookingDriverAssignmentRepository {
	return &LoungeBookingDriverAssignmentRepository{db: db}
}

// CreateAssignment creates a new driver assignment
func (r *LoungeBookingDriverAssignmentRepository) CreateAssignment(assignment *models.LoungeBookingDriverAssignment) error {
	query := `
		INSERT INTO lounge_booking_driver_assignments 
		(id, lounge_id, driver_id, lounge_booking_id, guest_name, guest_contact, driver_contact, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
	`
	_, err := r.db.Exec(
		query,
		assignment.ID,
		assignment.LoungeID,
		assignment.DriverID,
		assignment.LoungeBookingID,
		assignment.GuestName,
		assignment.GuestContact,
		assignment.DriverContact,
		assignment.Status,
	)
	return err
}

// GetAssignmentByID retrieves an assignment by ID
func (r *LoungeBookingDriverAssignmentRepository) GetAssignmentByID(assignmentID uuid.UUID) (*models.LoungeBookingDriverAssignment, error) {
	var assignment models.LoungeBookingDriverAssignment
	query := `
		SELECT id, lounge_id, driver_id, lounge_booking_id, guest_name, guest_contact, driver_contact, status, created_at, updated_at
		FROM lounge_booking_driver_assignments
		WHERE id = $1
	`
	err := r.db.Get(&assignment, query, assignmentID)
	if err != nil {
		return nil, err
	}
	return &assignment, nil
}

// GetAssignmentByBookingID retrieves assignments for a booking
func (r *LoungeBookingDriverAssignmentRepository) GetAssignmentByBookingID(bookingID uuid.UUID) ([]models.LoungeBookingDriverAssignment, error) {
	var assignments []models.LoungeBookingDriverAssignment
	query := `
		SELECT id, lounge_id, driver_id, lounge_booking_id, guest_name, guest_contact, driver_contact, status, created_at, updated_at
		FROM lounge_booking_driver_assignments
		WHERE lounge_booking_id = $1
		ORDER BY created_at DESC
	`
	err := r.db.Select(&assignments, query, bookingID)
	return assignments, err
}

// GetAssignmentsByDriverID retrieves assignments for a driver
func (r *LoungeBookingDriverAssignmentRepository) GetAssignmentsByDriverID(driverID uuid.UUID, status *string, limit, offset int) ([]models.LoungeBookingDriverAssignment, error) {
	var assignments []models.LoungeBookingDriverAssignment
	query := `
		SELECT id, lounge_id, driver_id, lounge_booking_id, guest_name, guest_contact, driver_contact, status, created_at, updated_at
		FROM lounge_booking_driver_assignments
		WHERE driver_id = $1
	`
	args := []interface{}{driverID}

	if status != nil {
		query += fmt.Sprintf(" AND status = $%d", len(args)+1)
		args = append(args, *status)
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	args = append(args, limit, offset)

	err := r.db.Select(&assignments, query, args...)
	return assignments, err
}

// GetAssignmentsByLoungeID retrieves assignments for a lounge
func (r *LoungeBookingDriverAssignmentRepository) GetAssignmentsByLoungeID(loungeID uuid.UUID, status *string, limit, offset int) ([]models.LoungeBookingDriverAssignment, error) {
	var assignments []models.LoungeBookingDriverAssignment
	query := `
		SELECT id, lounge_id, driver_id, lounge_booking_id, guest_name, guest_contact, driver_contact, status, created_at, updated_at
		FROM lounge_booking_driver_assignments
		WHERE lounge_id = $1
	`
	args := []interface{}{loungeID}

	if status != nil {
		query += fmt.Sprintf(" AND status = $%d", len(args)+1)
		args = append(args, *status)
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	args = append(args, limit, offset)

	err := r.db.Select(&assignments, query, args...)
	return assignments, err
}

// UpdateAssignment updates an existing assignment
func (r *LoungeBookingDriverAssignmentRepository) UpdateAssignment(assignmentID uuid.UUID, updates *models.UpdateLoungeBookingDriverAssignmentRequest) error {
	var setClauses []string
	var args []interface{}
	argIndex := 1

	if updates.Status != nil {
		setClauses = append(setClauses, fmt.Sprintf("status = $%d", argIndex))
		args = append(args, *updates.Status)
		argIndex++
	}

	if updates.GuestName != nil {
		setClauses = append(setClauses, fmt.Sprintf("guest_name = $%d", argIndex))
		args = append(args, *updates.GuestName)
		argIndex++
	}

	if updates.GuestContact != nil {
		setClauses = append(setClauses, fmt.Sprintf("guest_contact = $%d", argIndex))
		args = append(args, *updates.GuestContact)
		argIndex++
	}

	if updates.DriverContact != nil {
		setClauses = append(setClauses, fmt.Sprintf("driver_contact = $%d", argIndex))
		args = append(args, *updates.DriverContact)
		argIndex++
	}

	if len(setClauses) == 0 {
		return nil
	}

	setClauses = append(setClauses, fmt.Sprintf("updated_at = NOW()"))

	query := fmt.Sprintf("UPDATE lounge_booking_driver_assignments SET %s WHERE id = $%d",
		fmt.Sprintf("%v", setClauses), argIndex)
	args = append(args, assignmentID)

	_, err := r.db.Exec(query, args...)
	return err
}

// DeleteAssignment deletes an assignment
func (r *LoungeBookingDriverAssignmentRepository) DeleteAssignment(assignmentID uuid.UUID) error {
	query := `DELETE FROM lounge_booking_driver_assignments WHERE id = $1`
	_, err := r.db.Exec(query, assignmentID)
	return err
}

// CancelAssignment cancels an assignment by ID
func (r *LoungeBookingDriverAssignmentRepository) CancelAssignment(assignmentID uuid.UUID) error {
	query := `UPDATE lounge_booking_driver_assignments SET status = 'cancelled', updated_at = NOW() WHERE id = $1`
	_, err := r.db.Exec(query, assignmentID)
	return err
}

// CompleteAssignment completes an assignment by ID
func (r *LoungeBookingDriverAssignmentRepository) CompleteAssignment(assignmentID uuid.UUID) error {
	query := `UPDATE lounge_booking_driver_assignments SET status = 'completed', updated_at = NOW() WHERE id = $1`
	_, err := r.db.Exec(query, assignmentID)
	return err
}

// CheckIfDriverAssigned checks if a driver is already assigned to a booking
// Returns the driver assignment if one exists (in non-terminal status), nil otherwise
func (r *LoungeBookingDriverAssignmentRepository) CheckIfDriverAssigned(bookingID uuid.UUID) (*models.LoungeBookingDriverAssignment, error) {
	var assignment models.LoungeBookingDriverAssignment
	checkQuery := `
		SELECT id, lounge_id, driver_id, lounge_booking_id, guest_name, guest_contact, driver_contact, status, created_at, updated_at
		FROM lounge_booking_driver_assignments
		WHERE lounge_booking_id = $1 AND status NOT IN ('cancelled', 'completed')
		LIMIT 1
	`
	err := r.db.Get(&assignment, checkQuery, bookingID)
	if err != nil {
		return nil, err
	}
	return &assignment, nil
}

// UpdateTransportBookingStatus updates the status of the related transport booking
func (r *LoungeBookingDriverAssignmentRepository) UpdateTransportBookingStatus(loungeBookingID uuid.UUID, status string) error {
	query := `
		UPDATE transport_bookings 
		SET status = $2 
		WHERE booking_id = (SELECT master_booking_id FROM lounge_bookings WHERE id = $1)
	`
	_, err := r.db.Exec(query, loungeBookingID, status)
	return err
}
