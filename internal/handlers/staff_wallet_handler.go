package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/sirupsen/logrus"
	"github.com/smarttransit/sms-auth-backend/internal/database"
	"github.com/smarttransit/sms-auth-backend/internal/middleware"
	"github.com/smarttransit/sms-auth-backend/internal/models"
	"github.com/smarttransit/sms-auth-backend/internal/services"
)

// StaffWalletHandler handles staff wallet operations including bank details and payouts
type StaffWalletHandler struct {
	bankRepo       *database.StaffBankRepository
	busStaffRepo   *database.BusStaffRepository
	payhereService *services.PayHereService
	db             *sqlx.DB
	logger         *logrus.Logger
}

// NewStaffWalletHandler creates a new StaffWalletHandler
func NewStaffWalletHandler(
	bankRepo *database.StaffBankRepository,
	busStaffRepo *database.BusStaffRepository,
	payhereService *services.PayHereService,
	db *sqlx.DB,
	logger *logrus.Logger,
) *StaffWalletHandler {
	return &StaffWalletHandler{
		bankRepo:       bankRepo,
		busStaffRepo:   busStaffRepo,
		payhereService: payhereService,
		db:             db,
		logger:         logger,
	}
}

// GetBankDetails handles GET /api/v1/staff/bank-details
func (h *StaffWalletHandler) GetBankDetails(c *gin.Context) {
	userCtx, ok := middleware.GetUserContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "unauthorized", Message: "User context not found"})
		return
	}

	details, err := h.bankRepo.GetByUserID(userCtx.UserID)
	if err != nil {
		h.logger.WithError(err).Error("Failed to get staff bank details")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "database_error", Message: "Failed to retrieve bank details"})
		return
	}

	if details == nil {
		c.JSON(http.StatusOK, gin.H{
			"bank_details": nil,
			"has_details":  false,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"bank_details": details,
		"has_details":  true,
	})
}

// SaveBankDetails handles POST /api/v1/staff/bank-details
func (h *StaffWalletHandler) SaveBankDetails(c *gin.Context) {
	userCtx, ok := middleware.GetUserContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "unauthorized", Message: "User context not found"})
		return
	}

	var req models.SaveStaffBankDetailsInput
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "validation_error", Message: err.Error()})
		return
	}

	// Resolve staff profile ID if present
	var staffID *uuid.UUID
	staff, err := h.busStaffRepo.GetByUserID(userCtx.UserID.String())
	if err == nil && staff != nil {
		if parsedStaffID, err := uuid.Parse(staff.ID); err == nil {
			staffID = &parsedStaffID
		}
	}

	accountType := req.AccountType
	if accountType == "" {
		accountType = "savings"
	}

	model := &models.StaffBankDetails{
		UserID:            userCtx.UserID,
		StaffID:           staffID,
		BankName:          req.BankName,
		BankCode:          req.BankCode,
		BranchName:        req.BranchName,
		BranchCode:        req.BranchCode,
		AccountNumber:     req.AccountNumber,
		AccountHolderName: req.AccountHolderName,
		AccountType:       accountType,
		IsDefault:         true,
	}

	saved, err := h.bankRepo.Upsert(model)
	if err != nil {
		h.logger.WithError(err).Error("Failed to save staff bank details")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "database_error", Message: "Failed to save bank details"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"bank_details": saved,
		"message":      "Bank details saved successfully",
	})
}

// RequestPayout handles POST /api/v1/staff/wallet/payout
func (h *StaffWalletHandler) RequestPayout(c *gin.Context) {
	userCtx, ok := middleware.GetUserContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "unauthorized", Message: "User context not found"})
		return
	}

	var req models.PayoutRequestInput
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "validation_error", Message: err.Error()})
		return
	}

	if req.Amount <= 0 {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid_amount", Message: "Payout amount must be greater than zero"})
		return
	}

	// 1. Resolve staff profile ID
	var staffID *uuid.UUID
	staff, _ := h.busStaffRepo.GetByUserID(userCtx.UserID.String())
	if staff != nil {
		if parsedStaffID, err := uuid.Parse(staff.ID); err == nil {
			staffID = &parsedStaffID
		}
	}

	// 2. If SaveBankDetails is requested, upsert bank details
	if req.SaveBankDetails {
		bankModel := &models.StaffBankDetails{
			UserID:            userCtx.UserID,
			StaffID:           staffID,
			BankName:          req.BankName,
			BankCode:          req.BankCode,
			BranchName:        req.BranchName,
			BranchCode:        req.BranchCode,
			AccountNumber:     req.AccountNumber,
			AccountHolderName: req.AccountHolderName,
			AccountType:       req.AccountType,
			IsDefault:         true,
		}
		if _, err := h.bankRepo.Upsert(bankModel); err != nil {
			h.logger.WithError(err).Warn("Could not auto-save bank details during payout")
		}
	}

	// 3. Check wallet balance
	type WalletRecord struct {
		ID      uuid.UUID `db:"id"`
		Balance float64   `db:"balance"`
		Status  string    `db:"status"`
	}

	var wallet WalletRecord
	err := h.db.Get(&wallet, "SELECT id, balance, status FROM wallets_passenger WHERE user_id = $1 LIMIT 1", userCtx.UserID)
	if err != nil {
		h.logger.WithError(err).Error("Wallet not found for user")
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "wallet_not_found", Message: "Staff wallet not found"})
		return
	}

	if wallet.Balance < req.Amount {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "insufficient_balance",
			Message: fmt.Sprintf("Insufficient wallet balance. Available: LKR %.2f, Requested: LKR %.2f", wallet.Balance, req.Amount),
		})
		return
	}

	// 4. Begin DB Transaction to deduct balance & record transaction
	tx, err := h.db.Beginx()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "transaction_error", Message: "Failed to initiate transaction"})
		return
	}
	defer tx.Rollback()

	newBalance := wallet.Balance - req.Amount
	now := time.Now()
	txID := uuid.New()
	payoutRef := fmt.Sprintf("PH-PO-%d-%s", now.Unix(), txID.String()[:8])
	maskedAcc := maskAccountNumber(req.AccountNumber)
	description := fmt.Sprintf("Bank Transfer to %s (%s)", req.BankName, maskedAcc)

	// Update wallet balance
	_, err = tx.Exec(
		"UPDATE wallets_passenger SET balance = $1, updated_at = NOW() WHERE id = $2",
		newBalance, wallet.ID,
	)
	if err != nil {
		h.logger.WithError(err).Error("Failed to update wallet balance")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "database_error", Message: "Failed to deduct wallet balance"})
		return
	}

	// Insert into wallet_transactions
	_, err = tx.Exec(`
		INSERT INTO wallet_transactions (
			id, wallet_id, amount, transaction_type, reference_type,
			gateway_reference, description, status, balance_before, balance_after, created_at
		) VALUES (
			$1, $2, $3, 'debit', 'bank_payout',
			$4, $5, 'pending', $6, $7, NOW()
		)
	`, txID, wallet.ID, req.Amount, payoutRef, description, wallet.Balance, newBalance)
	if err != nil {
		h.logger.WithError(err).Error("Failed to insert wallet transaction")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "database_error", Message: "Failed to record payout transaction"})
		return
	}

	if err := tx.Commit(); err != nil {
		h.logger.WithError(err).Error("Failed to commit wallet payout transaction")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "database_error", Message: "Failed to complete payout deduction"})
		return
	}

	// 5. Invoke PayHere Payout Service
	bankCodeStr := ""
	if req.BankCode != nil {
		bankCodeStr = *req.BankCode
	}
	branchCodeStr := ""
	if req.BranchCode != nil {
		branchCodeStr = *req.BranchCode
	}

	payhereResult, pErr := h.payhereService.ProcessPayout(
		c.Request.Context(),
		txID.String(),
		req.Amount,
		req.BankName,
		bankCodeStr,
		req.BranchName,
		branchCodeStr,
		req.AccountNumber,
		req.AccountHolderName,
	)

	if pErr != nil {
		h.logger.WithError(pErr).Warn("PayHere service reported error during processing")
	} else if payhereResult != nil && payhereResult.GatewayReference != "" {
		payoutRef = payhereResult.GatewayReference
	}

	c.JSON(http.StatusOK, models.PayoutResponse{
		Success:       true,
		TransactionID: txID,
		Amount:        req.Amount,
		RemainingBal:  newBalance,
		Status:        "pending",
		GatewayRef:    payoutRef,
		PayoutMethod:  "PayHere Bank Transfer",
		Message:       fmt.Sprintf("Payout of LKR %.2f to %s has been initiated successfully.", req.Amount, req.BankName),
		CreatedAt:     now,
	})
}

func maskAccountNumber(acc string) string {
	if len(acc) <= 4 {
		return "****"
	}
	return fmt.Sprintf("****%s", acc[len(acc)-4:])
}

