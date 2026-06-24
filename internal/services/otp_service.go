package services

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/smarttransit/sms-auth-backend/internal/database"
	"github.com/smarttransit/sms-auth-backend/internal/models"
)

const (
	// OTPLength is the length of the OTP code
	OTPLength = 6

	// OTPExpiryDuration is how long an OTP is valid (5 minutes)
	OTPExpiryDuration = 5 * time.Minute

	// MaxOTPAttempts is the maximum number of validation attempts
	MaxOTPAttempts = 3
)

var (
	// ErrOTPExpired indicates the OTP has expired
	ErrOTPExpired = fmt.Errorf("OTP has expired")

	// ErrOTPInvalid indicates the OTP is incorrect
	ErrOTPInvalid = fmt.Errorf("invalid OTP code")

	// ErrMaxAttemptsExceeded indicates too many failed validation attempts
	ErrMaxAttemptsExceeded = fmt.Errorf("maximum OTP validation attempts exceeded")

	// ErrNoOTPFound indicates no OTP exists for the phone number
	ErrNoOTPFound = fmt.Errorf("no OTP found for this phone number")

	// ErrOTPAlreadyUsed indicates the OTP has already been successfully validated
	ErrOTPAlreadyUsed = fmt.Errorf("OTP has already been used")
)

// OTPService handles OTP generation and validation
type OTPService struct {
	db database.DB
}

// NewOTPService creates a new OTP service
func NewOTPService(db database.DB) *OTPService {
	return &OTPService{
		db: db,
	}
}

// GenerateOTP generates a new 6-digit OTP for the given phone number
// It invalidates any existing OTPs for the phone number and stores IP/User-Agent for security tracking
func (s *OTPService) GenerateOTP(phone, ipAddress, userAgent string) (string, error) {
	// Invalidate any existing OTPs for this phone
	if err := s.InvalidateOTP(phone); err != nil {
		return "", fmt.Errorf("failed to invalidate existing OTP: %w", err)
	}

	// Generate random 6-digit OTP
	otp, err := generateRandomOTP()
	if err != nil {
		return "", fmt.Errorf("failed to generate OTP: %w", err)
	}

	// Calculate expiry time
	expiresAt := time.Now().Add(OTPExpiryDuration)

	// Store in database with IP address and user agent for security tracking
	query := `
		INSERT INTO otp_verifications (phone, otp_code, purpose, expires_at, attempts, max_attempts, ip_address, user_agent)
		VALUES ($1, $2, 'authentication', $3, 0, $4, $5, $6)
	`

	_, err = s.db.Exec(query, phone, otp, expiresAt, MaxOTPAttempts, ipAddress, userAgent)
	if err != nil {
		return "", fmt.Errorf("failed to store OTP: %w", err)
	}

	return otp, nil
}

// GenerateOTPForApp generates an OTP and records lounge app OTPs in otp_master for admin tracking.
func (s *OTPService) GenerateOTPForApp(phone, ipAddress, userAgent, appName string) (string, error) {
	otp, err := s.GenerateOTP(phone, ipAddress, userAgent)
	if err != nil {
		return "", err
	}

	if isLoungeOTPApp(appName) {
		if err := s.RecordOTPMaster(phone, otp, appName); err != nil {
			return "", err
		}
	}

	return otp, nil
}

// RecordOTPMaster stores lounge OTP tracking data for admin review.
func (s *OTPService) RecordOTPMaster(phone, otp, appName string) error {
	if !isLoungeOTPApp(appName) {
		return nil
	}

	otpValue, err := strconv.ParseInt(otp, 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse otp value: %w", err)
	}

	query := `
		INSERT INTO otp_master (otp, phone, app_name)
		VALUES ($1, $2, $3)
	`

	_, err = s.db.Exec(query, otpValue, phone, normalizeAppName(appName))
	if err != nil {
		return fmt.Errorf("failed to store lounge otp: %w", err)
	}

	return nil
}

// ListOTPMasterRecords returns lounge OTP tracking records, optionally filtered by phone and app name.
func (s *OTPService) ListOTPMasterRecords(phone, appName string, limit int) ([]models.OTPMasterRecord, error) {
	if limit <= 0 {
		limit = 100
	}

	query := `SELECT id, otp, phone, app_name FROM otp_master`
	args := make([]interface{}, 0, 3)
	conditions := make([]string, 0, 2)
	argIndex := 1

	if phone != "" {
		conditions = append(conditions, fmt.Sprintf("phone = $%d", argIndex))
		args = append(args, phone)
		argIndex++
	}

	if appName != "" {
		conditions = append(conditions, fmt.Sprintf("app_name = $%d", argIndex))
		args = append(args, normalizeAppName(appName))
		argIndex++
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	query += fmt.Sprintf(" ORDER BY phone ASC, app_name ASC, id ASC LIMIT $%d", argIndex)
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query otp master records: %w", err)
	}
	defer rows.Close()

	records := make([]models.OTPMasterRecord, 0)
	for rows.Next() {
		var record models.OTPMasterRecord
		var phone, appName sql.NullString
		if err := rows.Scan(&record.ID, &record.OTP, &phone, &appName); err != nil {
			return nil, fmt.Errorf("failed to scan otp master record: %w", err)
		}
		if phone.Valid {
			record.Phone = phone.String
		}
		if appName.Valid {
			record.AppName = appName.String
		}
		records = append(records, record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read otp master records: %w", err)
	}

	return records, nil
}

func isLoungeOTPApp(appName string) bool {
	switch normalizeAppName(appName) {
	case "lounge_owner", "lounge_staff", "lounge":
		return true
	default:
		return false
	}
}

func normalizeAppName(appName string) string {
	return strings.TrimSpace(strings.ToLower(appName))
}

// ValidateOTP validates an OTP for the given phone number
// Returns true if valid, false if invalid, and error if something went wrong
func (s *OTPService) ValidateOTP(phone, otp string) (bool, error) {
	// Get the OTP record
	otpRecord, err := s.getOTPRecord(phone)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, ErrNoOTPFound
		}
		return false, fmt.Errorf("failed to get OTP record: %w", err)
	}

	// Check if already verified
	if otpRecord.Verified {
		return false, ErrOTPAlreadyUsed
	}

	// Check if expired
	if time.Now().After(otpRecord.ExpiresAt) {
		return false, ErrOTPExpired
	}

	// Check if max attempts exceeded
	if otpRecord.Attempts >= MaxOTPAttempts {
		return false, ErrMaxAttemptsExceeded
	}

	// Increment attempts
	if err := s.incrementAttempts(phone); err != nil {
		return false, fmt.Errorf("failed to increment attempts: %w", err)
	}

	// Validate OTP
	if otpRecord.OTPCode != otp {
		return false, ErrOTPInvalid
	}

	// Mark as verified
	if err := s.markAsVerified(phone); err != nil {
		return false, fmt.Errorf("failed to mark OTP as verified: %w", err)
	}

	return true, nil
}

// InvalidateOTP invalidates any existing OTPs for the given phone number
func (s *OTPService) InvalidateOTP(phone string) error {
	query := `
		UPDATE otp_verifications
		SET verified = true
		WHERE phone = $1 AND verified = false
	`

	_, err := s.db.Exec(query, phone)
	if err != nil {
		return fmt.Errorf("failed to invalidate OTP: %w", err)
	}

	return nil
}

// GetRemainingAttempts returns the number of remaining validation attempts
func (s *OTPService) GetRemainingAttempts(phone string) (int, error) {
	otpRecord, err := s.getOTPRecord(phone)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, ErrNoOTPFound
		}
		return 0, fmt.Errorf("failed to get OTP record: %w", err)
	}

	remaining := MaxOTPAttempts - otpRecord.Attempts
	if remaining < 0 {
		remaining = 0
	}

	return remaining, nil
}

// IsOTPExpired checks if the OTP for the given phone number is expired
func (s *OTPService) IsOTPExpired(phone string) (bool, error) {
	otpRecord, err := s.getOTPRecord(phone)
	if err != nil {
		if err == sql.ErrNoRows {
			return true, ErrNoOTPFound
		}
		return true, fmt.Errorf("failed to get OTP record: %w", err)
	}

	return time.Now().After(otpRecord.ExpiresAt), nil
}

// GetOTPExpiry returns the expiry time for the OTP
func (s *OTPService) GetOTPExpiry(phone string) (time.Time, error) {
	otpRecord, err := s.getOTPRecord(phone)
	if err != nil {
		if err == sql.ErrNoRows {
			return time.Time{}, ErrNoOTPFound
		}
		return time.Time{}, fmt.Errorf("failed to get OTP record: %w", err)
	}

	return otpRecord.ExpiresAt, nil
}

// CleanupExpiredOTPs removes all expired OTP records from the database
func (s *OTPService) CleanupExpiredOTPs() (int64, error) {
	query := `
		DELETE FROM otp_verifications
		WHERE expires_at < $1
	`

	result, err := s.db.Exec(query, time.Now())
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup expired OTPs: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return rowsAffected, nil
}

// CleanupOldOTPs removes OTP records older than the specified duration
func (s *OTPService) CleanupOldOTPs(olderThan time.Duration) (int64, error) {
	cutoffTime := time.Now().Add(-olderThan)

	query := `
		DELETE FROM otp_verifications
		WHERE created_at < $1
	`

	result, err := s.db.Exec(query, cutoffTime)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup old OTPs: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return rowsAffected, nil
}

// getOTPRecord retrieves the OTP record for the given phone number
func (s *OTPService) getOTPRecord(phone string) (*models.OTPVerification, error) {
	query := `
		SELECT id, phone, otp_code, purpose, created_at, expires_at, verified, verified_at, attempts, max_attempts, ip_address, user_agent
		FROM otp_verifications
		WHERE phone = $1 AND verified = false
		ORDER BY created_at DESC
		LIMIT 1
	`

	var otp models.OTPVerification
	err := s.db.QueryRow(query, phone).Scan(
		&otp.ID,
		&otp.Phone,
		&otp.OTPCode,
		&otp.Purpose,
		&otp.CreatedAt,
		&otp.ExpiresAt,
		&otp.Verified,
		&otp.VerifiedAt,
		&otp.Attempts,
		&otp.MaxAttempts,
		&otp.IPAddress,
		&otp.UserAgent,
	)

	if err != nil {
		return nil, err
	}

	return &otp, nil
}

// incrementAttempts increments the validation attempts counter
func (s *OTPService) incrementAttempts(phone string) error {
	query := `
		UPDATE otp_verifications
		SET attempts = attempts + 1
		WHERE phone = $1 AND verified = false
	`

	_, err := s.db.Exec(query, phone)
	if err != nil {
		return fmt.Errorf("failed to increment attempts: %w", err)
	}

	return nil
}

// markAsVerified marks the OTP as verified
func (s *OTPService) markAsVerified(phone string) error {
	query := `
		UPDATE otp_verifications
		SET verified = true, verified_at = $1
		WHERE phone = $2 AND verified = false
	`

	_, err := s.db.Exec(query, time.Now(), phone)
	if err != nil {
		return fmt.Errorf("failed to mark OTP as verified: %w", err)
	}

	return nil
}

// generateRandomOTP generates a cryptographically secure random 6-digit OTP
func generateRandomOTP() (string, error) {
	// Generate a random number between 0 and 999999
	max := big.NewInt(1000000) // 10^6
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", err
	}

	// Format as 6-digit string with leading zeros
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// ResendOTP generates a new OTP for the phone number
// This is an alias for GenerateOTP for clarity in API handlers
func (s *OTPService) ResendOTP(phone, ipAddress, userAgent string) (string, error) {
	return s.GenerateOTP(phone, ipAddress, userAgent)
}

// VerifyAndInvalidate validates the OTP and immediately invalidates it
// This is a convenience method for one-time use OTPs
func (s *OTPService) VerifyAndInvalidate(phone, otp string) (bool, error) {
	valid, err := s.ValidateOTP(phone, otp)
	if err != nil {
		return false, err
	}

	if !valid {
		return false, nil
	}

	// OTP is already marked as verified in ValidateOTP
	return true, nil
}

// GetOTPStats returns statistics about OTP usage
func (s *OTPService) GetOTPStats(phone string) (map[string]interface{}, error) {
	otpRecord, err := s.getOTPRecord(phone)
	if err != nil {
		if err == sql.ErrNoRows {
			return map[string]interface{}{
				"has_active_otp": false,
			}, nil
		}
		return nil, fmt.Errorf("failed to get OTP record: %w", err)
	}

	remaining := MaxOTPAttempts - otpRecord.Attempts
	if remaining < 0 {
		remaining = 0
	}

	timeUntilExpiry := time.Until(otpRecord.ExpiresAt)
	if timeUntilExpiry < 0 {
		timeUntilExpiry = 0
	}

	return map[string]interface{}{
		"has_active_otp":       true,
		"attempts_made":        otpRecord.Attempts,
		"attempts_remaining":   remaining,
		"is_expired":           time.Now().After(otpRecord.ExpiresAt),
		"expires_at":           otpRecord.ExpiresAt,
		"time_until_expiry":    timeUntilExpiry.Seconds(),
		"created_at":           otpRecord.CreatedAt,
		"max_attempts_allowed": MaxOTPAttempts,
	}, nil
}
