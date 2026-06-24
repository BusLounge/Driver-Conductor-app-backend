package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/smarttransit/sms-auth-backend/internal/models"
	"github.com/smarttransit/sms-auth-backend/internal/utils"
)

// LoungeOwnerBankDetailsRepository handles bank details storage
type LoungeOwnerBankDetailsRepository struct {
    db *sqlx.DB
}

// NewLoungeOwnerBankDetailsRepository creates repository
func NewLoungeOwnerBankDetailsRepository(db *sqlx.DB) *LoungeOwnerBankDetailsRepository {
    return &LoungeOwnerBankDetailsRepository{db: db}
}

// Create inserts a new bank details record (encrypts sensitive fields)
func (r *LoungeOwnerBankDetailsRepository) Create(d *models.LoungeOwnerBankDetails) (*models.LoungeOwnerBankDetails, error) {
    id := uuid.New()

    acHolderCipher, acHolderIV, err := utils.EncryptString(d.ACHolderName)
    if err != nil {
        return nil, fmt.Errorf("failed to encrypt ac_holder_name: %w", err)
    }

    acNumberCipher, acNumberIV, err := utils.EncryptString(d.ACNumber)
    if err != nil {
        return nil, fmt.Errorf("failed to encrypt ac_number: %w", err)
    }

    var swiftCipher interface{}
    var swiftIV interface{}
    if d.SwiftCode != nil && *d.SwiftCode != "" {
        sc, siv, err := utils.EncryptString(*d.SwiftCode)
        if err != nil {
            return nil, fmt.Errorf("failed to encrypt swift_code: %w", err)
        }
        swiftCipher = sc
        swiftIV = siv
    } else {
        swiftCipher = nil
        swiftIV = nil
    }

    query := `INSERT INTO lounge_owner_bank_details (
        id, bank_name, branch_name, branch_code, ac_type,
        ac_holder_name_encrypted, ac_holder_name_iv,
        ac_number_encrypted, ac_number_iv,
        swift_code_encrypted, swift_code_iv,
        created_at, updated_at
    ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NOW(),NOW()) RETURNING created_at, updated_at`

    var createdAt, updatedAt time.Time
    err = r.db.QueryRowx(query,
        id,
        d.BankName,
        d.BranchName,
        d.BranchCode,
        d.ACType,
        acHolderCipher,
        acHolderIV,
        acNumberCipher,
        acNumberIV,
        swiftCipher,
        swiftIV,
    ).Scan(&createdAt, &updatedAt)
    if err != nil {
        return nil, fmt.Errorf("failed to insert bank details: %w", err)
    }

    d.ID = id
    d.CreatedAt = createdAt
    d.UpdatedAt = updatedAt

    // Do not expose encrypted fields; keep plaintext fields populated
    return d, nil
}

// GetByID retrieves bank details and decrypts sensitive fields
func (r *LoungeOwnerBankDetailsRepository) GetByID(id uuid.UUID) (*models.LoungeOwnerBankDetails, error) {
    var (
        bankName, branchName, branchCode, acType string
        acHolderEnc, acHolderIV, acNumberEnc, acNumberIV sql.NullString
        swiftEnc, swiftIV sql.NullString
        createdAt, updatedAt time.Time
    )

    query := `SELECT id, bank_name, branch_name, branch_code, ac_type,
        ac_holder_name_encrypted, ac_holder_name_iv,
        ac_number_encrypted, ac_number_iv,
        swift_code_encrypted, swift_code_iv, created_at, updated_at
        FROM lounge_owner_bank_details WHERE id = $1`

    row := r.db.QueryRowx(query, id)
    var fetchedID uuid.UUID
    if err := row.Scan(&fetchedID, &bankName, &branchName, &branchCode, &acType,
        &acHolderEnc, &acHolderIV, &acNumberEnc, &acNumberIV,
        &swiftEnc, &swiftIV, &createdAt, &updatedAt); err != nil {
        if err == sql.ErrNoRows {
            return nil, nil
        }
        return nil, fmt.Errorf("failed to query bank details: %w", err)
    }

    // Decrypt
    acHolder, err := utils.DecryptString(acHolderEnc.String, acHolderIV.String)
    if err != nil {
        return nil, fmt.Errorf("failed to decrypt ac_holder_name: %w", err)
    }
    acNumber, err := utils.DecryptString(acNumberEnc.String, acNumberIV.String)
    if err != nil {
        return nil, fmt.Errorf("failed to decrypt ac_number: %w", err)
    }

    var swiftPtr *string
    if swiftEnc.Valid && swiftIV.Valid {
        sc, err := utils.DecryptString(swiftEnc.String, swiftIV.String)
        if err != nil {
            return nil, fmt.Errorf("failed to decrypt swift_code: %w", err)
        }
        swiftPtr = &sc
    }

    out := &models.LoungeOwnerBankDetails{
        ID: fetchedID,
        BankName: bankName,
        BranchName: branchName,
        BranchCode: branchCode,
        ACType: acType,
        ACHolderName: acHolder,
        ACNumber: acNumber,
        SwiftCode: swiftPtr,
        CreatedAt: createdAt,
        UpdatedAt: updatedAt,
    }

    return out, nil
}

// Update updates bank details (encrypting sensitive fields)
func (r *LoungeOwnerBankDetailsRepository) Update(d *models.LoungeOwnerBankDetails) error {
    // Encrypt sensitive fields
    acHolderCipher, acHolderIV, err := utils.EncryptString(d.ACHolderName)
    if err != nil {
        return fmt.Errorf("failed to encrypt ac_holder_name: %w", err)
    }
    acNumberCipher, acNumberIV, err := utils.EncryptString(d.ACNumber)
    if err != nil {
        return fmt.Errorf("failed to encrypt ac_number: %w", err)
    }

    var swiftCipher interface{}
    var swiftIV interface{}
    if d.SwiftCode != nil && *d.SwiftCode != "" {
        sc, siv, err := utils.EncryptString(*d.SwiftCode)
        if err != nil {
            return fmt.Errorf("failed to encrypt swift_code: %w", err)
        }
        swiftCipher = sc
        swiftIV = siv
    } else {
        swiftCipher = nil
        swiftIV = nil
    }

    query := `UPDATE lounge_owner_bank_details SET
        bank_name = $1, branch_name = $2, branch_code = $3, ac_type = $4,
        ac_holder_name_encrypted = $5, ac_holder_name_iv = $6,
        ac_number_encrypted = $7, ac_number_iv = $8,
        swift_code_encrypted = $9, swift_code_iv = $10,
        updated_at = NOW() WHERE id = $11`

    result, err := r.db.Exec(query,
        d.BankName,
        d.BranchName,
        d.BranchCode,
        d.ACType,
        acHolderCipher,
        acHolderIV,
        acNumberCipher,
        acNumberIV,
        swiftCipher,
        swiftIV,
        d.ID,
    )
    if err != nil {
        return fmt.Errorf("failed to update bank details: %w", err)
    }
    rows, err := result.RowsAffected()
    if err != nil {
        return fmt.Errorf("failed to get rows affected: %w", err)
    }
    if rows == 0 {
        return fmt.Errorf("bank details not found")
    }
    return nil
}

// Delete removes bank details by id
func (r *LoungeOwnerBankDetailsRepository) Delete(id uuid.UUID) error {
    query := `DELETE FROM lounge_owner_bank_details WHERE id = $1`
    _, err := r.db.Exec(query, id)
    if err != nil {
        return fmt.Errorf("failed to delete bank details: %w", err)
    }
    return nil
}
