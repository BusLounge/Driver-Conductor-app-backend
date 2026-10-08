package database

import (
	"context"
	"database/sql"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/smarttransit/sms-auth-backend/internal/models"
)

// SettlementRepository handles settlement and payout tracker database operations
type SettlementRepository struct {
	db *sqlx.DB
}

// NewSettlementRepository creates a new SettlementRepository
func NewSettlementRepository(db *sqlx.DB) *SettlementRepository {
	return &SettlementRepository{db: db}
}

// GetWalletStatus retrieves the wallet balance and payout tracker status for a user
func (r *SettlementRepository) GetWalletStatus(ctx context.Context, userID uuid.UUID, payeeType string) (*models.WalletStatusResponse, error) {
	// 1. Fetch wallet balance from wallets_passenger
	var balance float64
	err := r.db.GetContext(ctx, &balance, "SELECT COALESCE(balance, 0) FROM wallets_passenger WHERE user_id = $1 LIMIT 1", userID)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	// 2. Fetch total pending amount from settlements table
	var pendingAmount float64
	err = r.db.GetContext(ctx, &pendingAmount, `
		SELECT COALESCE(SUM(net_amount), 0)
		FROM settlements
		WHERE payee_user_id = $1 AND status = 'pending' AND is_paid = false
	`, userID)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	// Normalize payeeType
	if payeeType == "" {
		payeeType = "driver"
	}

	// 3. Fetch payout tracker record
	type trackerRecord struct {
		DaysAccumulated     int        `db:"days_accumulated"`
		PayoutFrequencyDays int        `db:"payout_frequency_days"`
		NextPayoutDate      *time.Time `db:"next_payout_date"`
		HasSpecialRequest   bool       `db:"has_special_request"`
	}

	var rec trackerRecord
	trackerErr := r.db.GetContext(ctx, &rec, `
		SELECT days_accumulated, payout_frequency_days, next_payout_date, has_special_request
		FROM payout_tracker
		WHERE payee_user_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`, userID)

	if trackerErr == sql.ErrNoRows {
		// Initialize default record for user if not exists
		nextDate := time.Now().AddDate(0, 0, 14)
		_, _ = r.db.ExecContext(ctx, `
			INSERT INTO payout_tracker (
				id, payee_user_id, payee_type, payout_frequency_days,
				next_payout_date, days_accumulated, has_special_request, is_active, created_at, updated_at
			) VALUES (
				$1, $2, $3, 14, $4, 0, false, true, NOW(), NOW()
			) ON CONFLICT (payee_user_id, payee_type) DO NOTHING
		`, uuid.New(), userID, payeeType, nextDate)

		rec = trackerRecord{
			DaysAccumulated:     0,
			PayoutFrequencyDays: 14,
			NextPayoutDate:      &nextDate,
			HasSpecialRequest:   false,
		}
	} else if trackerErr != nil {
		return nil, trackerErr
	}

	frequency := rec.PayoutFrequencyDays
	if frequency == 0 {
		frequency = 14
	}

	return &models.WalletStatusResponse{
		WalletBalance: balance,
		Currency:      "LKR",
		PayoutTracker: models.TrackerStatus{
			DaysAccumulated:        rec.DaysAccumulated,
			FrequencyDays:          frequency,
			NextExpectedPayoutDate: rec.NextPayoutDate,
			TotalPendingAmount:     pendingAmount,
			HasSpecialRequest:      rec.HasSpecialRequest,
		},
	}, nil
}

// GetPendingSettlements returns list of pending trip commissions
func (r *SettlementRepository) GetPendingSettlements(ctx context.Context, userID uuid.UUID) (*models.PendingEarningsResponse, error) {
	type rowItem struct {
		ID            uuid.UUID `db:"id"`
		EarningDate   time.Time `db:"earning_date"`
		ReferenceType string    `db:"ref_type"`
		NetAmount     float64   `db:"net_amount"`
		Status        string    `db:"status"`
	}

	var rows []rowItem
	err := r.db.SelectContext(ctx, &rows, `
		SELECT 
			id,
			earning_date,
			CASE 
				WHEN scheduled_trip_id IS NOT NULL THEN 'bus_trip'
				WHEN lounge_booking_id IS NOT NULL THEN 'lounge_booking'
				ELSE 'bus_trip'
			END AS ref_type,
			net_amount,
			status
		FROM settlements
		WHERE payee_user_id = $1 AND status = 'pending' AND is_paid = false
		ORDER BY earning_date DESC, created_at DESC
	`, userID)

	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	var totalPending float64
	earnings := make([]models.PendingEarningItem, 0, len(rows))
	for _, row := range rows {
		totalPending += row.NetAmount
		earnings = append(earnings, models.PendingEarningItem{
			ID:            row.ID,
			EarningDate:   row.EarningDate.Format(time.RFC3339),
			ReferenceType: row.ReferenceType,
			NetAmount:     row.NetAmount,
			Status:        row.Status,
		})
	}

	return &models.PendingEarningsResponse{
		TotalPending: totalPending,
		Earnings:     earnings,
	}, nil
}

// GetWalletTransactions returns paginated wallet transaction history
func (r *SettlementRepository) GetWalletTransactions(ctx context.Context, userID uuid.UUID, page, limit int) (*models.WalletTransactionsResponse, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	var walletID uuid.UUID
	err := r.db.GetContext(ctx, &walletID, "SELECT id FROM wallets_passenger WHERE user_id = $1 LIMIT 1", userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return &models.WalletTransactionsResponse{
				Transactions: []models.WalletTransactionItem{},
				Pagination: models.PaginationMeta{
					CurrentPage: page,
					TotalPages:  0,
				},
			}, nil
		}
		return nil, err
	}

	var totalCount int
	err = r.db.GetContext(ctx, &totalCount, "SELECT COUNT(*) FROM wallet_transactions WHERE wallet_id = $1", walletID)
	if err != nil {
		return nil, err
	}

	type txRow struct {
		ID              uuid.UUID `db:"id"`
		CreatedAt       time.Time `db:"created_at"`
		TransactionType string    `db:"transaction_type"`
		Description     *string   `db:"description"`
		Amount          float64   `db:"amount"`
		BalanceAfter    *float64  `db:"balance_after"`
	}

	var rows []txRow
	err = r.db.SelectContext(ctx, &rows, `
		SELECT id, created_at, transaction_type, description, amount, balance_after
		FROM wallet_transactions
		WHERE wallet_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, walletID, limit, offset)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	items := make([]models.WalletTransactionItem, 0, len(rows))
	for _, r := range rows {
		desc := ""
		if r.Description != nil {
			desc = *r.Description
		}
		var balAfter float64
		if r.BalanceAfter != nil {
			balAfter = *r.BalanceAfter
		}
		items = append(items, models.WalletTransactionItem{
			TransactionID: r.ID.String(),
			Date:          r.CreatedAt,
			Type:          r.TransactionType,
			Description:   desc,
			Amount:        r.Amount,
			BalanceAfter:  balAfter,
		})
	}

	totalPages := 0
	if totalCount > 0 {
		totalPages = int(math.Ceil(float64(totalCount) / float64(limit)))
	}

	return &models.WalletTransactionsResponse{
		Transactions: items,
		Pagination: models.PaginationMeta{
			CurrentPage: page,
			TotalPages:  totalPages,
		},
	}, nil
}

// RequestSpecialPayout records an early payout request for the payee
func (r *SettlementRepository) RequestSpecialPayout(ctx context.Context, userID uuid.UUID, payeeType string) error {
	if payeeType == "" {
		payeeType = "driver"
	}

	query := `
		INSERT INTO payout_tracker (
			id, payee_user_id, payee_type, payout_frequency_days,
			next_payout_date, days_accumulated, has_special_request, special_request_date,
			is_active, created_at, updated_at
		) VALUES (
			$1, $2, $3, 14, CURRENT_DATE + INTERVAL '14 days', 0, true, NOW(), true, NOW(), NOW()
		)
		ON CONFLICT (payee_user_id, payee_type) DO UPDATE SET
			has_special_request = true,
			special_request_date = NOW(),
			updated_at = NOW()
	`

	_, err := r.db.ExecContext(ctx, query, uuid.New(), userID, payeeType)
	return err
}
