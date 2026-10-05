package models

import (
	"time"

	"github.com/google/uuid"
)

// StaffBankDetails represents a staff member's bank account details for payouts
type StaffBankDetails struct {
	ID                uuid.UUID  `json:"id" db:"id"`
	UserID            uuid.UUID  `json:"user_id" db:"user_id"`
	StaffID           *uuid.UUID `json:"staff_id,omitempty" db:"staff_id"`
	BankName          string     `json:"bank_name" db:"bank_name"`
	BankCode          *string    `json:"bank_code,omitempty" db:"bank_code"`
	BranchName        string     `json:"branch_name" db:"branch_name"`
	BranchCode        *string    `json:"branch_code,omitempty" db:"branch_code"`
	AccountNumber     string     `json:"account_number" db:"account_number"`
	AccountHolderName string     `json:"account_holder_name" db:"account_holder_name"`
	AccountType       string     `json:"account_type" db:"account_type"`
	IsDefault         bool       `json:"is_default" db:"is_default"`
	CreatedAt         time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at" db:"updated_at"`
}

// SaveStaffBankDetailsInput represents the request payload to save or update bank details
type SaveStaffBankDetailsInput struct {
	BankName          string  `json:"bank_name" binding:"required"`
	BankCode          *string `json:"bank_code"`
	BranchName        string  `json:"branch_name" binding:"required"`
	BranchCode        *string `json:"branch_code"`
	AccountNumber     string  `json:"account_number" binding:"required"`
	AccountHolderName string  `json:"account_holder_name" binding:"required"`
	AccountType       string  `json:"account_type"`
}

// PayoutRequestInput represents the request payload when requesting a payout from wallet to bank
type PayoutRequestInput struct {
	Amount            float64 `json:"amount" binding:"required,gt=0"`
	BankName          string  `json:"bank_name" binding:"required"`
	BankCode          *string `json:"bank_code"`
	BranchName        string  `json:"branch_name" binding:"required"`
	BranchCode        *string `json:"branch_code"`
	AccountNumber     string  `json:"account_number" binding:"required"`
	AccountHolderName string  `json:"account_holder_name" binding:"required"`
	AccountType       string  `json:"account_type"`
	SaveBankDetails   bool    `json:"save_bank_details"`
}

// PayoutResponse represents the response after requesting a payout
type PayoutResponse struct {
	Success         bool      `json:"success"`
	TransactionID   uuid.UUID `json:"transaction_id"`
	Amount          float64   `json:"amount"`
	RemainingBal    float64   `json:"remaining_balance"`
	Status          string    `json:"status"`
	GatewayRef      string    `json:"gateway_reference,omitempty"`
	PayoutMethod    string    `json:"payout_method"`
	Message         string    `json:"message"`
	CreatedAt       time.Time `json:"created_at"`
}
