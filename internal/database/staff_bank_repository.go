package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/smarttransit/sms-auth-backend/internal/models"
)

// StaffBankRepository handles bank details storage for bus staff (drivers/conductors)
type StaffBankRepository struct {
	db *sqlx.DB
}

// NewStaffBankRepository creates a new StaffBankRepository
func NewStaffBankRepository(db *sqlx.DB) *StaffBankRepository {
	return &StaffBankRepository{db: db}
}

// GetByUserID retrieves bank details by user_id
func (r *StaffBankRepository) GetByUserID(userID uuid.UUID) (*models.StaffBankDetails, error) {
	var details models.StaffBankDetails
	query := `
		SELECT id, user_id, staff_id, bank_name, bank_code, branch_name, branch_code,
		       account_number, account_holder_name, account_type, is_default, created_at, updated_at
		FROM staff_bank_details
		WHERE user_id = $1
		LIMIT 1
	`
	err := r.db.Get(&details, query, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get staff bank details: %w", err)
	}
	return &details, nil
}

// Upsert inserts or updates bank details for a staff member based on user_id
func (r *StaffBankRepository) Upsert(d *models.StaffBankDetails) (*models.StaffBankDetails, error) {
	if d.AccountType == "" {
		d.AccountType = "savings"
	}

	query := `
		INSERT INTO staff_bank_details (
			user_id, staff_id, bank_name, bank_code, branch_name, branch_code,
			account_number, account_holder_name, account_type, is_default, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW()
		)
		ON CONFLICT (user_id) DO UPDATE SET
			staff_id = EXCLUDED.staff_id,
			bank_name = EXCLUDED.bank_name,
			bank_code = EXCLUDED.bank_code,
			branch_name = EXCLUDED.branch_name,
			branch_code = EXCLUDED.branch_code,
			account_number = EXCLUDED.account_number,
			account_holder_name = EXCLUDED.account_holder_name,
			account_type = EXCLUDED.account_type,
			is_default = EXCLUDED.is_default,
			updated_at = NOW()
		RETURNING id, created_at, updated_at
	`

	var id uuid.UUID
	var createdAt, updatedAt time.Time

	err := r.db.QueryRowx(query,
		d.UserID,
		d.StaffID,
		d.BankName,
		d.BankCode,
		d.BranchName,
		d.BranchCode,
		d.AccountNumber,
		d.AccountHolderName,
		d.AccountType,
		d.IsDefault,
	).Scan(&id, &createdAt, &updatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to upsert staff bank details: %w", err)
	}

	d.ID = id
	d.CreatedAt = createdAt
	d.UpdatedAt = updatedAt
	return d, nil
}

// DeleteByUserID removes bank details for a staff user
func (r *StaffBankRepository) DeleteByUserID(userID uuid.UUID) error {
	query := `DELETE FROM staff_bank_details WHERE user_id = $1`
	_, err := r.db.Exec(query, userID)
	if err != nil {
		return fmt.Errorf("failed to delete staff bank details: %w", err)
	}
	return nil
}
