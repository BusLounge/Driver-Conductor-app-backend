package models

import (
	"time"

	"github.com/google/uuid"
)

// Settlement represents a row in the settlements table
type Settlement struct {
	ID                uuid.UUID  `db:"id" json:"id"`
	PayeeType         string     `db:"payee_type" json:"payee_type"`
	PayeeUserID       uuid.UUID  `db:"payee_user_id" json:"payee_user_id"`
	BookingID         *uuid.UUID `db:"booking_id" json:"booking_id,omitempty"`
	BookingReference  *string    `db:"booking_reference" json:"booking_reference,omitempty"`
	ScheduledTripID   *uuid.UUID `db:"scheduled_trip_id" json:"scheduled_trip_id,omitempty"`
	LoungeBookingID   *uuid.UUID `db:"lounge_booking_id" json:"lounge_booking_id,omitempty"`
	GrossAmount       float64    `db:"gross_amount" json:"gross_amount"`
	CommissionRate    float64    `db:"commission_rate" json:"commission_rate"`
	CommissionAmount  float64    `db:"commission_amount" json:"commission_amount"`
	NetAmount         float64    `db:"net_amount" json:"net_amount"`
	Currency          string     `db:"currency" json:"currency"`
	EarningDate       time.Time  `db:"earning_date" json:"earning_date"`
	Status            string     `db:"status" json:"status"`
	IsPaid            bool       `db:"is_paid" json:"is_paid"`
	PaidAt            *time.Time `db:"paid_at" json:"paid_at,omitempty"`
	SettlementBatchID *uuid.UUID `db:"settlement_batch_id" json:"settlement_batch_id,omitempty"`
	Notes             *string    `db:"notes" json:"notes,omitempty"`
	CreatedAt         time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt         time.Time  `db:"updated_at" json:"updated_at"`
}

// PayoutTracker represents a row in the payout_tracker table
type PayoutTracker struct {
	ID                  uuid.UUID  `db:"id" json:"id"`
	PayeeUserID         uuid.UUID  `db:"payee_user_id" json:"payee_user_id"`
	PayeeType           string     `db:"payee_type" json:"payee_type"`
	PayoutFrequencyDays int        `db:"payout_frequency_days" json:"payout_frequency_days"`
	LastPayoutDate      *time.Time `db:"last_payout_date" json:"last_payout_date,omitempty"`
	NextPayoutDate      *time.Time `db:"next_payout_date" json:"next_payout_date,omitempty"`
	DaysAccumulated     int        `db:"days_accumulated" json:"days_accumulated"`
	HasSpecialRequest   bool       `db:"has_special_request" json:"has_special_request"`
	SpecialRequestDate  *time.Time `db:"special_request_date" json:"special_request_date,omitempty"`
	IsActive            bool       `db:"is_active" json:"is_active"`
	CreatedAt           time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt           time.Time  `db:"updated_at" json:"updated_at"`
}

// TrackerStatus represents the nested payout tracker object in WalletStatusResponse
type TrackerStatus struct {
	DaysAccumulated        int        `json:"days_accumulated"`
	FrequencyDays          int        `json:"frequency_days"`
	NextExpectedPayoutDate *time.Time `json:"next_expected_payout_date"`
	TotalPendingAmount     float64    `json:"total_pending_amount"`
	HasSpecialRequest      bool       `json:"has_special_request"`
}

// WalletStatusResponse is returned by GET /wallet/status
type WalletStatusResponse struct {
	WalletBalance float64       `json:"wallet_balance"`
	Currency      string        `json:"currency"`
	PayoutTracker TrackerStatus `json:"payout_tracker"`
}

// PendingEarningItem represents a single trip commission item waiting in holding
type PendingEarningItem struct {
	ID            uuid.UUID `json:"id"`
	EarningDate   string    `json:"earning_date"`
	ReferenceType string    `json:"reference_type"`
	NetAmount     float64   `json:"net_amount"`
	Status        string    `json:"status"`
}

// PendingEarningsResponse is returned by GET /settlements/pending
type PendingEarningsResponse struct {
	TotalPending float64              `json:"total_pending"`
	Earnings     []PendingEarningItem `json:"earnings"`
}

// WalletTransactionItem represents a single transaction item in the history
type WalletTransactionItem struct {
	TransactionID string    `json:"transaction_id"`
	Date          time.Time `json:"date"`
	Type          string    `json:"type"`
	Description   string    `json:"description"`
	Amount        float64   `json:"amount"`
	BalanceAfter  float64   `json:"balance_after"`
}

// PaginationMeta contains page metadata
type PaginationMeta struct {
	CurrentPage int `json:"current_page"`
	TotalPages  int `json:"total_pages"`
}

// WalletTransactionsResponse is returned by GET /wallet/transactions
type WalletTransactionsResponse struct {
	Transactions []WalletTransactionItem `json:"transactions"`
	Pagination   PaginationMeta          `json:"pagination"`
}

// SpecialPayoutResponse is returned by POST /settlements/special-request
type SpecialPayoutResponse struct {
	Message string `json:"message"`
	Status  string `json:"status"`
}
