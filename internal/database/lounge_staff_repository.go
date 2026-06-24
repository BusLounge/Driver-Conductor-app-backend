package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/smarttransit/sms-auth-backend/internal/models"
)

// LoungeStaffRepository handles database operations for lounge staff
type LoungeStaffRepository struct {
	db *sqlx.DB
}

// NewLoungeStaffRepository creates a new lounge staff repository
func NewLoungeStaffRepository(db *sqlx.DB) *LoungeStaffRepository {
	return &LoungeStaffRepository{db: db}
}

// NEW ADDITION inoder to add_staff record
func (r *LoungeStaffRepository) CreateLoungeStaff(userID uuid.UUID) (*models.LoungeStaff, error) {
	return nil, nil
}

// NEW METHOD TO ADD COMPLETE STAFF DETAILS (THE OLD WAS LEFT TO BACKEND COMPATABILITY)
func (r *LoungeStaffRepository) AddStaffToLoungeWithCompleteData(
	loungeID uuid.UUID,
	userID uuid.UUID,
	employmentStatus string,
	fullName string,
	nicNumber string,
	email string,
) (*models.LoungeStaff, error) {

	// feeding data into the variable
	staff := &models.LoungeStaff{
		ID:               uuid.New(),
		LoungeID:         loungeID,
		UserID:           userID,
		FullName:         sql.NullString{String: fullName, Valid: fullName != ""},
		NICNumber:        sql.NullString{String: nicNumber, Valid: nicNumber != ""},
		Email:            sql.NullString{String: email, Valid: email != ""},
		ProfileCompleted: true,                                   // Set to true since all data is provided
		ApprovalStatus:   models.LoungeStaffApproveStatusPending, // Set approval status to pending
		EmploymentStatus: models.LoungeStaffEmploymentStatus(employmentStatus),
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	// database query
	query := `
        INSERT INTO lounge_staff (
            id, lounge_id, user_id, full_name, nic_number, email,
            profile_completed, approval_status, employment_status, created_at, updated_at
        )
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
        RETURNING id, created_at, updated_at
    `

	err := r.db.QueryRowx(
		query,
		staff.ID,
		loungeID,
		userID,
		staff.FullName,
		staff.NICNumber,
		staff.Email,
		staff.ProfileCompleted,
		staff.ApprovalStatus,
		staff.EmploymentStatus,
		staff.CreatedAt,
		staff.UpdatedAt,
	).Scan(&staff.ID, &staff.CreatedAt, &staff.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to add staff: %w", err)
	}

	return staff, nil
}

// (THIS WILL BE REMOVED IF NOT NEEDED JUST KEPT FOR BACKWARD COMPATABILITY)
// AddStaffToLounge adds a staff member to a lounge (staff is only approved by lounge owner - NO INVITATION)
func (r *LoungeStaffRepository) AddStaffToLounge(
	loungeID uuid.UUID,
	userID uuid.UUID,
	employmentStatus string,
) (*models.LoungeStaff, error) {
	staff := &models.LoungeStaff{
		ID:               uuid.New(),
		LoungeID:         loungeID,
		UserID:           userID,
		ProfileCompleted: false,
		EmploymentStatus: models.LoungeStaffEmploymentStatus(employmentStatus),
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	query := `
		INSERT INTO lounge_staff (
			id, lounge_id, user_id, profile_completed,
			employment_status, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at
	`

	err := r.db.QueryRowx(
		query,
		staff.ID,
		loungeID,
		userID,
		staff.ProfileCompleted,
		staff.EmploymentStatus,
		staff.CreatedAt,
		staff.UpdatedAt,
	).Scan(&staff.ID, &staff.CreatedAt, &staff.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to add staff: %w", err)
	}

	return staff, nil
}

// GetStaffByID retrieves a staff member by ID
func (r *LoungeStaffRepository) GetStaffByID(id uuid.UUID) (*models.LoungeStaff, error) {
	var staff models.LoungeStaff
	query := `SELECT * FROM lounge_staff WHERE id = $1`
	err := r.db.Get(&staff, query, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get staff: %w", err)
	}
	return &staff, nil
}

// GetStaffByLoungeID retrieves all staff for a specific lounge
func (r *LoungeStaffRepository) GetStaffByLoungeID(loungeID uuid.UUID) ([]models.LoungeStaff, error) {
	var staff []models.LoungeStaff
	query := `
		SELECT * FROM lounge_staff 
		WHERE lounge_id = $1 
		ORDER BY created_at DESC
	`
	err := r.db.Select(&staff, query, loungeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get staff: %w", err)
	}
	return staff, nil
}

// GetStaffByUserID retrieves a staff member by user_id
func (r *LoungeStaffRepository) GetStaffByUserID(userID uuid.UUID) (*models.LoungeStaff, error) {
	var staff models.LoungeStaff
	query := `SELECT * FROM lounge_staff WHERE user_id = $1`
	err := r.db.Get(&staff, query, userID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get staff: %w", err)
	}
	return &staff, nil
}

// this must be work with update of user table paralalley
// UpdateStaffProfile updates staff profile information when they complete registration
func (r *LoungeStaffRepository) UpdateStaffProfile(
	userID uuid.UUID,
	fullName string,
	nicNumber string,
	email *string,
) error {
	query := `
		UPDATE lounge_staff 
		SET 
			full_name = $1,
			nic_number = $2,
			email = $3,
			profile_completed = true,
			updated_at = NOW()
		WHERE user_id = $4
	`

	var emailValue interface{}
	if email != nil && *email != "" {
		emailValue = *email
	} else {
		emailValue = nil
	}

	result, err := r.db.Exec(query, fullName, nicNumber, emailValue, userID)
	if err != nil {
		return fmt.Errorf("failed to update staff profile: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("staff not found")
	}

	return nil
}

// UpdateStaffEmploymentStatus updates staff employment status
func (r *LoungeStaffRepository) UpdateStaffEmploymentStatus(
	id uuid.UUID,
	status string,
) error {
	query := `
		UPDATE lounge_staff 
		SET 
			employment_status = $1,
			terminated_date = CASE
				WHEN $1 = 'terminated' THEN NOW()
				ELSE terminated_date
			END,
			updated_at = NOW()
		WHERE id = $2
	`

	_, err := r.db.Exec(query, status, id)
	if err != nil {
		return fmt.Errorf("failed to update staff status: %w", err)
	}

	return nil
}

// RemoveStaff deletes a staff member
func (r *LoungeStaffRepository) RemoveStaff(id uuid.UUID) error {
	query := `DELETE FROM lounge_staff WHERE id = $1`
	_, err := r.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to remove staff: %w", err)
	}
	return nil
}

// GetActiveStaffByLoungeID retrieves all active staff for a lounge
func (r *LoungeStaffRepository) GetActiveStaffByLoungeID(loungeID uuid.UUID) ([]models.LoungeStaff, error) {
	var staff []models.LoungeStaff
	query := `
		SELECT * FROM lounge_staff 
		WHERE lounge_id = $1 AND employment_status = 'active'
		ORDER BY created_at DESC
	`
	err := r.db.Select(&staff, query, loungeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get active staff: %w", err)
	}
	return staff, nil
}

// GetStaffWithUserDetails retrieves staff with user phone via JOIN
func (r *LoungeStaffRepository) GetStaffWithUserDetails(staffID uuid.UUID) (map[string]interface{}, error) {
	query := `
		SELECT 
			ls.*,
			u.phone as user_phone
		FROM lounge_staff ls
		JOIN users u ON ls.user_id = u.id
		WHERE ls.id = $1
	`

	result := make(map[string]interface{})
	err := r.db.QueryRowx(query, staffID).MapScan(result)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get staff with user details: %w", err)
	}

	return result, nil
}

// Update staffApproval status
func (r *LoungeStaffRepository) UpdateStaffApprovalStatus(
	staffID uuid.UUID,
	approvalStatus string,
	employmentStatus *string,
) error {

	query := `
		UPDATE lounge_staff
		SET
			approval_status = $1,
			employment_status = COALESCE($2, employment_status),
			hired_date = CASE
				WHEN $1 = 'approved' THEN NOW()
				ELSE hired_date
			END,	
			updated_at = NOW()
		WHERE id = $3
	`

	result, err := r.db.Exec(
		query,
		approvalStatus,
		employmentStatus,
		staffID,
	)

	if err != nil {
		return fmt.Errorf("failed to update staff approval status: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("staff member not found")
	}

	return nil
}

// function to get loungeStaff according the approvalStatus
func (r *LoungeStaffRepository) GetStaffByLoungeWithFilter(
	loungeID uuid.UUID,
	approvalStatusFilter *string, // nil for all, or "pending", "approved", "declined"
) ([]models.LoungeStaff, error) {

	var staff []models.LoungeStaff

	query := `
		SELECT * FROM lounge_staff
		WHERE lounge_id = $1
	`
	args := []interface{}{loungeID}

	if approvalStatusFilter != nil {
		query += ` AND approval_status = $2`
		args = append(args, *approvalStatusFilter)
	}

	query += ` ORDER BY created_at DESC`

	err := r.db.Select(&staff, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get staff by lounge: %w", err)
	}

	return staff, nil
}

// GetStaffByIDWithDetails retrieves a staff member and verifies ownership
func (r *LoungeStaffRepository) GetStaffByIDWithDetails(staffID uuid.UUID) (*models.LoungeStaff, error) {
	var staff models.LoungeStaff

	query := `
		SELECT * FROM lounge_staff
		WHERE id = $1
	`

	err := r.db.Get(&staff, query, staffID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get staff: %w", err)
	}

	return &staff, nil
}

// AddStaffToLoungeDirectByOwner
func (r *LoungeStaffRepository) AddStaffToLoungeDirectByOwner(
	loungeID uuid.UUID,
	userID uuid.UUID,
	fullName string,
	nicNumber string,
) (*models.LoungeStaff, error) {

	staff := &models.LoungeStaff{
		ID:               uuid.New(),
		LoungeID:         loungeID,
		UserID:           userID,
		FullName:         sql.NullString{String: fullName, Valid: fullName != ""},
		NICNumber:        sql.NullString{String: nicNumber, Valid: nicNumber != ""},
		ProfileCompleted: true,
		ApprovalStatus:   models.LoungeStaffApproveStatusApproved,     // Approved immediately
		EmploymentStatus: models.LoungeStaffEmploymentActive,          // Active immediately
		HiredDate:        sql.NullTime{Time: time.Now(), Valid: true}, // Current date/time
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	query := `
		INSERT INTO lounge_staff (
			id, lounge_id, user_id, full_name, nic_number,
			profile_completed, approval_status, employment_status,
			hired_date, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, created_at, updated_at
	`
	err := r.db.QueryRowx(
		query,
		staff.ID,
		staff.LoungeID,
		staff.UserID,
		staff.FullName,
		staff.NICNumber,
		staff.ProfileCompleted,
		staff.ApprovalStatus,
		staff.EmploymentStatus,
		staff.HiredDate,
		staff.CreatedAt,
		staff.UpdatedAt,
	).Scan(&staff.ID, &staff.CreatedAt, &staff.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to add staff with approval: %w", err)
	}

	return staff, nil
}

// GetStaffByLoungeIDAndUserID retrieves a staff member by lounge ID and user ID
func (r *LoungeStaffRepository) GetStaffByLoungeIDAndUserID(ctx context.Context, loungeID, userID uuid.UUID) (*models.LoungeStaff, error) {
	var staff models.LoungeStaff
	query := `SELECT * FROM lounge_staff WHERE lounge_id = $1 AND user_id = $2`
	err := r.db.GetContext(ctx, &staff, query, loungeID, userID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &staff, nil
}

// GetApprovedStaffByUserID retrieves an approved and active lounge staff by user ID
// Returns error if staff is not approved or not active
func (r *LoungeStaffRepository) GetApprovedStaffaByUserID(userID uuid.UUID)(*models.LoungeStaff,error){

	query := `
		SELECT
			id, lounge_id, user_id, full_name, nic_number, email,
			profile_completed, approval_status, employment_status,
			hired_date, terminated_date, notes, created_at, updated_at
		FROM lounge_staff
		WHERE user_id = $1
			AND approval_status = 'approved'
			AND employment_status = 'active'
			AND profile_completed = true
		LIMIT 1
	`
	var staff models.LoungeStaff
	err := r.db.QueryRowx(query, userID).StructScan(&staff)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get approved staff: %w", err)
	}

	return &staff, nil
}

// UpdateProfile updates lounge staff profile with optional fields
func (r *LoungeStaffRepository) UpdateProfile(
	userID uuid.UUID,
	fullName *string,
	nicNumber *string,
	email *string,
	notes *string,
) error {
	// Build dynamic UPDATE query with only provided fields
	var updates []string
	var args []interface{}
	argIndex := 1

	if fullName != nil {
		updates = append(updates, fmt.Sprintf("full_name = $%d", argIndex))
		args = append(args, *fullName)
		argIndex++
	}

	if nicNumber != nil {
		updates = append(updates, fmt.Sprintf("nic_number = $%d", argIndex))
		args = append(args, *nicNumber)
		argIndex++
	}

	if email != nil {
		updates = append(updates, fmt.Sprintf("email = $%d", argIndex))
		args = append(args, *email)
		argIndex++
	}

	if notes != nil {
		updates = append(updates, fmt.Sprintf("notes = $%d", argIndex))
		args = append(args, *notes)
		argIndex++
	}

	// Always update updated_at
	updates = append(updates, fmt.Sprintf("updated_at = $%d", argIndex))
	args = append(args, time.Now())
	argIndex++

	// Add user_id as WHERE clause
	args = append(args, userID)

	// Build the update string
	var updateStr string
	for i, u := range updates {
		if i > 0 {
			updateStr += ", "
		}
		updateStr += u
	}

	query := fmt.Sprintf(
		"UPDATE lounge_staff SET %s WHERE user_id = $%d",
		updateStr,
		argIndex,
	)

	result, err := r.db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("failed to update lounge staff profile: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("no lounge staff found with user_id %s", userID)
	}

	return nil
}
