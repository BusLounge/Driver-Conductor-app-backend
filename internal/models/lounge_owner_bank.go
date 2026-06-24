package models

import (
	"time"

	"github.com/google/uuid"
)

// LoungeOwnerBankDetails represents decrypted bank details returned by the API
type LoungeOwnerBankDetails struct {
    ID        uuid.UUID `json:"id" db:"id"`
    BankName  string    `json:"bank_name" db:"bank_name"`
    BranchName string   `json:"branch_name" db:"branch_name"`
    BranchCode string   `json:"branch_code" db:"branch_code"`
    ACType     string   `json:"ac_type" db:"ac_type"`

    // Sensitive fields (decrypted) - not stored directly in DB by this struct
    ACHolderName string  `json:"ac_holder_name"`
    ACNumber     string  `json:"ac_number"`
    SwiftCode    *string `json:"swift_code,omitempty"`

    CreatedAt time.Time `json:"created_at" db:"created_at"`
    UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// LoungeOwnerBankLink links a lounge owner / lounge to bank details
type LoungeOwnerBankLink struct {
    ID            uuid.UUID  `json:"id" db:"id"`
    OwnerID       uuid.UUID  `json:"owner_id" db:"owner_id"`
    LoungeID      *uuid.UUID `json:"lounge_id,omitempty" db:"lounge_id"`
    BankDetailsID uuid.UUID  `json:"bank_details_id" db:"bank_details_id"`
    CreatedAt     time.Time  `json:"created_at" db:"created_at"`
}
