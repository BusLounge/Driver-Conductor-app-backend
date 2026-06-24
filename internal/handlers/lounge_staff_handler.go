package handlers

import (
	"database/sql"
	"log"
	"net/http"
	"strconv"
	"strings"

	//"os/user"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/smarttransit/sms-auth-backend/internal/database"
	"github.com/smarttransit/sms-auth-backend/internal/middleware"

	// importing validator
	"github.com/smarttransit/sms-auth-backend/pkg/validator"
	// import models
	"github.com/smarttransit/sms-auth-backend/internal/models"
)

// LoungeStaffHandler handles lounge staff-related HTTP requests
type LoungeStaffHandler struct {
	staffRepo              *database.LoungeStaffRepository
	loungeRepo             *database.LoungeRepository
	loungeOwnerRepo        *database.LoungeOwnerRepository
	// adding user repo (THIS WILL HELP IN FINDING IF A USER IS ALREADY REGISTERD WHEN ADDING BY OWNERS SIDE)
	userRepo *database.UserRepository
	// adding phone validator to validate phone numbers directly inside the handler(for owner to directly add staff)
	phoneValidator *validator.PhoneValidator
	// adding loungeStaff repository
	loungeStaffRepo *database.LoungeStaffRepository
	// adding booking repository
	bookingRepo *database.BookingRepository
	// adding lounge booking repository for staff to view bookings
	loungeBookingRepo *database.LoungeBookingRepository
}

// NewLoungeStaffHandler creates a new lounge staff handler
func NewLoungeStaffHandler(
	staffRepo *database.LoungeStaffRepository,
	loungeRepo *database.LoungeRepository,
	loungeOwnerRepo *database.LoungeOwnerRepository,
	userRepo *database.UserRepository,
	phoneValidator *validator.PhoneValidator,
	loungeStaffRepo *database.LoungeStaffRepository,
	bookingRepo *database.BookingRepository,
	loungeBookingRepo *database.LoungeBookingRepository,
) *LoungeStaffHandler {
	return &LoungeStaffHandler{
		staffRepo:         staffRepo,
		loungeRepo:        loungeRepo,
		loungeOwnerRepo:   loungeOwnerRepo,
		userRepo:          userRepo,
		phoneValidator:    phoneValidator,
		loungeStaffRepo:   loungeStaffRepo,
		bookingRepo:       bookingRepo,
		loungeBookingRepo: loungeBookingRepo,
	}
}

// ===================================================================
// GET PROFILE
// ===================================================================

// GetProfile handles GET /api/v1/lounge-staff/profile
// Returns the current staff profile including approval status, employment status, and details
func (h *LoungeStaffHandler) GetProfile(c *gin.Context) {
	// Get user context from JWT middleware
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "User context not found",
		})
		return
	}

	// Get lounge staff record for this user
	staff, err := h.staffRepo.GetStaffByUserID(userCtx.UserID)
	if err != nil {
		log.Printf("ERROR: Failed to get lounge staff for user %s: %v", userCtx.UserID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to retrieve profile",
		})
		return
	}

	if staff == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "Lounge staff profile not found",
		})
		return
	}

	// Get user record to fetch phone number
	user, err := h.userRepo.GetUserByID(userCtx.UserID)
	if err != nil {
		log.Printf("ERROR: Failed to get user for staff %s: %v", userCtx.UserID, err)
		// Continue anyway, phone is optional in response
	}

	// Get lounge information
	lounge, err := h.loungeRepo.GetLoungeByID(staff.LoungeID)
	if err != nil {
		log.Printf("ERROR: Failed to get lounge for staff %s: %v", staff.ID, err)
		// Don't fail if lounge fetch fails, continue with staff data
	}

	// Helper function to extract values from sql.Null* types
	getNullableString := func(ns sql.NullString) *string {
		if ns.Valid {
			return &ns.String
		}
		return nil
	}

	getNullableTime := func(nt sql.NullTime) *string {
		if nt.Valid {
			timeStr := nt.Time.Format("2006-01-02T15:04:05Z07:00")
			return &timeStr
		}
		return nil
	}

	// Build response
	response := gin.H{
		"id":      staff.ID,
		"user_id": staff.UserID,
		"phone": func() *string {
			if user != nil {
				return &user.Phone
			}
			return nil
		}(),
		"lounge_id":          staff.LoungeID,
		"full_name":          getNullableString(staff.FullName),
		"nic_number":         getNullableString(staff.NICNumber),
		"email":              getNullableString(staff.Email),
		"profile_completed":  staff.ProfileCompleted,
		"approval_status":    staff.ApprovalStatus,
		"employment_status":  staff.EmploymentStatus,
		"hired_date":         getNullableTime(staff.HiredDate),
		"terminated_date":    getNullableTime(staff.TerminatedDate),
		"notes":              getNullableString(staff.Notes),
		"created_at":         staff.CreatedAt,
		"updated_at":         staff.UpdatedAt,
	}

	// Add lounge information if available
	if lounge != nil {
		response["lounge"] = gin.H{
			"id":     lounge.ID,
			"status": lounge.Status,
		}
	}

	c.JSON(http.StatusOK, response)
}

// ===================================================================
// ADD STAFF TO LOUNGE
// ===================================================================

// AddStaffRequest represents the staff creation request
type AddStaffRequest struct {
	LoungeID string `json:"lounge_id" binding:"required"`
	Phone    string `json:"phone" binding:"required"` // Staff's phone number
}

type ApproveStaffRequest struct {
	ApprovalStatus string `json:"approval_status" binding:"required,oneof=approved declined"`
}

// Direct staff add request (FOR OWNER TO ADD STAFF DIRECTLY)
type AddStaffToLoungeDirectByOwnerRequest struct {
	LoungeID  string `json:"lounge_id" binding:"required"`   // UUID of the lounge
	FullName  string `json:"full_name" binding:"required"`   // Staff member's full name
	NICNumber string `json:"nic_number" binding:"required"`  // NIC for identification
	Phone     string `json:"phone" binding:"required"`       // Phone for user lookup or creation
}

// DirectAddStaffResponse represents the response after adding staff
type AddStaffToLoungeDirectByOwnerResponse struct {
	ID                uuid.UUID `json:"id"`
	LoungeID          uuid.UUID `json:"lounge_id"`
	UserID            uuid.UUID `json:"user_id"`
	FullName          string    `json:"full_name"`
	NICNumber         string    `json:"nic_number"`
	ProfileCompleted  bool      `json:"profile_completed"`
	ApprovalStatus    string    `json:"approval_status"`
	EmploymentStatus  string    `json:"employment_status"`
	HiredDate         time.Time `json:"hired_date"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	IsNewUser         bool      `json:"is_new_user"`
	Message           string    `json:"message"` //kept for messages(if not needed delete in future)
}

// AddStaff handles POST /api/v1/lounges/:id/staff/direct-add
// This will only be used by lounge owner inorder to add staff directly to the lounge (No OTP verification and stuff)
func (h *LoungeStaffHandler) AddStaffToLoungeDirectByOwner(c *gin.Context) {

	// Get user context from JWT middleware
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "User context not found",
		})
		return
	}

	// Parse request body
	var req AddStaffToLoungeDirectByOwnerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "validation_error",
			Message: "Invalid request body: " + err.Error(),
		})
		return
	}

	// Parse lounge ID directly from the request struct
	loungeID, err := uuid.Parse(req.LoungeID)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_lounge_id",
			Message: "Invalid lounge ID format",
		})
		return
	}

	// Verify the user is a lounge owner
	owner, err := h.loungeOwnerRepo.GetLoungeOwnerByUserID(userCtx.UserID)
	if err != nil || owner == nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "Only lounge owners can add staff",
		})
		return
	}

	// Get the lounge to verify ownership
	lounge, err := h.loungeRepo.GetLoungeByID(loungeID)
	if err != nil || lounge == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "Lounge not found",
		})
		return
	}

	// Verify the user owns this lounge
	if lounge.LoungeOwnerID != owner.ID {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Error:   "forbidden",
			Message: "You don't have permission to add staff to this lounge",
		})
		return
	}

	// if lounge is not approved send error
	if lounge.Status != models.LoungeStatusApproved {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "lounge_not_approved",
			Message: "Selected lounge is not available for staff registration",
		})
		return
	}

	// Validate and normalize the phone number
	phone, err := h.phoneValidator.Validate(req.Phone)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_phone",
			Message: err.Error(),
		})
		return
	}

	// USER CREATION PART - STEP 01
	// Check if a user with this phone already exists
	existingUser, err := h.userRepo.GetUserByPhone(phone)
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
        
		existingStaff, err := h.loungeStaffRepo.GetStaffByLoungeIDAndUserID(c.Request.Context(), loungeID, user.ID)
		 if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check existing staff"})
			return
		}
		if existingStaff != nil {
			c.JSON(http.StatusConflict, gin.H{"error": "Staff member already exists for this lounge"})
			return
		}

		// Add lounge_staff role if not present
		hasRole := false
		for _, r := range user.Roles {
			if r == "lounge_staff" {
				hasRole = true
				break
			}
		}
		if !hasRole {
			if err := h.userRepo.AddRole(user.ID, "lounge_staff"); err != nil {
				log.Printf("ERROR: Failed to add lounge_staff role: %v", err)
			} else {
				user.Roles = append(user.Roles, "lounge_staff")
			}
		}

	} else {
		// NEW USER - Create with basic data only (phone + role)
		// user, err = h.userRepo.CreateUserWithRole(phone, "lounge_staff")
		// if err != nil {
		// 	log.Printf("ERROR: Failed to create lounge staff user for phone %s: %v", phone, err)
		// 	c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "user_creation_failed", Message: "Failed to create user account"})
		// 	return
		// }

		email:=""//email is set to null and sending 

		// USER CREATION WITH MORE DETAILS
		user, err = h.userRepo.CreateUserWithFullData(phone,"lounge_staff",req.FullName,req.NICNumber,email)
		if err != nil {
			log.Printf("ERROR: Failed to create lounge staff user for phone %s: %v", phone, err)
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "user_creation_failed", Message: "Failed to create user account"})
			return
		}

		isNew = true
	}

	// LOUNGE STAFF RECORD CREATION - STEP 02
	staff, err := h.staffRepo.AddStaffToLoungeDirectByOwner(
		loungeID,
		user.ID,
		req.FullName,
		req.NICNumber,
	)
	if err != nil {
		log.Printf("ERROR: Failed to add staff: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to add staff member",
		})
		return
	}

	// Return success response
	c.JSON(http.StatusCreated, AddStaffToLoungeDirectByOwnerResponse{
		ID:                staff.ID,
		LoungeID:          staff.LoungeID,
		UserID:            staff.UserID,
		FullName:          staff.FullName.String,
		NICNumber:         staff.NICNumber.String,
		ProfileCompleted:  staff.ProfileCompleted,
		ApprovalStatus:    string(staff.ApprovalStatus),
		EmploymentStatus:  string(staff.EmploymentStatus),
		HiredDate:         staff.HiredDate.Time,
		CreatedAt:         staff.CreatedAt,
		UpdatedAt:         staff.UpdatedAt,
		IsNewUser:         isNew,
		Message:           "Staff member and user account created successfully with immediate approval",
	})

}

// ===================================================================
// GET STAFF BY LOUNGE
// ===================================================================

// GetStaffByLounge handles GET /api/v1/lounges/:lounge_id/staff
func (h *LoungeStaffHandler) GetStaffByLounge(c *gin.Context) {
	// Get user context from JWT middleware
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "User context not found",
		})
		return
	}

	loungeIDStr := c.Param("lounge_id")
	loungeID, err := uuid.Parse(loungeIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid lounge ID format",
		})
		return
	}

	// Get lounge owner record
	owner, err := h.loungeOwnerRepo.GetLoungeOwnerByUserID(userCtx.UserID)
	if err != nil || owner == nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "Lounge owner not found",
		})
		return
	}

	// Verify lounge ownership
	lounge, err := h.loungeRepo.GetLoungeByID(loungeID)
	if err != nil || lounge == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "Lounge not found",
		})
		return
	}

	if lounge.LoungeOwnerID != owner.ID {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Error:   "forbidden",
			Message: "You don't have permission to view staff for this lounge",
		})
		return
	}

	// Get staff
	staff, err := h.staffRepo.GetStaffByLoungeID(loungeID)
	if err != nil {
		log.Printf("ERROR: Failed to get staff for lounge %s: %v", loungeID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to retrieve staff",
		})
		return
	}

	// Convert to response format - only using fields that exist in new schema
	response := make([]gin.H, 0, len(staff))
	for _, s := range staff {
		response = append(response, gin.H{
			"id":                s.ID,
			"lounge_id":         s.LoungeID,
			"user_id":           s.UserID,
			"full_name":         s.FullName,
			"nic_number":        s.NICNumber,
			"email":             s.Email,
			"profile_completed": s.ProfileCompleted,
			"employment_status": s.EmploymentStatus,
			"hired_date":        s.HiredDate,
			"terminated_date":   s.TerminatedDate,
			"notes":             s.Notes,
			"created_at":        s.CreatedAt,
			"updated_at":        s.UpdatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"staff": response,
		"total": len(response),
	})
}

// ===================================================================
// UPDATE STAFF PERMISSION - REMOVED (Use users.roles instead)
// ===================================================================
// Permission management moved to users table roles array

// ===================================================================
// UPDATE STAFF EMPLOYMENT STATUS
// ===================================================================

// UpdateStaffStatusRequest represents the employment status update request
type UpdateStaffStatusRequest struct {
	EmploymentStatus string `json:"employment_status" binding:"required,oneof=active inactive terminated"`
}

// UpdateStaffStatus handles PUT /api/v1/lounges/:lounge_id/staff/:staff_id/status
func (h *LoungeStaffHandler) UpdateStaffStatus(c *gin.Context) {
	// Get user context from JWT middleware
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "User context not found",
		})
		return
	}

	loungeIDStr := c.Param("id")
	loungeID, err := uuid.Parse(loungeIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid lounge ID format",
		})
		return
	}

	staffIDStr := c.Param("staff_id")
	staffID, err := uuid.Parse(staffIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid staff ID format",
		})
		return
	}

	var req UpdateStaffStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "validation_error",
			Message: "Invalid request body: " + err.Error(),
		})
		return
	}

	// Get lounge owner record
	owner, err := h.loungeOwnerRepo.GetLoungeOwnerByUserID(userCtx.UserID)
	if err != nil || owner == nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "Lounge owner not found",
		})
		return
	}

	// Verify lounge ownership
	lounge, err := h.loungeRepo.GetLoungeByID(loungeID)
	if err != nil || lounge == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "Lounge not found",
		})
		return
	}

	if lounge.LoungeOwnerID != owner.ID {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Error:   "forbidden",
			Message: "You don't have permission to update staff for this lounge",
		})
		return
	}

	// Update employment status using repository method
	err = h.staffRepo.UpdateStaffEmploymentStatus(staffID, req.EmploymentStatus)
	if err != nil {
		log.Printf("ERROR: Failed to update staff status: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "update_failed",
			Message: "Failed to update staff status",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Staff status updated successfully",
	})
}

// ===================================================================
// REMOVE STAFF
// ===================================================================

// RemoveStaff handles DELETE /api/v1/lounges/:lounge_id/staff/:staff_id
func (h *LoungeStaffHandler) RemoveStaff(c *gin.Context) {
	// Get user context from JWT middleware
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "User context not found",
		})
		return
	}

	loungeIDStr := c.Param("id")
	loungeID, err := uuid.Parse(loungeIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid lounge ID format",
		})
		return
	}

	staffIDStr := c.Param("staff_id")
	staffID, err := uuid.Parse(staffIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid staff ID format",
		})
		return
	}

	// Get lounge owner record
	owner, err := h.loungeOwnerRepo.GetLoungeOwnerByUserID(userCtx.UserID)
	if err != nil || owner == nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "Lounge owner not found",
		})
		return
	}

	// Verify lounge ownership
	lounge, err := h.loungeRepo.GetLoungeByID(loungeID)
	if err != nil || lounge == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "Lounge not found",
		})
		return
	}

	if lounge.LoungeOwnerID != owner.ID {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Error:   "forbidden",
			Message: "You don't have permission to remove staff from this lounge",
		})
		return
	}

	// Delete staff record
	err = h.staffRepo.RemoveStaff(staffID)
	if err != nil {
		log.Printf("ERROR: Failed to remove staff %s: %v", staffID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "delete_failed",
			Message: "Failed to remove staff",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Staff removed successfully",
	})
}

// ===================================================================
// GET MY STAFF PROFILE (For staff members to check their lounge)
// ===================================================================

// GetMyStaffProfile handles GET /api/v1/staff/my-profile
func (h *LoungeStaffHandler) GetMyStaffProfile(c *gin.Context) {
	// Get user context from JWT middleware
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "User context not found",
		})
		return
	}

	// Get staff record by user_id
	staff, err := h.staffRepo.GetStaffByUserID(userCtx.UserID)
	if err != nil {
		log.Printf("ERROR: Failed to get staff for user %s: %v", userCtx.UserID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to retrieve staff profile",
		})
		return
	}

	if staff == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "Staff profile not found",
		})
		return
	}

	// Get lounge details
	lounge, err := h.loungeRepo.GetLoungeByID(staff.LoungeID)
	if err != nil {
		log.Printf("ERROR: Failed to get lounge %s: %v", staff.LoungeID, err)
		// Continue without lounge details
	}

	response := gin.H{
		"id":                staff.ID,
		"lounge_id":         staff.LoungeID,
		"user_id":           staff.UserID,
		"full_name":         staff.FullName,
		"nic_number":        staff.NICNumber,
		"email":             staff.Email,
		"profile_completed": staff.ProfileCompleted,
		"employment_status": staff.EmploymentStatus,
		"hired_date":        staff.HiredDate,
		"terminated_date":   staff.TerminatedDate,
		"notes":             staff.Notes,
		"created_at":        staff.CreatedAt,
		"updated_at":        staff.UpdatedAt,
	}

	if lounge != nil {
		response["lounge"] = gin.H{
			"id":          lounge.ID,
			"lounge_name": lounge.LoungeName,
			"address":     lounge.Address,
			"status":      lounge.Status,
		}
	}

	c.JSON(http.StatusOK, response)
}

// ApproveStaff handles PUT /api/v1/lounges/:lounge_id/staff/:staff_id/approval
// handler to handle the staff approvement status
func (h *LoungeStaffHandler) ApproveStaff(c *gin.Context) {

	// Get user context from JWT middleware
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "User context not found",
		})
		return
	}

	// Extract route parameters
	loungeIDStr := c.Param("id")
	staffIDStr := c.Param("staff_id")

	loungeID, err := uuid.Parse(loungeIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_lounge_id",
			Message: "Invalid lounge ID format",
		})
		return
	}

	staffID, err := uuid.Parse(staffIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_staff_id",
			Message: "Invalid staff ID format",
		})
		return
	}

	// Parse request body
	var req ApproveStaffRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "validation_error",
			Message: "Invalid request body: " + err.Error(),
		})
		return
	}

	// Get the lounge owner record for this user
	owner, err := h.loungeOwnerRepo.GetLoungeOwnerByUserID(userCtx.UserID)
	if err != nil || owner == nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "Only lounge owners can view staff",
		})
		return
	}

	// Get the lounge to verify ownership
	lounge, err := h.loungeRepo.GetLoungeByID(loungeID)
	if err != nil {
		log.Printf("ERROR: Failed to get lounge %s: %v", loungeID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to retrieve lounge",
		})
		return
	}

	if lounge == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "Lounge not found",
		})
		return
	}

	// Compare lounge owner IDs 
    if lounge.LoungeOwnerID != owner.ID {
        c.JSON(http.StatusForbidden, ErrorResponse{
            Error:   "forbidden",
            Message: "You do not have permission to view staff for this lounge",
        })
        return
    }

	// Get the staff member to verify they belong to this lounge
	staff, err := h.staffRepo.GetStaffByIDWithDetails(staffID)
	if err != nil {
		log.Printf("ERROR: Failed to get staff %s: %v", staffID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to retrieve staff member",
		})
		return
	}

	if staff == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "Staff member not found",
		})
		return
	}

	// Verify the staff belongs to this lounge
	if staff.LoungeID != loungeID {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: "Staff member does not belong to this lounge",
		})
		return
	}

	// Determine employment status based on approval decision
	var employmentStatus *string
	switch req.ApprovalStatus {
	case "approved":
		activeStatus := "active"
		employmentStatus = &activeStatus
	case "declined":
		inactiveStatus := "terminated"
		employmentStatus = &inactiveStatus
	}

	// Update staff approval status
	err = h.staffRepo.UpdateStaffApprovalStatus(
		staffID,
		req.ApprovalStatus,
		employmentStatus,
	)
	if err != nil {
		log.Printf("ERROR: Failed to update staff approval status for %s: %v", staffID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "update_failed",
			Message: "Failed to update staff approval status",
		})
		return
	}

	// Retrieve updated staff record
	updatedStaff, err := h.staffRepo.GetStaffByIDWithDetails(staffID)
	if err != nil {
		log.Printf("ERROR: Failed to retrieve updated staff %s: %v", staffID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to retrieve updated staff information",
		})
		return
	}

	log.Printf("INFO: Staff %s approval status updated to %s by owner %s",
		staffID, req.ApprovalStatus, userCtx.UserID)

	c.JSON(http.StatusOK, gin.H{
		"message":            "Staff approval status updated successfully",
		"staff_id":           updatedStaff.ID,
		"approval_status":    updatedStaff.ApprovalStatus,
		"employment_status":  updatedStaff.EmploymentStatus,
		"updated_at":         updatedStaff.UpdatedAt,
	})
}

// GetStaffByLoungeWithApprovalFilter handles GET /api/v1/lounges/:id/staff with optional status filter
// This can be used to get staffData based on filter conditions like Query params: ?approval_status=pending|approved|declined
func (h *LoungeStaffHandler) GetStaffByLoungeWithApprovalFilter(c *gin.Context) {

	// Get user context from JWT middleware
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "User context not found",
		})
		return
	}

	// Extract route parameter
	loungeIDStr := c.Param("id")
	loungeID, err := uuid.Parse(loungeIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_lounge_id",
			Message: "Invalid lounge ID format",
		})
		return
	}

	// Get the lounge owner record for this user
    owner, err := h.loungeOwnerRepo.GetLoungeOwnerByUserID(userCtx.UserID)
    if err != nil || owner == nil {
        c.JSON(http.StatusUnauthorized, ErrorResponse{
            Error:   "unauthorized",
            Message: "Only lounge owners can view staff",
        })
        return
    }

	// Get the lounge
    lounge, err := h.loungeRepo.GetLoungeByID(loungeID)
    if err != nil {
        log.Printf("ERROR: Failed to get lounge %s: %v", loungeID, err)
        c.JSON(http.StatusInternalServerError, ErrorResponse{
            Error:   "database_error",
            Message: "Failed to retrieve lounge",
        })
        return
    }

    if lounge == nil {
        c.JSON(http.StatusNotFound, ErrorResponse{
            Error:   "not_found",
            Message: "Lounge not found",
        })
        return
    }

	// Compare lounge owner IDs 
    if lounge.LoungeOwnerID != owner.ID {
        c.JSON(http.StatusForbidden, ErrorResponse{
            Error:   "forbidden",
            Message: "You do not have permission to view staff for this lounge",
        })
        return
    }

	// Get optional approval status filter from query params
	approvalStatusParam := c.Query("approval_status")
	var approvalStatusFilter *string
	if approvalStatusParam != "" {
		validStatuses := map[string]bool{
			"pending":  true,
			"approved": true,
			"declined": true,
		}
		if !validStatuses[approvalStatusParam] {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "invalid_filter",
				Message: "Invalid approval status filter. Use: pending, approved, or declined",
			})
			return
		}
		approvalStatusFilter = &approvalStatusParam
	}

	// Get staff with optional filter
	staff, err := h.staffRepo.GetStaffByLoungeWithFilter(loungeID, approvalStatusFilter)
	if err != nil {
		log.Printf("ERROR: Failed to get staff for lounge %s: %v", loungeID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to retrieve staff",
		})
		return
	}

	// Helper function to extract nullable strings
	getNullableString := func(ns sql.NullString) *string {
		if ns.Valid {
			return &ns.String
		}
		return nil
	}

	// Convert to response format(i changed the format to match the repo)
	response := make([]gin.H, 0, len(staff))
	for _, s := range staff {
		response = append(response, gin.H{
			"id":                 s.ID,
			"lounge_id":          s.LoungeID,
			"first_name":         getNullableString(s.FullName),
			"nic_number":         getNullableString(s.NICNumber),
			"email":              getNullableString(s.Email),
			"approval_status":    s.ApprovalStatus,
			"employment_status":  s.EmploymentStatus,
			"created_at":         s.CreatedAt,
			"updated_at":         s.UpdatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"staff":  response,
		"total":  len(response),
		"filter": approvalStatusFilter,
	})

}

// GetBookingByReference handles GET /api/v1/lounge-staff/bookings/reference/:reference
// Allows approved lounge staff to lookup booking details by booking reference
func (h *LoungeStaffHandler) GetBookingByReference(c *gin.Context){

	// Get user context from JWT middleware
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "unauthorized",
			Message: "User context not found",
		})
		return
	}

	// Get booking reference from URL parameter
	bookingReference := c.Param("reference")
	if bookingReference == "" {
		c.JSON(http.StatusBadRequest,ErrorResponse{
			Error: "invalid_request",
			Message: "Booking reference is required",
		})
		return
	}

	// Verify user is approved and active lounge staff
	staff, err := h.loungeStaffRepo.GetApprovedStaffaByUserID(userCtx.UserID)
	if err != nil {
		log.Printf("ERROR: Failed to verify lounge staff for user %s: %v", userCtx.UserID, err)
		c.JSON(http.StatusInternalServerError,ErrorResponse{
			Error: "database_error",
			Message: "Failed to verify staff credentials",
		})
		return
	}

	if staff == nil {
		c.JSON(http.StatusForbidden,ErrorResponse{
			Error: "forbidden",
			Message: "Access denied. You must be an approved and active lounge staff member to access bookings." ,
		})
		return
	}

	// Get lounge details for logging/auditing
	lounge, err := h.loungeRepo.GetLoungeByID(staff.LoungeID)
	if err != nil {
		log.Printf("WARNING: Could not fetch lounge details for staff %s: %v", staff.ID, err)
		// Continue anyway - lounge info is just for logging
	}

	// Fetch booking by reference (using existing BookingRepository)
	// Note: You'll need to inject BookingRepository into LoungeStaffHandler
	booking, err := h.bookingRepo.GetByReference(bookingReference)
	if err != nil {
		log.Printf("ERROR: Failed to fetch booking %s: %v", bookingReference, err)
		c.JSON(http.StatusInternalServerError,ErrorResponse{
			Error: "database_error",
			Message: "Failed to retrieve booking",
		})
		return
	}

	if booking == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: "not_found",
			Message: "Booking not found",
		})
		return
	}

	// Log the access for auditing
	log.Printf("INFO: Lounge staff %s (Lounge: %s, Staff: %s) accessed booking %s",
		userCtx.UserID,
		staff.LoungeID,
		staff.FullName.String,
		bookingReference,
	)

	// Return booking data
	c.JSON(http.StatusOK, gin.H{
		"booking": booking,
		"accessed_by": gin.H{
			"staff_id":    staff.ID,
			"staff_name":  staff.FullName.String,
			"lounge_id":   staff.LoungeID,
			"lounge_name": func() string {
				if lounge != nil {
					return lounge.LoungeName
				}
				return ""
			}(),
		},
		"accessed_at": time.Now(),
	})
}

// GetLoungeBookingsForStaff handles GET /api/v1/lounge-staff/bookings
// Allows approved lounge staff to view all bookings for their assigned lounge
func (h *LoungeStaffHandler) GetLoungeBookingsForStaff(c *gin.Context) {
	// Get user context from JWT middleware
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "User context not found",
		})
		return
	}

	// Verify user is approved and active lounge staff
	staff, err := h.loungeStaffRepo.GetApprovedStaffaByUserID(userCtx.UserID)
	if err != nil {
		log.Printf("ERROR: Failed to verify lounge staff for user %s: %v", userCtx.UserID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to verify staff credentials",
		})
		return
	}

	if staff == nil {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Error:   "forbidden",
			Message: "Access denied. You must be an approved and active lounge staff member to access bookings.",
		})
		return
	}

	// Get pagination parameters
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	// Get optional filters
	statusQuery := strings.TrimSpace(c.Query("status"))
	var status *string
	if statusQuery != "" {
		status = &statusQuery
	}

	dateQuery := strings.TrimSpace(c.Query("date"))
	var bookingDate *string
	if dateQuery != "" {
		parsedDate, err := time.Parse("2006-01-02", dateQuery)
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "invalid_date",
				Message: "Invalid date format. Use YYYY-MM-DD",
			})
			return
		}
		formattedDate := parsedDate.Format("2006-01-02")
		bookingDate = &formattedDate
	}

	// Get bookings for the staff's assigned lounge
	bookings, err := h.loungeBookingRepo.GetLoungeBookingsByLoungeID(staff.LoungeID, status, bookingDate, limit, offset)
	if err != nil {
		log.Printf("ERROR: Failed to get lounge bookings for staff %s: %v", staff.ID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to retrieve bookings",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"bookings":  bookings,
		"lounge_id": staff.LoungeID,
		"limit":     limit,
		"offset":    offset,
	})
}

// ===================================================================
// UPDATE LOUNGE STAFF PROFILE
// ===================================================================

// UpdateLoungeStaffProfileRequest represents the profile update request
type UpdateLoungeStaffProfileRequest struct {
	FullName  *string `json:"full_name"`
	NICNumber *string `json:"nic_number"`
	Email     *string `json:"email"`
	Notes     *string `json:"notes"`
}

// UpdateProfile handles PUT /api/v1/lounge-staff/profile
func (h *LoungeStaffHandler) UpdateProfile(c *gin.Context) {
	// Get user context from JWT middleware
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "User context not found",
		})
		return
	}

	var req UpdateLoungeStaffProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "validation_error",
			Message: "Invalid request body: " + err.Error(),
		})
		return
	}

	// Get lounge staff record for this user
	staff, err := h.staffRepo.GetStaffByUserID(userCtx.UserID)
	if err != nil {
		log.Printf("ERROR: Failed to get lounge staff for user %s: %v", userCtx.UserID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to retrieve profile",
		})
		return
	}

	if staff == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "Lounge staff profile not found",
		})
		return
	}

	// Update profile with provided fields
	err = h.staffRepo.UpdateProfile(
		userCtx.UserID,
		req.FullName,
		req.NICNumber,
		req.Email,
		req.Notes,
	)
	if err != nil {
		log.Printf("ERROR: Failed to update profile for staff %s: %v", userCtx.UserID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "update_failed",
			Message: "Failed to update lounge staff profile",
		})
		return
	}

	// Fetch updated profile
	updatedStaff, err := h.staffRepo.GetStaffByUserID(userCtx.UserID)
	if err != nil {
		log.Printf("ERROR: Failed to fetch updated profile for staff %s: %v", userCtx.UserID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to retrieve updated profile",
		})
		return
	}

	// Helper function to extract values from sql.NullString
	getNullableString := func(ns sql.NullString) interface{} {
		if ns.Valid {
			return ns.String
		}
		return nil
	}

	// Helper function to extract values from sql.NullTime
	getNullableTime := func(nt sql.NullTime) interface{} {
		if nt.Valid {
			return nt.Time
		}
		return nil
	}

	log.Printf("INFO: Profile updated for lounge staff %s", userCtx.UserID)

	c.JSON(http.StatusOK, gin.H{
		"id":                 updatedStaff.ID,
		"lounge_id":          updatedStaff.LoungeID,
		"user_id":            updatedStaff.UserID,
		"full_name":          getNullableString(updatedStaff.FullName),
		"nic_number":         getNullableString(updatedStaff.NICNumber),
		"email":              getNullableString(updatedStaff.Email),
		"profile_completed":  updatedStaff.ProfileCompleted,
		"approval_status":    updatedStaff.ApprovalStatus,
		"employment_status":  updatedStaff.EmploymentStatus,
		"hired_date":         getNullableTime(updatedStaff.HiredDate),
		"terminated_date":    getNullableTime(updatedStaff.TerminatedDate),
		"notes":              getNullableString(updatedStaff.Notes),
		"created_at":         updatedStaff.CreatedAt,
		"updated_at":         updatedStaff.UpdatedAt,
	})
}

//     return nil
// }

// func getPointerFromNullTime(nt sql.NullTime) *time.Time {
//     if nt.Valid {
//         return &nt.Time
//     }
//     return nil
// }
