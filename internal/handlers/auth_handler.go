package handlers

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid" //NEW ADDITION FOR VerifyOTPLoungeStaff handler
	"github.com/smarttransit/sms-auth-backend/internal/config"
	"github.com/smarttransit/sms-auth-backend/internal/database"
	"github.com/smarttransit/sms-auth-backend/internal/middleware"
	"github.com/smarttransit/sms-auth-backend/internal/models"
	"github.com/smarttransit/sms-auth-backend/internal/services"
	"github.com/smarttransit/sms-auth-backend/internal/utils"
	"github.com/smarttransit/sms-auth-backend/pkg/jwt"
	"github.com/smarttransit/sms-auth-backend/pkg/sms"
	"github.com/smarttransit/sms-auth-backend/pkg/validator"
)

// AuthHandler handles authentication-related HTTP requests
type AuthHandler struct {
	jwtService             *jwt.Service
	otpService             *services.OTPService
	phoneValidator         *validator.PhoneValidator
	rateLimitService       *services.RateLimitService
	auditService           *services.AuditService
	notificationService    *services.NotificationService
	userRepository         *database.UserRepository
	passengerRepository    *database.PassengerRepository
	refreshTokenRepository *database.RefreshTokenRepository
	userSessionRepository  *database.UserSessionRepository
	smsGateway             sms.SMSGateway
	config                 *config.Config
	loungeRepository       *database.LoungeRepository
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(
	jwtService *jwt.Service,
	otpService *services.OTPService,
	phoneValidator *validator.PhoneValidator,
	rateLimitService *services.RateLimitService,
	auditService *services.AuditService,
	notificationService *services.NotificationService,
	userRepository *database.UserRepository,
	passengerRepository *database.PassengerRepository,
	refreshTokenRepository *database.RefreshTokenRepository,
	userSessionRepository *database.UserSessionRepository,
	smsGateway sms.SMSGateway,
	cfg *config.Config,
	loungeRepository *database.LoungeRepository,
) *AuthHandler {
	return &AuthHandler{
		jwtService:             jwtService,
		otpService:             otpService,
		phoneValidator:         phoneValidator,
		rateLimitService:       rateLimitService,
		auditService:           auditService,
		notificationService:    notificationService,
		userRepository:         userRepository,
		passengerRepository:    passengerRepository,
		refreshTokenRepository: refreshTokenRepository,
		userSessionRepository:  userSessionRepository,
		smsGateway:             smsGateway,
		config:                 cfg,
		loungeRepository:       loungeRepository,
	}
}

// SendOTPRequest represents the request to send OTP
type SendOTPRequest struct {
	Phone   string `json:"phone_number" binding:"required"`
	AppType string `json:"app_type"` // "passenger", "driver", "conductor", "lounge_owner"
}

// SendOTPResponse represents the response after sending OTP
type SendOTPResponse struct {
	Message   string    `json:"message"`
	Phone     string    `json:"phone"`
	ExpiresAt time.Time `json:"expires_at"`
	ExpiresIn int       `json:"expires_in_seconds"`
}

// RecordOTPMasterRequest represents a manual OTP master record request for admin testing.
type RecordOTPMasterRequest struct {
	Phone   string `json:"phone_number" binding:"required"`
	OTP     string `json:"otp" binding:"required"`
	AppName string `json:"app_name" binding:"required"`
}

func isLoungeOTPApp(appName string) bool {
	switch strings.TrimSpace(strings.ToLower(appName)) {
	case "lounge_owner", "lounge_staff", "lounge":
		return true
	default:
		return false
	}
}

// VerifyOTPRequest represents the request to verify OTP
type VerifyOTPRequest struct {
	Phone string `json:"phone_number" binding:"required"`
	OTP   string `json:"otp" binding:"required"`
}

// VerifyOTPLoungeStaffRegisteredRequest represents the request to verify OTP for registered lounge staff
type VerifyOTPLoungeStaffRegisteredRequest struct {
	Phone string `json:"phone_number" binding:"required"`
	OTP   string `json:"otp" binding:"required"`
}

// VerifyOTPLoungeStaffRegisteredResponse represents the response for lounge staff OTP login
type VerifyOTPLoungeStaffRegisteredResponse struct {
	Message          string                 `json:"message"`
	AccessToken      string                 `json:"access_token"`
	RefreshToken     string                 `json:"refresh_token"`
	ExpiresIn        int                    `json:"expires_in_seconds"`
	IsNewUser        bool                   `json:"is_new_user"`
	ProfileComplete  bool                   `json:"profile_complete"`
	Roles            []string               `json:"roles"`
	ApprovalStatus   string                 `json:"approval_status"`
	EmploymentStatus string                 `json:"employment_status"`
	Staff            map[string]interface{} `json:"staff"`
}

// NEW Added struct explicitly for loungeStaff OTP verification and handling
type VerifyOTPLoungeStaffRequest struct {
	// OTP Verification
	Phone    string `json:"phone_number" binding:"required"`
	OTP      string `json:"otp" binding:"required"`
	LoungeID string `json:"lounge_id" binding:"required"` // Required: UUID of the lounge

	// NEWLY ADDED ROWS OF DATA
	// Lounge Staff table Data
	FullName  string `json:"full_name" binding:"required"`
	NicNumber string `json:"nic_number" binding:"required"`
	Email     string `json:"email" binding:"required"`
}

// VerifyOTPResponse represents the response after verifying OTP
type VerifyOTPResponse struct {
	Message          string   `json:"message"`
	AccessToken      string   `json:"access_token"`
	RefreshToken     string   `json:"refresh_token"`
	ExpiresIn        int      `json:"expires_in_seconds"`
	IsNewUser        bool     `json:"is_new_user"`
	ProfileComplete  bool     `json:"profile_complete"`
	Roles            []string `json:"roles"`                       // User's roles (can be empty for new staff users)
	RegistrationStep string   `json:"registration_step,omitempty"` // For lounge owners: phone_verified, personal_info, nic_uploaded, lounge_added, completed
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

// SendOTP handles POST /api/v1/auth/send-otp
func (h *AuthHandler) SendOTP(c *gin.Context) {
	var req SendOTPRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "validation_error",
			Message: "Invalid request body",
		})
		return
	}

	// Validate phone number
	phone, err := h.phoneValidator.Validate(req.Phone)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_phone",
			Message: err.Error(),
		})
		return
	}

	// Get real client IP and user agent
	clientIP := utils.GetRealIP(c)
	userAgent := utils.GetUserAgent(c)

	// Check rate limiting
	if err := h.rateLimitService.CheckOTPRateLimit(phone, clientIP); err != nil {
		if rateLimitErr, ok := err.(*services.RateLimitError); ok {
			// Log rate limit violation
			h.auditService.LogRateLimitViolation(phone, clientIP, userAgent, rateLimitErr.Type, rateLimitErr.RetryAfter)

			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "rate_limit_exceeded",
				"message":     rateLimitErr.Message,
				"retry_after": rateLimitErr.RetryAfter,
				"type":        rateLimitErr.Type,
			})
			return
		}
		// Other errors
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "rate_limit_check_failed",
			Message: "Failed to check rate limit",
		})
		return
	}

	// Generate OTP with IP and user agent tracking
	otp, err := h.otpService.GenerateOTPForApp(phone, clientIP, userAgent, req.AppType)
	if err != nil {
		// Log failed OTP request
		h.auditService.LogOTPRequest(phone, clientIP, userAgent, false, "generation_failed")

		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "otp_generation_failed",
			Message: "Failed to generate OTP",
		})
		return
	}

	// Record rate limit request
	if err := h.rateLimitService.RecordOTPRequest(phone, clientIP); err != nil {
		// Log error but don't fail the request
		// The OTP is already generated and stored
		c.Error(err) // This logs the error in Gin
	}

	// Log successful OTP request
	h.auditService.LogOTPRequest(phone, clientIP, userAgent, true, "")

	// Get expiry time
	expiresAt, _ := h.otpService.GetOTPExpiry(phone)
	expiresIn := int(time.Until(expiresAt).Seconds())

	// Check SMS configuration before attempting to send
	if h.config.SMS.Mode == "production" {
		// Validate SMS configuration
		if h.config.SMS.Method == "url" && h.config.SMS.ESMSQK == "" {
			log.Printf("❌ ERROR: SMS API key (DIALOG_SMS_ESMSQK) is not configured")
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "sms_not_configured",
				"message": "SMS gateway is not properly configured. Please contact support.",
				"details": "Dialog API key not set",
			})
			return
		}

		if h.config.SMS.Mask == "" {
			log.Printf("❌ ERROR: SMS Mask (DIALOG_SMS_MASK) is not configured")
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "sms_not_configured",
				"message": "SMS gateway is not properly configured. Please contact support.",
				"details": "SMS Mask not set",
			})
			return
		}
	}

	// Send SMS based on mode
	if h.config.SMS.Mode == "production" {
		// Production mode: Send actual SMS via Dialog gateway
		log.Printf("🔵 Attempting to send SMS to %s via Dialog gateway (App: %s)...", phone, req.AppType)
		log.Printf("📝 SMS Method: %s", h.config.SMS.Method)
		if h.config.SMS.Method == "url" {
			log.Printf("📝 Using API Key: %s****", h.config.SMS.ESMSQK[:3])
		}
		log.Printf("📝 SMS Mask: %s", h.config.SMS.Mask)

		transactionID, err := h.smsGateway.SendOTP(phone, otp, req.AppType)
		if err != nil {
			log.Printf("❌ ERROR: Failed to send SMS to %s: %v", phone, err)
			log.Printf("❌ Error type: %T", err)
			log.Printf("❌ Full error details: %+v", err)
			errorMsg := fmt.Sprintf("Failed to send OTP: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "sms_send_failed",
				"message": "Failed to send OTP via SMS. Please try again.",
				"details": errorMsg,
			})
			return
		}

		log.Printf("✅ SMS sent successfully to %s, transaction_id: %d", phone, transactionID)

		// Production response (without OTP)
		c.JSON(http.StatusOK, gin.H{
			"message":    "OTP sent successfully to your phone",
			"phone":      phone,
			"expires_at": expiresAt,
			"expires_in": expiresIn,
			"mode":       "production",
		})
		return
	}

	// Development mode: Return OTP in response (no actual SMS sent)
	log.Printf("🧪 DEV MODE OTP | phone=%s | otp=%s | app=%s", phone, otp, req.AppType)
	c.JSON(http.StatusOK, gin.H{
		"message":    "OTP generated successfully (dev mode - no SMS sent)",
		"phone":      phone,
		"expires_at": expiresAt,
		"expires_in": expiresIn,
		"otp":        otp, // Only in development mode
		"mode":       "development",
	})
}

// RecordOTPMaster handles POST /api/v1/admin/otp-master
func (h *AuthHandler) RecordOTPMaster(c *gin.Context) {
	var req RecordOTPMasterRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "validation_error", Message: "Invalid request body"})
		return
	}

	phone, err := h.phoneValidator.Validate(req.Phone)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid_phone", Message: err.Error()})
		return
	}

	appName := strings.TrimSpace(req.AppName)
	if !isLoungeOTPApp(appName) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid_app_name", Message: "Only lounge OTP app names can be recorded"})
		return
	}

	if _, err := strconv.ParseInt(req.OTP, 10, 64); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid_otp", Message: "OTP must be numeric"})
		return
	}

	if err := h.otpService.RecordOTPMaster(phone, req.OTP, appName); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "record_failed", Message: "Failed to store OTP master record"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "OTP master record stored successfully",
		"phone":    phone,
		"otp":      req.OTP,
		"app_name": strings.ToLower(appName),
	})
}

// GetOTPMasterRecords handles GET /api/v1/admin/otp-master
func (h *AuthHandler) GetOTPMasterRecords(c *gin.Context) {
	phoneQuery := strings.TrimSpace(c.Query("phone_number"))
	if phoneQuery == "" {
		phoneQuery = strings.TrimSpace(c.Query("phone"))
	}

	phone := ""
	if phoneQuery != "" {
		validatedPhone, err := h.phoneValidator.Validate(phoneQuery)
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid_phone", Message: err.Error()})
			return
		}
		phone = validatedPhone
	}

	appName := strings.TrimSpace(c.Query("app_name"))
	if appName != "" && !isLoungeOTPApp(appName) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid_app_name", Message: "Only lounge OTP app names are supported"})
		return
	}

	limit := 100
	if limitValue := strings.TrimSpace(c.Query("limit")); limitValue != "" {
		parsedLimit, err := strconv.Atoi(limitValue)
		if err != nil || parsedLimit <= 0 {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid_limit", Message: "Limit must be a positive number"})
			return
		}
		if parsedLimit > 500 {
			parsedLimit = 500
		}
		limit = parsedLimit
	}

	records, err := h.otpService.ListOTPMasterRecords(phone, appName, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "query_failed", Message: "Failed to read OTP master records"})
		return
	}

	responseRecords := make([]gin.H, 0, len(records))
	for _, record := range records {
		responseRecords = append(responseRecords, gin.H{
			"id":       record.ID,
			"otp":      fmt.Sprintf("%06d", record.OTP),
			"phone":    record.Phone,
			"app_name": record.AppName,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"records": responseRecords,
		"total":   len(responseRecords),
		"limit":   limit,
	})
}

// VerifyOTP handles POST /api/v1/auth/verify-otp
func (h *AuthHandler) VerifyOTP(c *gin.Context) {
	var req VerifyOTPRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "validation_error",
			Message: "Invalid request body",
		})
		return
	}

	// Validate phone number
	phone, err := h.phoneValidator.Validate(req.Phone)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_phone",
			Message: err.Error(),
		})
		return
	}

	// Get real client IP and user agent
	clientIP := utils.GetRealIP(c)
	userAgent := utils.GetUserAgent(c)

	// Get current attempts before validation
	remainingBefore, _ := h.otpService.GetRemainingAttempts(phone)

	// Validate OTP
	valid, err := h.otpService.ValidateOTP(phone, req.OTP)
	if err != nil {
		// Log failed verification
		attempts := 3 - remainingBefore + 1 // Calculated attempts made
		h.auditService.LogOTPVerification(nil, phone, false, attempts, clientIP, userAgent, err.Error())

		// Check specific error types
		switch err {
		case services.ErrOTPExpired:
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "otp_expired",
				Message: "OTP has expired. Please request a new one.",
				Code:    "OTP_EXPIRED",
			})
		case services.ErrOTPInvalid:
			remaining, _ := h.otpService.GetRemainingAttempts(phone)
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "otp_invalid",
				Message: "Invalid OTP code",
				Code:    "OTP_INVALID",
			})
			c.Header("X-Remaining-Attempts", string(rune(remaining)))
		case services.ErrMaxAttemptsExceeded:
			c.JSON(http.StatusTooManyRequests, ErrorResponse{
				Error:   "max_attempts_exceeded",
				Message: "Maximum OTP validation attempts exceeded. Please request a new OTP.",
				Code:    "MAX_ATTEMPTS",
			})
		case services.ErrNoOTPFound:
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error:   "no_otp_found",
				Message: "No OTP found for this phone number. Please request an OTP first.",
				Code:    "NO_OTP",
			})
		case services.ErrOTPAlreadyUsed:
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "otp_already_used",
				Message: "This OTP has already been used. Please request a new one.",
				Code:    "OTP_USED",
			})
		default:
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error:   "validation_failed",
				Message: "Failed to validate OTP",
			})
		}
		return
	}

	if !valid {
		// Log invalid OTP
		attempts := 3 - remainingBefore + 1
		h.auditService.LogOTPVerification(nil, phone, false, attempts, clientIP, userAgent, "invalid_code")

		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "otp_invalid",
			Message: "Invalid OTP code",
		})
		return
	}

	// Get or create user
	user, isNew, err := h.userRepository.GetOrCreateUser(phone)
	if err != nil {
		log.Printf("ERROR: Failed to get or create user for phone %s: %v", phone, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "user_creation_failed",
			Message: "Failed to get or create user",
		})
		return
	}

	// For users with passenger role, ensure passenger record exists
	// This creates the passenger profile record in the passengers table
	if h.userRepository.HasRole(user, "passenger") {
		_, _, err := h.passengerRepository.GetOrCreatePassenger(user.ID)
		if err != nil {
			log.Printf("WARNING: Failed to create passenger record for user %s: %v", user.ID, err)
			// Don't fail login, just log warning
		}
	}

	// Get passenger profile completion status (if passenger role)
	profileCompleted := false
	if h.userRepository.HasRole(user, "passenger") {
		profileCompleted, _ = h.passengerRepository.IsPassengerProfileComplete(user.ID)
	}

	// Generate JWT tokens with user's actual data (use passenger profile completion status)
	accessToken, err := h.jwtService.GenerateAccessToken(
		user.ID,
		user.Phone,
		user.Roles,
		profileCompleted, // Use passenger table's profile_completed
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "token_generation_failed",
			Message: "Failed to generate access token",
		})
		return
	}

	refreshToken, err := h.jwtService.GenerateRefreshToken(user.ID, user.Phone)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "token_generation_failed",
			Message: "Failed to generate refresh token",
		})
		return
	}

	// Store refresh token in database
	expiresAt := time.Now().Add(7 * 24 * time.Hour) // 7 days

	// Get device info from request if provided
	deviceID := c.GetHeader("X-Device-ID")
	deviceType := c.GetHeader("X-Device-Type")

	err = h.refreshTokenRepository.StoreRefreshToken(
		user.ID,
		refreshToken,
		deviceID,
		deviceType,
		clientIP,
		userAgent,
		expiresAt,
	)
	if err != nil {
		// Log error but don't fail the login
		// In production, you'd want proper logging here
	}

	// Log successful OTP verification and login
	h.auditService.LogOTPVerification(&user.ID, phone, true, 3-remainingBefore+1, clientIP, userAgent, "")
	h.auditService.LogLogin(user.ID, phone, clientIP, userAgent, deviceID, deviceType)

	// Create or update user session
	deviceModel := c.GetHeader("X-Device-Model")
	appVersion := c.GetHeader("X-App-Version")
	osVersion := c.GetHeader("X-OS-Version")
	fcmToken := c.GetHeader("X-FCM-Token")

	if deviceID != "" && deviceType != "" {
		_, err = h.userSessionRepository.CreateOrUpdateSession(
			user.ID,
			deviceID,
			deviceType,
			deviceModel,
			appVersion,
			osVersion,
			clientIP,
			fcmToken,
		)
		if err != nil {
			// Log error but don't fail the login
			log.Printf("WARNING: Failed to create/update session for user %s: %v", user.ID, err)
		}
	}

	c.JSON(http.StatusOK, VerifyOTPResponse{
		Message:         "OTP verified successfully",
		AccessToken:     accessToken,
		RefreshToken:    refreshToken,
		ExpiresIn:       3600, // 1 hour
		IsNewUser:       isNew,
		ProfileComplete: profileCompleted, // Use passenger table's profile_completed
		Roles:           user.Roles,       // Include user roles in response
	})
}

// VerifyOTPStaff handles POST /api/v1/auth/verify-otp-staff
// This endpoint is specifically for staff app authentication
// It creates users WITHOUT assigning any role initially
func (h *AuthHandler) VerifyOTPStaff(c *gin.Context) {
	var req VerifyOTPRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "validation_error",
			Message: "Invalid request body",
		})
		return
	}

	// Validate phone number
	phone, err := h.phoneValidator.Validate(req.Phone)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_phone",
			Message: err.Error(),
		})
		return
	}

	// Get real client IP and user agent
	clientIP := utils.GetRealIP(c)
	userAgent := utils.GetUserAgent(c)

	// Get current attempts before validation
	remainingBefore, _ := h.otpService.GetRemainingAttempts(phone)

	// Validate OTP
	valid, err := h.otpService.ValidateOTP(phone, req.OTP)
	if err != nil {
		// Log failed verification
		attempts := 3 - remainingBefore + 1
		h.auditService.LogOTPVerification(nil, phone, false, attempts, clientIP, userAgent, err.Error())

		// Check specific error types
		switch err {
		case services.ErrOTPExpired:
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "otp_expired",
				Message: "OTP has expired. Please request a new one.",
				Code:    "OTP_EXPIRED",
			})
		case services.ErrOTPInvalid:
			remaining, _ := h.otpService.GetRemainingAttempts(phone)
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "otp_invalid",
				Message: "Invalid OTP code",
				Code:    "OTP_INVALID",
			})
			c.Header("X-Remaining-Attempts", string(rune(remaining)))
		case services.ErrMaxAttemptsExceeded:
			c.JSON(http.StatusTooManyRequests, ErrorResponse{
				Error:   "max_attempts_exceeded",
				Message: "Maximum OTP validation attempts exceeded. Please request a new OTP.",
				Code:    "MAX_ATTEMPTS",
			})
		case services.ErrNoOTPFound:
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error:   "no_otp_found",
				Message: "No OTP found for this phone number. Please request an OTP first.",
				Code:    "NO_OTP",
			})
		case services.ErrOTPAlreadyUsed:
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "otp_already_used",
				Message: "This OTP has already been used. Please request a new one.",
				Code:    "OTP_USED",
			})
		default:
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error:   "validation_failed",
				Message: "Failed to validate OTP",
			})
		}
		return
	}

	if !valid {
		// Log invalid OTP
		attempts := 3 - remainingBefore + 1
		h.auditService.LogOTPVerification(nil, phone, false, attempts, clientIP, userAgent, "invalid_code")

		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "otp_invalid",
			Message: "Invalid OTP code",
		})
		return
	}

	// Check if user already exists
	existingUser, err := h.userRepository.GetUserByPhone(phone)
	if err != nil {
		log.Printf("ERROR: Failed to check existing user for phone %s: %v", phone, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "user_check_failed",
			Message: "Failed to check user status",
		})
		return
	}

	var user *models.User
	isNew := false

	if existingUser != nil {
		// EXISTING USER - Allow them to use staff app regardless of current roles
		user = existingUser

		// Multi-role system: Users can be BOTH passenger AND driver/conductor
		// If they only have 'passenger' role → They'll select staff role (driver/conductor) next
		// If they have driver/conductor role → They go directly to dashboard
		// No blocking based on existing roles!

		log.Printf("INFO: Existing user logged in to staff app: %s (roles: %v)", phone, user.Roles)
	} else {
		// NEW USER - Create without role
		user, err = h.userRepository.CreateUserWithoutRole(phone)
		if err != nil {
			log.Printf("ERROR: Failed to create staff user for phone %s: %v", phone, err)
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error:   "user_creation_failed",
				Message: "Failed to create user account",
			})
			return
		}
		isNew = true
		log.Printf("INFO: New staff user created: %s (no role assigned yet)", phone)
	}

	// Generate JWT tokens with user's actual data
	accessToken, err := h.jwtService.GenerateAccessToken(
		user.ID,
		user.Phone,
		user.Roles,
		user.ProfileCompleted,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "token_generation_failed",
			Message: "Failed to generate access token",
		})
		return
	}

	refreshToken, err := h.jwtService.GenerateRefreshToken(user.ID, user.Phone)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "token_generation_failed",
			Message: "Failed to generate refresh token",
		})
		return
	}

	// Store refresh token in database
	expiresAt := time.Now().Add(7 * 24 * time.Hour) // 7 days

	// Get device info from request if provided
	deviceID := c.GetHeader("X-Device-ID")
	deviceType := c.GetHeader("X-Device-Type")

	err = h.refreshTokenRepository.StoreRefreshToken(
		user.ID,
		refreshToken,
		deviceID,
		deviceType,
		clientIP,
		userAgent,
		expiresAt,
	)
	if err != nil {
		// Log error but don't fail the login
		log.Printf("WARNING: Failed to store refresh token for user %s: %v", user.ID, err)
	}

	// Log successful OTP verification and login (staff app)
	h.auditService.LogOTPVerification(&user.ID, phone, true, 3-remainingBefore+1, clientIP, userAgent, "")
	h.auditService.LogLogin(user.ID, phone, clientIP, userAgent, deviceID, deviceType)

	// Create or update user session
	deviceModel := c.GetHeader("X-Device-Model")
	appVersion := c.GetHeader("X-App-Version")
	osVersion := c.GetHeader("X-OS-Version")
	fcmToken := c.GetHeader("X-FCM-Token")

	if deviceID != "" && deviceType != "" {
		_, err = h.userSessionRepository.CreateOrUpdateSession(
			user.ID,
			deviceID,
			deviceType,
			deviceModel,
			appVersion,
			osVersion,
			clientIP,
			fcmToken,
		)
		if err != nil {
			// Log error but don't fail the login
			log.Printf("WARNING: Failed to create/update session for user %s: %v", user.ID, err)
		}
	}

	c.JSON(http.StatusOK, VerifyOTPResponse{
		Message:         "OTP verified successfully",
		AccessToken:     accessToken,
		RefreshToken:    refreshToken,
		ExpiresIn:       3600, // 1 hour
		IsNewUser:       isNew,
		ProfileComplete: user.ProfileCompleted,
		Roles:           user.Roles, // Include user roles - empty [] for new users, ["driver"]/["conductor"] for existing staff
	})
}

// VerifyOTPLoungeOwner handles POST /api/v1/auth/verify-otp-lounge-owner
// This endpoint is specifically for lounge owner app authentication
// It creates users and adds 'lounge_owner' role immediately
func (h *AuthHandler) VerifyOTPLoungeOwner(c *gin.Context, loungeOwnerRepo *database.LoungeOwnerRepository) {
	var req VerifyOTPRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "validation_error",
			Message: "Invalid request body",
		})
		return
	}

	// Validate phone number
	phone, err := h.phoneValidator.Validate(req.Phone)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_phone",
			Message: err.Error(),
		})
		return
	}

	// Get real client IP and user agent
	clientIP := utils.GetRealIP(c)
	userAgent := utils.GetUserAgent(c)

	// Get current attempts before validation
	remainingBefore, _ := h.otpService.GetRemainingAttempts(phone)

	// Validate OTP
	valid, err := h.otpService.ValidateOTP(phone, req.OTP)
	if err != nil {
		// Log failed verification
		attempts := 3 - remainingBefore + 1
		h.auditService.LogOTPVerification(nil, phone, false, attempts, clientIP, userAgent, err.Error())

		// Check specific error types
		switch err {
		case services.ErrOTPExpired:
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "otp_expired",
				Message: "OTP has expired. Please request a new one.",
				Code:    "OTP_EXPIRED",
			})
		case services.ErrOTPInvalid:
			remaining, _ := h.otpService.GetRemainingAttempts(phone)
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "otp_invalid",
				Message: "Invalid OTP code",
				Code:    "OTP_INVALID",
			})
			c.Header("X-Remaining-Attempts", string(rune(remaining)))
		case services.ErrMaxAttemptsExceeded:
			c.JSON(http.StatusTooManyRequests, ErrorResponse{
				Error:   "max_attempts_exceeded",
				Message: "Maximum OTP validation attempts exceeded. Please request a new OTP.",
				Code:    "MAX_ATTEMPTS",
			})
		case services.ErrNoOTPFound:
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error:   "no_otp_found",
				Message: "No OTP found for this phone number. Please request an OTP first.",
				Code:    "NO_OTP",
			})
		case services.ErrOTPAlreadyUsed:
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "otp_already_used",
				Message: "This OTP has already been used. Please request a new one.",
				Code:    "OTP_USED",
			})
		default:
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error:   "validation_failed",
				Message: "Failed to validate OTP",
			})
		}
		return
	}

	if !valid {
		// Log invalid OTP
		attempts := 3 - remainingBefore + 1
		h.auditService.LogOTPVerification(nil, phone, false, attempts, clientIP, userAgent, "invalid_code")

		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "otp_invalid",
			Message: "Invalid OTP code",
		})
		return
	}

	// Check if user already exists
	existingUser, err := h.userRepository.GetUserByPhone(phone)
	if err != nil {
		log.Printf("ERROR: Failed to check existing user for phone %s: %v", phone, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "user_check_failed",
			Message: "Failed to check user status",
		})
		return
	}

	var user *models.User
	isNew := false

	if existingUser != nil {
		// EXISTING USER
		user = existingUser

		// Add 'lounge_owner' role if not already present
		hasLoungeOwnerRole := false
		for _, role := range user.Roles {
			if role == "lounge_owner" {
				hasLoungeOwnerRole = true
				break
			}
		}

		if !hasLoungeOwnerRole {
			// Add lounge_owner role
			err = h.userRepository.AddRole(user.ID, "lounge_owner")
			if err != nil {
				log.Printf("ERROR: Failed to add lounge_owner role to user %s: %v", user.ID, err)
				// Continue anyway
			} else {
				user.Roles = append(user.Roles, "lounge_owner")
				log.Printf("INFO: Added lounge_owner role to existing user: %s", phone)
			}
		}

		log.Printf("INFO: Existing user logged in to lounge owner app: %s (roles: %v)", phone, user.Roles)
	} else {
		// NEW USER - Create with lounge_owner role immediately
		// This app is exclusively for lounge owners
		user, err = h.userRepository.CreateUserWithRole(phone, "lounge_owner")
		if err != nil {
			log.Printf("ERROR: Failed to create lounge owner user for phone %s: %v", phone, err)
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error:   "user_creation_failed",
				Message: "Failed to create user account",
			})
			return
		}
		isNew = true
		log.Printf("INFO: New lounge owner user created: %s", phone)
	}

	// Create lounge_owner record if doesn't exist
	var registrationStep string = ""
	existingOwner, err := loungeOwnerRepo.GetLoungeOwnerByUserID(user.ID)
	if err != nil {
		log.Printf("ERROR: Failed to check lounge owner record: %v", err)
		// Continue anyway
	}

	if existingOwner == nil {
		// Create lounge owner record
		newOwner, err := loungeOwnerRepo.CreateLoungeOwner(user.ID)
		if err != nil {
			log.Printf("ERROR: Failed to create lounge owner record for user %s: %v", user.ID, err)
			// Don't fail login, but log error
		} else {
			log.Printf("INFO: Created lounge_owner record for user %s", user.ID)
			registrationStep = string(newOwner.RegistrationStep) // Convert ENUM to string
		}
	} else {
		registrationStep = string(existingOwner.RegistrationStep) // Convert ENUM to string
	}

	// Generate JWT tokens with user's actual data
	accessToken, err := h.jwtService.GenerateAccessToken(
		user.ID,
		user.Phone,
		user.Roles,
		user.ProfileCompleted,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "token_generation_failed",
			Message: "Failed to generate access token",
		})
		return
	}

	refreshToken, err := h.jwtService.GenerateRefreshToken(user.ID, user.Phone)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "token_generation_failed",
			Message: "Failed to generate refresh token",
		})
		return
	}

	// Store refresh token in database
	expiresAt := time.Now().Add(7 * 24 * time.Hour) // 7 days

	// Get device info from request if provided
	deviceID := c.GetHeader("X-Device-ID")
	deviceType := c.GetHeader("X-Device-Type")

	err = h.refreshTokenRepository.StoreRefreshToken(
		user.ID,
		refreshToken,
		deviceID,
		deviceType,
		clientIP,
		userAgent,
		expiresAt,
	)
	if err != nil {
		// Log error but don't fail the login
		log.Printf("WARNING: Failed to store refresh token for user %s: %v", user.ID, err)
	}

	// Log successful OTP verification and login (lounge owner app)
	h.auditService.LogOTPVerification(&user.ID, phone, true, 3-remainingBefore+1, clientIP, userAgent, "")
	h.auditService.LogLogin(user.ID, phone, clientIP, userAgent, deviceID, deviceType)

	// Create or update user session
	deviceModel := c.GetHeader("X-Device-Model")
	appVersion := c.GetHeader("X-App-Version")
	osVersion := c.GetHeader("X-OS-Version")
	fcmToken := c.GetHeader("X-FCM-Token")

	if deviceID != "" && deviceType != "" {
		_, err = h.userSessionRepository.CreateOrUpdateSession(
			user.ID,
			deviceID,
			deviceType,
			deviceModel,
			appVersion,
			osVersion,
			clientIP,
			fcmToken,
		)
		if err != nil {
			// Log error but don't fail the login
			log.Printf("WARNING: Failed to create/update session for user %s: %v", user.ID, err)
		}
	}

	c.JSON(http.StatusOK, VerifyOTPResponse{
		Message:          "OTP verified successfully",
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		ExpiresIn:        3600, // 1 hour
		IsNewUser:        isNew,
		ProfileComplete:  user.ProfileCompleted,
		Roles:            user.Roles,       // Include user roles including 'lounge_owner'
		RegistrationStep: registrationStep, // Include registration step for navigation
	})
}

// VerifyOTPLoungeStaff handles POST /api/v1/auth/verify-otp-lounge-staff
// It creates users and adds 'lounge_staff' role into it
func (h *AuthHandler) VerifyOTPLoungeStaff(c *gin.Context, loungeStaffRepo *database.LoungeStaffRepository) {
	//STILL IMPLEMENTING
	var req VerifyOTPLoungeStaffRequest

	// binding data into the request (if error return error)
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "validation_error",
			Message: "Invalid request body",
		})
		return
	}

	// Validate phone number
	phone, err := h.phoneValidator.Validate(req.Phone)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_phone",
			Message: err.Error(),
		})
		return
	}

	// Get client info
	clientIP := utils.GetRealIP(c)
	userAgent := utils.GetUserAgent(c)

	// Get current attempts before validation
	remainingBefore, _ := h.otpService.GetRemainingAttempts(phone)

	// Validate OTP
	valid, err := h.otpService.ValidateOTP(phone, req.OTP)
	if err != nil {
		// Log failed verification
		attempts := 3 - remainingBefore + 1
		h.auditService.LogOTPVerification(nil, phone, false, attempts, clientIP, userAgent, err.Error())

		// Check specific error types with DETAILED ERROR HANDLING
		switch err {
		case services.ErrOTPExpired:
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "otp_expired",
				Message: "OTP has expired. Please request a new one.",
				Code:    "OTP_EXPIRED",
			})
		case services.ErrOTPInvalid:
			remaining, _ := h.otpService.GetRemainingAttempts(phone)
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "otp_invalid",
				Message: "Invalid OTP code",
				Code:    "OTP_INVALID",
			})
			c.Header("X-Remaining-Attempts", string(rune(remaining)))
		case services.ErrMaxAttemptsExceeded:
			c.JSON(http.StatusTooManyRequests, ErrorResponse{
				Error:   "max_attempts_exceeded",
				Message: "Maximum OTP validation attempts exceeded. Please request a new OTP.",
				Code:    "MAX_ATTEMPTS",
			})
		case services.ErrNoOTPFound:
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error:   "no_otp_found",
				Message: "No OTP found for this phone number. Please request an OTP first.",
				Code:    "NO_OTP",
			})
		case services.ErrOTPAlreadyUsed:
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "otp_already_used",
				Message: "This OTP has already been used. Please request a new one.",
				Code:    "OTP_USED",
			})
		default:
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error:   "validation_failed",
				Message: "Failed to validate OTP",
			})
		}
		return
	}

	if !valid {
		// Log invalid OTP
		attempts := 3 - remainingBefore + 1
		h.auditService.LogOTPVerification(nil, phone, false, attempts, clientIP, userAgent, "invalid_code")

		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "otp_invalid",
			Message: "Invalid OTP code",
		})
		return
	}

	// Parse and validate lounge_id
	loungeUUID, err := uuid.Parse(req.LoungeID)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid_lounge_id", Message: "Invalid lounge UUID"})
		return
	}
	// verify lounge exists and approved
	lounge, err := h.loungeRepository.GetLoungeByID(loungeUUID)

	if err != nil {
		log.Printf("ERROR: Failed to verify lounge %s: %v", loungeUUID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to verify lounge",
		})
		return
	}

	if lounge == nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_lounge",
			Message: "Selected lounge does not exist",
		})
		return
	}

	if lounge.Status != models.LoungeStatusApproved {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "lounge_not_approved",
			Message: "Selected lounge is not available for staff registration",
		})
		return
	}

	// Get or create user
	existingUser, err := h.userRepository.GetUserByPhone(phone)
	if err != nil {
		log.Printf("ERROR: Failed to check existing user for phone %s: %v", phone, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "user_check_failed", Message: "Failed to check user status"})
		return
	}

	var user *models.User
	isNew := false

	if existingUser != nil {
		// EXISTING USER - Update with new data
		user = existingUser
		// Add lounge_staff role if not present
		hasRole := false
		for _, r := range user.Roles {
			if r == "lounge_staff" {
				hasRole = true
				break
			}
		}
		if !hasRole {
			if err := h.userRepository.AddRole(user.ID, "lounge_staff"); err != nil {
				log.Printf("ERROR: Failed to add lounge_staff role: %v", err)
				return //added return
			} else {
				user.Roles = append(user.Roles, "lounge_staff")
			}
		}

		// update the user profile data

	} else {

		// NEW USER - Create with basic data only (phone + role)
		// user, err = h.userRepository.CreateUserWithRole(phone, "lounge_staff")
		// if err != nil {
		// 	log.Printf("ERROR: Failed to create lounge staff user for phone %s: %v", phone, err)
		// 	c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "user_creation_failed", Message: "Failed to create user account"})
		// 	return
		// }

		// USER CREATION WITH MORE DETAILS
		user, err = h.userRepository.CreateUserWithFullData(phone, "lounge_staff", req.FullName, req.NicNumber, req.Email)
		if err != nil {
			log.Printf("ERROR: Failed to create lounge staff user for phone %s: %v", phone, err)
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "user_creation_failed", Message: "Failed to create user account"})
			return
		}

		isNew = true
	}

	// HERE WILL BE IMPLEMENTING LoungeStaff Records

	StaffRecord, err := loungeStaffRepo.AddStaffToLoungeWithCompleteData(
		loungeUUID,
		user.ID,
		"active", //changed to active to check db
		req.FullName,
		req.NicNumber,
		req.Email,
	)
	if err != nil {
		log.Printf("ERROR: Failed to create/update lounge staff record: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "staff_record_failed",
			Message: "Failed to create staff record",
		})
		return
	}
	// Send notification to lounge owner (non-blocking)
	go func() {
		err := h.notificationService.NotifyLoungeOwnerNewStaff(
			loungeUUID,
			req.FullName,
			phone,
			req.NicNumber,
		)
		if err != nil {
			log.Printf("WARNING: Failed to send notification to lounge owner: %v", err)
			// Don't fail the request - notification is non-critical
		}
	}()

	// Determine registration step based on lounge_staff record status
	var registrationStep string = "pending"
	if StaffRecord != nil && StaffRecord.EmploymentStatus != "" {
		registrationStep = string(StaffRecord.EmploymentStatus)
	}

	// Generate tokens
	accessToken, err := h.jwtService.GenerateAccessToken(user.ID, user.Phone, user.Roles, user.ProfileCompleted)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "token_generation_failed", Message: "Failed to generate access token"})
		return
	}
	refreshToken, err := h.jwtService.GenerateRefreshToken(user.ID, user.Phone)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "token_generation_failed", Message: "Failed to generate refresh token"})
		return
	}

	// Store refresh token in database
	expiresAt := time.Now().Add(7 * 24 * time.Hour) // 7 days
	deviceID := c.GetHeader("X-Device-ID")
	deviceType := c.GetHeader("X-Device-Type")

	err = h.refreshTokenRepository.StoreRefreshToken(
		user.ID,
		refreshToken,
		deviceID,
		deviceType,
		clientIP,
		userAgent,
		expiresAt,
	)
	if err != nil {
		// Log error but don't fail the login
		log.Printf("WARNING: Failed to store refresh token for user %s: %v", user.ID, err)
	}

	// Audit & session
	h.auditService.LogOTPVerification(&user.ID, phone, true, 3-remainingBefore+1, clientIP, userAgent, "")
	h.auditService.LogLogin(user.ID, phone, clientIP, userAgent, deviceID, deviceType)
	if deviceID != "" && deviceType != "" {
		_, _ = h.userSessionRepository.CreateOrUpdateSession(user.ID, deviceID, deviceType, c.GetHeader("X-Device-Model"), c.GetHeader("X-App-Version"), c.GetHeader("X-OS-Version"), clientIP, c.GetHeader("X-FCM-Token"))
	}

	// Response
	c.JSON(http.StatusOK, VerifyOTPResponse{
		Message:          "OTP verified and lounge staff profile completed successfully",
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		ExpiresIn:        3600,
		IsNewUser:        isNew,
		ProfileComplete:  false, // User profile not completed, only lounge staff profile
		Roles:            user.Roles,
		RegistrationStep: registrationStep,
	})

}

// VerifyOTPLoungeStaffRegistered handles POST /api/v1/auth/verify-otp-lounge-staff-registered
// It verifies OTP for already-registered lounge staff and returns staff profile with approval status.
func (h *AuthHandler) VerifyOTPLoungeStaffRegistered(c *gin.Context, loungeStaffRepo *database.LoungeStaffRepository) {
	var req VerifyOTPLoungeStaffRegisteredRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "validation_error",
			Message: "Invalid request body",
		})
		return
	}

	// Validate phone number
	phone, err := h.phoneValidator.Validate(req.Phone)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_phone",
			Message: err.Error(),
		})
		return
	}

	// Client info
	clientIP := utils.GetRealIP(c)
	userAgent := utils.GetUserAgent(c)

	remainingBefore, _ := h.otpService.GetRemainingAttempts(phone)

	// Validate OTP
	valid, err := h.otpService.ValidateOTP(phone, req.OTP)
	if err != nil {
		attempts := 3 - remainingBefore + 1
		h.auditService.LogOTPVerification(nil, phone, false, attempts, clientIP, userAgent, err.Error())

		switch err {
		case services.ErrOTPExpired:
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "otp_expired",
				Message: "OTP has expired. Please request a new one.",
				Code:    "OTP_EXPIRED",
			})
		case services.ErrOTPInvalid:
			remaining, _ := h.otpService.GetRemainingAttempts(phone)
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "otp_invalid",
				Message: "Invalid OTP code",
				Code:    "OTP_INVALID",
			})
			c.Header("X-Remaining-Attempts", string(rune(remaining)))
		case services.ErrMaxAttemptsExceeded:
			c.JSON(http.StatusTooManyRequests, ErrorResponse{
				Error:   "max_attempts_exceeded",
				Message: "Maximum OTP validation attempts exceeded. Please request a new OTP.",
				Code:    "MAX_ATTEMPTS",
			})
		case services.ErrNoOTPFound:
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error:   "no_otp_found",
				Message: "No OTP found for this phone number. Please request an OTP first.",
				Code:    "NO_OTP",
			})
		case services.ErrOTPAlreadyUsed:
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "otp_already_used",
				Message: "This OTP has already been used. Please request a new one.",
				Code:    "OTP_USED",
			})
		default:
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error:   "validation_failed",
				Message: "Failed to validate OTP",
			})
		}
		return
	}

	if !valid {
		attempts := 3 - remainingBefore + 1
		h.auditService.LogOTPVerification(nil, phone, false, attempts, clientIP, userAgent, "invalid_code")

		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "otp_invalid",
			Message: "Invalid OTP code",
		})
		return
	}

	// User must already exist
	user, err := h.userRepository.GetUserByPhone(phone)
	if err != nil {
		log.Printf("ERROR: Failed to check existing user for phone %s: %v", phone, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "user_check_failed",
			Message: "Failed to check user status",
		})
		return
	}

	if user == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "user_not_found",
			Message: "User not found for this phone number",
		})
		return
	}

	// Fetch lounge staff record for this user
	staff, err := loungeStaffRepo.GetStaffByUserID(user.ID)
	if err != nil {
		log.Printf("ERROR: Failed to get lounge staff record for user %s: %v", user.ID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "staff_lookup_failed",
			Message: "Failed to fetch lounge staff profile",
		})
		return
	}

	if staff == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "staff_not_found",
			Message: "Lounge staff profile not found",
		})
		return
	}

	// Ensure lounge_staff role exists on user
	hasRole := false
	for _, role := range user.Roles {
		if role == "lounge_staff" {
			hasRole = true
			break
		}
	}
	if !hasRole {
		if err := h.userRepository.AddRole(user.ID, "lounge_staff"); err != nil {
			log.Printf("ERROR: Failed to add lounge_staff role to user %s: %v", user.ID, err)
		} else {
			user.Roles = append(user.Roles, "lounge_staff")
		}
	}

	// Generate tokens
	accessToken, err := h.jwtService.GenerateAccessToken(user.ID, user.Phone, user.Roles, user.ProfileCompleted)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "token_generation_failed",
			Message: "Failed to generate access token",
		})
		return
	}

	refreshToken, err := h.jwtService.GenerateRefreshToken(user.ID, user.Phone)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "token_generation_failed",
			Message: "Failed to generate refresh token",
		})
		return
	}

	// Store refresh token in database
	expiresAt := time.Now().Add(7 * 24 * time.Hour)
	deviceID := c.GetHeader("X-Device-ID")
	deviceType := c.GetHeader("X-Device-Type")

	err = h.refreshTokenRepository.StoreRefreshToken(
		user.ID,
		refreshToken,
		deviceID,
		deviceType,
		clientIP,
		userAgent,
		expiresAt,
	)
	if err != nil {
		log.Printf("WARNING: Failed to store refresh token for user %s: %v", user.ID, err)
	}

	// Audit & session
	h.auditService.LogOTPVerification(&user.ID, phone, true, 3-remainingBefore+1, clientIP, userAgent, "")
	h.auditService.LogLogin(user.ID, phone, clientIP, userAgent, deviceID, deviceType)
	if deviceID != "" && deviceType != "" {
		_, _ = h.userSessionRepository.CreateOrUpdateSession(
			user.ID,
			deviceID,
			deviceType,
			c.GetHeader("X-Device-Model"),
			c.GetHeader("X-App-Version"),
			c.GetHeader("X-OS-Version"),
			clientIP,
			c.GetHeader("X-FCM-Token"),
		)
	}

	// Helpers for nullable fields
	getNullString := func(ns sql.NullString) interface{} {
		if ns.Valid {
			return ns.String
		}
		return nil
	}
	getNullTime := func(nt sql.NullTime) interface{} {
		if nt.Valid {
			return nt.Time
		}
		return nil
	}

	staffPayload := map[string]interface{}{
		"id":                staff.ID,
		"user_id":           staff.UserID,
		"lounge_id":         staff.LoungeID,
		"full_name":         getNullString(staff.FullName),
		"nic_number":        getNullString(staff.NICNumber),
		"email":             getNullString(staff.Email),
		"profile_completed": staff.ProfileCompleted,
		"approval_status":   staff.ApprovalStatus,
		"employment_status": staff.EmploymentStatus,
		"hired_date":        getNullTime(staff.HiredDate),
		"terminated_date":   getNullTime(staff.TerminatedDate),
		"notes":             getNullString(staff.Notes),
		"created_at":        staff.CreatedAt,
		"updated_at":        staff.UpdatedAt,
	}

	// Response
	c.JSON(http.StatusOK, VerifyOTPLoungeStaffRegisteredResponse{
		Message:          "OTP verified successfully",
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		ExpiresIn:        3600,
		IsNewUser:        false,
		ProfileComplete:  staff.ProfileCompleted,
		Roles:            user.Roles,
		ApprovalStatus:   string(staff.ApprovalStatus),
		EmploymentStatus: string(staff.EmploymentStatus),
		Staff:            staffPayload,
	})
}

// GetOTPStatus handles GET /api/v1/auth/otp-status/:phone
func (h *AuthHandler) GetOTPStatus(c *gin.Context) {
	phone := c.Param("phone")

	// Validate phone number
	phone, err := h.phoneValidator.Validate(phone)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_phone",
			Message: err.Error(),
		})
		return
	}

	// Get OTP stats
	stats, err := h.otpService.GetOTPStats(phone)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "stats_retrieval_failed",
			Message: "Failed to retrieve OTP status",
		})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// ProfileResponse represents the user profile data
type ProfileResponse struct {
	ID               string   `json:"id"`
	Phone            string   `json:"phone"`
	Email            *string  `json:"email"`
	FirstName        *string  `json:"first_name"`
	LastName         *string  `json:"last_name"`
	NIC              *string  `json:"nic"`
	DateOfBirth      *string  `json:"date_of_birth"`
	Address          *string  `json:"address"`
	City             *string  `json:"city"`
	PostalCode       *string  `json:"postal_code"`
	Roles            []string `json:"roles"`
	ProfilePhotoURL  *string  `json:"profile_photo_url"`
	ProfileCompleted bool     `json:"profile_completed"`
	Status           string   `json:"status"`
	PhoneVerified    bool     `json:"phone_verified"`
	EmailVerified    bool     `json:"email_verified"`
}

// UpdateProfileRequest represents the request to update profile
type UpdateProfileRequest struct {
	FirstName  string `json:"first_name" binding:"required"`
	LastName   string `json:"last_name" binding:"required"`
	Email      string `json:"email" binding:"required,email"`
	Address    string `json:"address" binding:"required"`
	City       string `json:"city"`
	PostalCode string `json:"postal_code"`
}

// CompleteBasicProfileRequest represents request for completing basic profile (first_name + last_name only)
// Used by passenger app after OTP verification for new users
type CompleteBasicProfileRequest struct {
	FirstName string `json:"first_name" binding:"required,min=1,max=100"`
	LastName  string `json:"last_name" binding:"required,min=1,max=100"`
}

// CompleteBasicProfile handles POST /api/v1/auth/complete-basic-profile
// This is a simplified endpoint for passengers that only requires first_name and last_name
// Data is stored in the passengers table (not users table)
func (h *AuthHandler) CompleteBasicProfile(c *gin.Context) {
	// Get user context from middleware
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "User context not found",
		})
		return
	}

	// Parse request body
	var req CompleteBasicProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: "First name and last name are required",
		})
		return
	}

	// Ensure passenger record exists
	_, _, err := h.passengerRepository.GetOrCreatePassenger(userCtx.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "passenger_creation_failed",
			Message: "Failed to create passenger record",
		})
		return
	}

	// Update first_name and last_name in passengers table
	err = h.passengerRepository.UpdatePassengerNames(userCtx.UserID, req.FirstName, req.LastName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "profile_update_failed",
			Message: "Failed to update passenger record",
		})
		return
	}

	// Also update first_name and last_name in users table for synchronization
	err = h.userRepository.UpdateUserNames(userCtx.UserID, req.FirstName, req.LastName)
	if err != nil {
		log.Printf("WARNING: Failed to update user names for synchronization (user %s): %v", userCtx.UserID, err)
	}

	// Set profile as completed in passengers table
	err = h.passengerRepository.SetPassengerProfileCompleted(userCtx.UserID, true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "profile_completion_failed",
			Message: "Failed to mark profile as completed",
		})
		return
	}

	// Get user data for response
	user, err := h.userRepository.GetUserByID(userCtx.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "profile_retrieval_failed",
			Message: "Failed to retrieve user profile",
		})
		return
	}

	// Get passenger profile
	passenger, err := h.passengerRepository.GetPassengerByUserID(userCtx.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "profile_retrieval_failed",
			Message: "Failed to retrieve passenger profile",
		})
		return
	}

	// Convert to response format
	response := ProfileResponse{
		ID:               user.ID.String(),
		Phone:            user.Phone,
		Roles:            user.Roles,
		ProfileCompleted: passenger.ProfileCompleted, // Use passenger's profile_completed
		Status:           user.Status,
		PhoneVerified:    user.PhoneVerified,
		EmailVerified:    false, // Passengers don't have email verification in users table anymore
	}

	// Handle nullable fields from passenger table
	if passenger.FirstName.Valid {
		response.FirstName = &passenger.FirstName.String
	}
	if passenger.LastName.Valid {
		response.LastName = &passenger.LastName.String
	}
	if passenger.Email.Valid {
		response.Email = &passenger.Email.String
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Profile completed successfully",
		"profile": response,
	})
}

// GetProfile handles GET /api/v1/auth/profile
// For passengers, profile data comes from passengers table
// For other roles, profile data comes from their respective tables (bus_owners, bus_staff, etc.)
func (h *AuthHandler) GetProfile(c *gin.Context) {
	// Get user context from middleware
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "User context not found",
		})
		return
	}

	// Get user from database
	user, err := h.userRepository.GetUserByID(userCtx.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "profile_retrieval_failed",
			Message: "Failed to retrieve user profile",
		})
		return
	}

	if user == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "user_not_found",
			Message: "User profile not found",
		})
		return
	}

	// Initialize response with user data
	response := ProfileResponse{
		ID:            user.ID.String(),
		Phone:         user.Phone,
		Roles:         user.Roles,
		Status:        user.Status,
		PhoneVerified: user.PhoneVerified,
		EmailVerified: false, // Will be updated based on role
	}

	// For passengers, get profile data from passengers table
	if h.userRepository.HasRole(user, "passenger") {
		passenger, err := h.passengerRepository.GetPassengerByUserID(user.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error:   "profile_retrieval_failed",
				Message: "Failed to retrieve passenger profile",
			})
			return
		}

		if passenger != nil {
			response.ProfileCompleted = passenger.ProfileCompleted

			// Handle nullable fields from passenger table
			if passenger.Email.Valid {
				response.Email = &passenger.Email.String
			}
			if passenger.FirstName.Valid {
				response.FirstName = &passenger.FirstName.String
			}
			if passenger.LastName.Valid {
				response.LastName = &passenger.LastName.String
			}
			if passenger.NIC.Valid {
				response.NIC = &passenger.NIC.String
			}
			if passenger.DateOfBirth.Valid {
				dob := passenger.DateOfBirth.Time.Format("2006-01-02")
				response.DateOfBirth = &dob
			}
			if passenger.Address.Valid {
				response.Address = &passenger.Address.String
			}
			if passenger.City.Valid {
				response.City = &passenger.City.String
			}
			if passenger.PostalCode.Valid {
				response.PostalCode = &passenger.PostalCode.String
			}
			if passenger.ProfilePhotoURL.Valid {
				response.ProfilePhotoURL = &passenger.ProfilePhotoURL.String
			}
		} else {
			// No passenger record yet, profile not completed
			response.ProfileCompleted = false
		}
	} else {
		// For non-passenger roles, use legacy user table data (will be migrated later)
		response.ProfileCompleted = user.ProfileCompleted
		response.EmailVerified = user.EmailVerified

		// Handle nullable fields from users table (legacy)
		if user.Email.Valid {
			response.Email = &user.Email.String
		}
		if user.FirstName.Valid {
			response.FirstName = &user.FirstName.String
		}
		if user.LastName.Valid {
			response.LastName = &user.LastName.String
		}
		if user.NIC.Valid {
			response.NIC = &user.NIC.String
		}
		if user.DateOfBirth.Valid {
			dob := user.DateOfBirth.Time.Format("2006-01-02")
			response.DateOfBirth = &dob
		}
		if user.Address.Valid {
			response.Address = &user.Address.String
		}
		if user.City.Valid {
			response.City = &user.City.String
		}
		if user.PostalCode.Valid {
			response.PostalCode = &user.PostalCode.String
		}
		if user.ProfilePhotoURL.Valid {
			response.ProfilePhotoURL = &user.ProfilePhotoURL.String
		}
	}

	c.JSON(http.StatusOK, response)
}

// UpdateProfile handles PUT /api/v1/auth/profile
func (h *AuthHandler) UpdateProfile(c *gin.Context) {
	// Get user context from middleware
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "User context not found",
		})
		return
	}

	// Parse request body
	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
		return
	}

	// Update profile in users table
	err := h.userRepository.UpdateProfile(
		userCtx.UserID,
		req.FirstName,
		req.LastName,
		req.Email,
		req.Address,
		req.City,
		req.PostalCode,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "profile_update_failed",
			Message: "Failed to update user profile",
		})
		return
	}

	// Get user to check roles
	user, err := h.userRepository.GetUserByID(userCtx.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "profile_retrieval_failed",
			Message: "Failed to retrieve user profile for role checking",
		})
		return
	}

	// If user is a passenger, update the passengers table too
	isPassenger := h.userRepository.HasRole(user, "passenger")
	if isPassenger {
		err = h.passengerRepository.UpdatePassengerProfile(
			user.ID,
			req.FirstName,
			req.LastName,
			req.Email,
			req.Address,
			req.City,
			req.PostalCode,
		)
		if err != nil {
			log.Printf("WARNING: Failed to update passenger profile for user %s: %v", user.ID, err)
			// We don't return error here because the main user record was updated,
			// but this is why users see old data in the app
		}
	}

	// Update profile completion status
	err = h.userRepository.UpdateProfileCompletion(userCtx.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "profile_completion_check_failed",
			Message: "Failed to check profile completion",
		})
		return
	}

	// If passenger, sync the profile_completed status to passengers table
	if isPassenger {
		// Re-fetch user to get the newly updated profile_completed status from users table
		updatedUser, err := h.userRepository.GetUserByID(userCtx.UserID)
		if err == nil {
			err = h.passengerRepository.SetPassengerProfileCompleted(updatedUser.ID, updatedUser.ProfileCompleted)
			if err != nil {
				log.Printf("WARNING: Failed to sync passenger profile completion status for user %s: %v", updatedUser.ID, err)
			}
		}
	}

	// Get updated user profile
	user, err = h.userRepository.GetUserByID(userCtx.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "profile_retrieval_failed",
			Message: "Failed to retrieve updated profile",
		})
		return
	}

	// Convert to response format
	response := ProfileResponse{
		ID:               user.ID.String(),
		Phone:            user.Phone,
		Roles:            user.Roles,
		ProfileCompleted: user.ProfileCompleted,
		Status:           user.Status,
		PhoneVerified:    user.PhoneVerified,
		EmailVerified:    user.EmailVerified,
	}

	// Handle nullable fields
	if user.Email.Valid {
		response.Email = &user.Email.String
	}
	if user.FirstName.Valid {
		response.FirstName = &user.FirstName.String
	}
	if user.LastName.Valid {
		response.LastName = &user.LastName.String
	}
	if user.NIC.Valid {
		response.NIC = &user.NIC.String
	}
	if user.DateOfBirth.Valid {
		dob := user.DateOfBirth.Time.Format("2006-01-02")
		response.DateOfBirth = &dob
	}
	if user.Address.Valid {
		response.Address = &user.Address.String
	}
	if user.City.Valid {
		response.City = &user.City.String
	}
	if user.PostalCode.Valid {
		response.PostalCode = &user.PostalCode.String
	}
	if user.ProfilePhotoURL.Valid {
		response.ProfilePhotoURL = &user.ProfilePhotoURL.String
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Profile updated successfully",
		"profile": response,
	})
}

// RefreshTokenRequest represents the request to refresh access token
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
	DeviceID     string `json:"device_id"`
	DeviceType   string `json:"device_type"`
}

// RefreshTokenResponse represents the response after refreshing token
type RefreshTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in_seconds"`
	TokenType    string `json:"token_type"`
}

// RefreshToken handles POST /api/v1/auth/refresh-token
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req RefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("❌ REFRESH TOKEN ERROR: Invalid request body - %v", err)
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: "Invalid request body",
		})
		return
	}

	log.Printf("🔄 REFRESH TOKEN REQUEST: Token length: %d, DeviceID: %s, DeviceType: %s",
		len(req.RefreshToken), req.DeviceID, req.DeviceType)

	// Validate refresh token
	claims, err := h.jwtService.ValidateRefreshToken(req.RefreshToken)
	if err != nil {
		log.Printf("❌ REFRESH TOKEN ERROR: Token validation failed - %v", err)
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "invalid_token",
			Message: "Invalid or expired refresh token",
		})
		return
	}

	log.Printf("✅ REFRESH TOKEN: Token validated successfully for user: %s, phone: %s",
		claims.UserID, claims.Phone)

	log.Printf("✅ REFRESH TOKEN: Token validated successfully for user: %s, phone: %s",
		claims.UserID, claims.Phone)

	// Check if token is revoked in database
	revoked, err := h.refreshTokenRepository.IsTokenRevoked(req.RefreshToken)
	if err != nil {
		log.Printf("❌ REFRESH TOKEN ERROR: Failed to check if token is revoked - %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "token_check_failed",
			Message: "Failed to verify token status",
		})
		return
	}

	if revoked {
		log.Printf("❌ REFRESH TOKEN ERROR: Token has been revoked for user: %s", claims.UserID)
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "token_revoked",
			Message: "Refresh token has been revoked",
		})
		return
	}

	log.Printf("✅ REFRESH TOKEN: Token is not revoked, fetching user: %s", claims.UserID)

	// Get user from database to ensure they still exist and get current profile status
	user, err := h.userRepository.GetUserByID(claims.UserID)
	if err != nil {
		log.Printf("❌ REFRESH TOKEN ERROR: Failed to fetch user %s - %v", claims.UserID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "user_fetch_failed",
			Message: "Failed to fetch user information",
		})
		return
	}

	if user == nil {
		log.Printf("❌ REFRESH TOKEN ERROR: User %s no longer exists", claims.UserID)
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "user_not_found",
			Message: "User no longer exists",
		})
		return
	}

	log.Printf("✅ REFRESH TOKEN: User found - ID: %s, Status: %s", user.ID, user.Status)

	// Check if user is active
	if user.Status != "active" {
		log.Printf("❌ REFRESH TOKEN ERROR: User %s is not active, status: %s", user.ID, user.Status)
		c.JSON(http.StatusForbidden, ErrorResponse{
			Error:   "user_inactive",
			Message: "User account is not active",
		})
		return
	}

	// Update last used timestamp for the old refresh token
	if err := h.refreshTokenRepository.UpdateLastUsed(req.RefreshToken); err != nil {
		// Log error but don't fail the request
		// In production, you'd log this properly
	}

	// Generate new access token FIRST (before revoking old token)
	accessToken, err := h.jwtService.GenerateAccessToken(
		user.ID,
		user.Phone,
		user.Roles,
		user.ProfileCompleted,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "token_generation_failed",
			Message: "Failed to generate new access token",
		})
		return
	}

	// Generate new refresh token
	newRefreshToken, err := h.jwtService.GenerateRefreshToken(user.ID, user.Phone)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "token_generation_failed",
			Message: "Failed to generate new refresh token",
		})
		return
	}

	// Store new refresh token in database BEFORE revoking old one
	clientIP := utils.GetRealIP(c)
	userAgent := utils.GetUserAgent(c)
	expiresAt := time.Now().Add(7 * 24 * time.Hour) // 7 days

	err = h.refreshTokenRepository.StoreRefreshToken(
		user.ID,
		newRefreshToken,
		req.DeviceID,
		req.DeviceType,
		clientIP,
		userAgent,
		expiresAt,
	)
	if err != nil {
		// Log failed token refresh
		h.auditService.LogTokenRefresh(user.ID, clientIP, userAgent, false)

		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "token_storage_failed",
			Message: "Failed to store new refresh token",
		})
		return
	}

	// ✅ IMPORTANT: Revoke old token AFTER successfully storing new one (token rotation)
	// This prevents race conditions where concurrent requests might fail
	if err := h.refreshTokenRepository.RevokeToken(req.RefreshToken); err != nil {
		// Log error but don't fail the request - new tokens are already issued
		log.Printf("⚠️ REFRESH TOKEN WARNING: Failed to revoke old token (non-critical): %v", err)
	}

	// Log successful token refresh
	h.auditService.LogTokenRefresh(user.ID, clientIP, userAgent, true)

	log.Printf("✅ REFRESH TOKEN SUCCESS: New tokens generated for user: %s", user.ID)

	c.JSON(http.StatusOK, RefreshTokenResponse{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		ExpiresIn:    3600, // 1 hour
		TokenType:    "Bearer",
	})
}

// LogoutRequest represents the request to logout
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token"`
	LogoutAll    bool   `json:"logout_all"` // If true, logout from all devices
}

// Logout handles POST /api/v1/auth/logout
func (h *AuthHandler) Logout(c *gin.Context) {
	// Get user context from JWT middleware
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "User context not found",
		})
		return
	}

	// Get real client IP and user agent for audit logging
	clientIP := utils.GetRealIP(c)
	userAgent := utils.GetUserAgent(c)

	var req LogoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// If no body provided, default to single device logout
		log.Printf("No request body, defaulting to single device logout for user %s", userCtx.UserID)
		req.LogoutAll = false
	}

	// Log the received request for debugging
	log.Printf("Logout request received - User: %s, LogoutAll: %v, HasRefreshToken: %v",
		userCtx.UserID, req.LogoutAll, req.RefreshToken != "")

	if req.LogoutAll {
		// Revoke all refresh tokens for the user
		log.Printf("Revoking all tokens for user %s", userCtx.UserID)
		err := h.refreshTokenRepository.RevokeAllUserTokens(userCtx.UserID)
		if err != nil {
			log.Printf("ERROR: Failed to revoke all tokens for user %s: %v", userCtx.UserID, err)
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error:   "logout_failed",
				Message: "Failed to logout from all devices",
			})
			return
		}

		// Deactivate all user sessions
		err = h.userSessionRepository.DeactivateAllUserSessions(userCtx.UserID)
		if err != nil {
			log.Printf("WARNING: Failed to deactivate all sessions for user %s: %v", userCtx.UserID, err)
			// Don't fail the logout
		}

		// Log logout from all devices
		h.auditService.LogLogout(userCtx.UserID, clientIP, userAgent, true)

		c.JSON(http.StatusOK, gin.H{
			"message": "Successfully logged out from all devices",
		})
		return
	}

	// Single device logout
	// If specific refresh token provided, revoke it
	if req.RefreshToken != "" {
		log.Printf("Revoking specific token for user %s", userCtx.UserID)
		err := h.refreshTokenRepository.RevokeToken(req.RefreshToken)
		if err != nil {
			// Check if token is already revoked - this is not an error for logout
			if err.Error() == "token not found or already revoked" {
				log.Printf("INFO: Token already revoked for user %s - treating as success", userCtx.UserID)
			} else {
				log.Printf("ERROR: Failed to revoke token for user %s: %v", userCtx.UserID, err)
				c.JSON(http.StatusInternalServerError, ErrorResponse{
					Error:   "logout_failed",
					Message: "Failed to revoke token",
				})
				return
			}
		}

		// Deactivate session for this device
		deviceID := c.GetHeader("X-Device-ID")
		if deviceID != "" {
			err = h.userSessionRepository.DeactivateSession(userCtx.UserID, deviceID)
			if err != nil {
				log.Printf("WARNING: Failed to deactivate session for user %s device %s: %v", userCtx.UserID, deviceID, err)
				// Don't fail the logout
			}
		}

		// Log single device logout
		h.auditService.LogLogout(userCtx.UserID, clientIP, userAgent, false)

		c.JSON(http.StatusOK, gin.H{
			"message": "Successfully logged out",
		})
		return
	}

	// If no refresh token provided, revoke the most recent active token
	// This handles the case where Flutter sends logout_all: false but no refresh_token
	log.Printf("No refresh token provided, revoking most recent token for user %s", userCtx.UserID)
	err := h.refreshTokenRepository.RevokeMostRecentToken(userCtx.UserID)
	if err != nil {
		log.Printf("ERROR: Failed to revoke most recent token for user %s: %v", userCtx.UserID, err)
		// Don't fail the logout - client-side logout is still valid
		log.Printf("WARN: Server-side token revocation failed, but allowing logout")
	}

	// Deactivate session for this device
	deviceID := c.GetHeader("X-Device-ID")
	if deviceID != "" {
		err = h.userSessionRepository.DeactivateSession(userCtx.UserID, deviceID)
		if err != nil {
			log.Printf("WARNING: Failed to deactivate session for user %s device %s: %v", userCtx.UserID, deviceID, err)
			// Don't fail the logout
		}
	}

	// Log logout
	h.auditService.LogLogout(userCtx.UserID, clientIP, userAgent, false)

	c.JSON(http.StatusOK, gin.H{
		"message": "Successfully logged out",
	})
}

// VerifyOTPGeneric handles POST /api/v1/auth/verify-otp-generic
// Generic OTP verification endpoint that:
// - Creates user with NO role assigned if new
// - Returns empty roles array for new users
// - Returns existing roles for existing users
// - Does NOT automatically assign any role
func (h *AuthHandler) VerifyOTPGeneric(c *gin.Context) {
	var req VerifyOTPRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "validation_error",
			Message: "Invalid request body",
		})
		return
	}

	// Validate phone number
	phone, err := h.phoneValidator.Validate(req.Phone)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_phone",
			Message: err.Error(),
		})
		return
	}

	// Get real client IP and user agent
	clientIP := utils.GetRealIP(c)
	userAgent := utils.GetUserAgent(c)

	// Get current attempts before validation
	remainingBefore, _ := h.otpService.GetRemainingAttempts(phone)

	// Validate OTP
	valid, err := h.otpService.ValidateOTP(phone, req.OTP)
	if err != nil {
		// Log failed verification
		attempts := 3 - remainingBefore + 1
		h.auditService.LogOTPVerification(nil, phone, false, attempts, clientIP, userAgent, err.Error())

		// Check specific error types
		switch err {
		case services.ErrOTPExpired:
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "otp_expired",
				Message: "OTP has expired. Please request a new one.",
				Code:    "OTP_EXPIRED",
			})
		case services.ErrOTPInvalid:
			remaining, _ := h.otpService.GetRemainingAttempts(phone)
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "otp_invalid",
				Message: "Invalid OTP code",
				Code:    "OTP_INVALID",
			})
			c.Header("X-Remaining-Attempts", string(rune(remaining)))
		case services.ErrMaxAttemptsExceeded:
			c.JSON(http.StatusTooManyRequests, ErrorResponse{
				Error:   "max_attempts_exceeded",
				Message: "Maximum OTP validation attempts exceeded. Please request a new OTP.",
				Code:    "MAX_ATTEMPTS",
			})
		case services.ErrNoOTPFound:
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error:   "no_otp_found",
				Message: "No OTP found for this phone number. Please request an OTP first.",
				Code:    "NO_OTP",
			})
		case services.ErrOTPAlreadyUsed:
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "otp_already_used",
				Message: "This OTP has already been used. Please request a new one.",
				Code:    "OTP_USED",
			})
		default:
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error:   "validation_failed",
				Message: "Failed to validate OTP",
			})
		}
		return
	}

	if !valid {
		// Log invalid OTP
		attempts := 3 - remainingBefore + 1
		h.auditService.LogOTPVerification(nil, phone, false, attempts, clientIP, userAgent, "invalid_code")

		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "otp_invalid",
			Message: "Invalid OTP code",
		})
		return
	}

	// Check if user already exists
	existingUser, err := h.userRepository.GetUserByPhone(phone)
	if err != nil {
		log.Printf("ERROR: Failed to check existing user for phone %s: %v", phone, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "user_check_failed",
			Message: "Failed to check user status",
		})
		return
	}

	var user *models.User
	isNew := false

	if existingUser != nil {
		// EXISTING USER - Return with their current roles
		user = existingUser
		log.Printf("INFO: Existing user verified via generic OTP endpoint: %s (roles: %v)", phone, user.Roles)
	} else {
		// NEW USER - Create with NO role assigned
		user, err = h.userRepository.CreateUserWithoutRole(phone)
		if err != nil {
			log.Printf("ERROR: Failed to create user without role for phone %s: %v", phone, err)
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error:   "user_creation_failed",
				Message: "Failed to create user account",
			})
			return
		}
		isNew = true
		log.Printf("INFO: New user created via generic OTP endpoint (no role assigned): %s", phone)
	}

	// Generate JWT tokens with user's actual roles (empty for new users without roles)
	accessToken, err := h.jwtService.GenerateAccessToken(
		user.ID,
		user.Phone,
		user.Roles, // Will be empty array for new users, or existing roles for returning users
		false,      // profile_completed = false initially
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "token_generation_failed",
			Message: "Failed to generate access token",
		})
		return
	}

	refreshToken, err := h.jwtService.GenerateRefreshToken(user.ID, user.Phone)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "token_generation_failed",
			Message: "Failed to generate refresh token",
		})
		return
	}

	// Store refresh token in database
	expiresAt := time.Now().Add(7 * 24 * time.Hour) // 7 days

	// Get device info from request if provided
	deviceID := c.GetHeader("X-Device-ID")
	deviceType := c.GetHeader("X-Device-Type")

	err = h.refreshTokenRepository.StoreRefreshToken(
		user.ID,
		refreshToken,
		deviceID,
		deviceType,
		clientIP,
		userAgent,
		expiresAt,
	)
	if err != nil {
		// Log error but don't fail the login
		log.Printf("WARNING: Failed to store refresh token for user %s: %v", user.ID, err)
	}

	// Log successful OTP verification
	h.auditService.LogOTPVerification(&user.ID, phone, true, 3-remainingBefore+1, clientIP, userAgent, "")
	h.auditService.LogLogin(user.ID, phone, clientIP, userAgent, deviceID, deviceType)

	// Create or update user session
	deviceModel := c.GetHeader("X-Device-Model")
	appVersion := c.GetHeader("X-App-Version")
	osVersion := c.GetHeader("X-OS-Version")
	fcmToken := c.GetHeader("X-FCM-Token")

	if deviceID != "" && deviceType != "" {
		_, err = h.userSessionRepository.CreateOrUpdateSession(
			user.ID,
			deviceID,
			deviceType,
			deviceModel,
			appVersion,
			osVersion,
			clientIP,
			fcmToken,
		)
		if err != nil {
			log.Printf("WARNING: Failed to create/update session for user %s: %v", user.ID, err)
		}
	}

	c.JSON(http.StatusOK, VerifyOTPResponse{
		Message:         "OTP verified successfully",
		AccessToken:     accessToken,
		RefreshToken:    refreshToken,
		ExpiresIn:       3600, // 1 hour
		IsNewUser:       isNew,
		ProfileComplete: false,      // No profile until role is assigned
		Roles:           user.Roles, // Will be empty for new users
	})
}
