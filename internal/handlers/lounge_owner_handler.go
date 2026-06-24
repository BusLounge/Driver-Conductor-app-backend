package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/smarttransit/sms-auth-backend/internal/database"
	"github.com/smarttransit/sms-auth-backend/internal/middleware"
	"github.com/smarttransit/sms-auth-backend/internal/models"
)

// LoungeOwnerHandler handles lounge owner-related HTTP requests
type LoungeOwnerHandler struct {
	loungeOwnerRepo *database.LoungeOwnerRepository
	userRepo        *database.UserRepository
	loungeRepo      *database.LoungeRepository
}

// NewLoungeOwnerHandler creates a new lounge owner handler
func NewLoungeOwnerHandler(
	loungeOwnerRepo *database.LoungeOwnerRepository,
	userRepo *database.UserRepository,
	loungeRepo *database.LoungeRepository,
) *LoungeOwnerHandler {
	return &LoungeOwnerHandler{
		loungeOwnerRepo: loungeOwnerRepo,
		userRepo:        userRepo,
		loungeRepo:      loungeRepo,
	}
}

// ===================================================================
// STEP 1: Save Business and Manager Information
// ===================================================================

// SaveBusinessAndManagerInfoRequest represents the business/manager info request
// NIC images are optional - can be uploaded here or later for admin review
type SaveBusinessAndManagerInfoRequest struct {
	BusinessName       string  `json:"business_name" binding:"required"`
	BusinessLicense    *string `json:"business_license"`
	ManagerFullName    string  `json:"manager_full_name" binding:"required"`
	ManagerNICNumber   string  `json:"manager_nic_number" binding:"required"`
	ManagerEmail       *string `json:"manager_email"`
	DistrictID         *string `json:"district_id" binding:"required"` // UUID string that will be parsed and validated
	ManagerNICFrontURL *string `json:"manager_nic_front_url"`          // Optional: NIC front image URL from Supabase
	ManagerNICBackURL  *string `json:"manager_nic_back_url"`           // Optional: NIC back image URL from Supabase
}

// SaveBusinessAndManagerInfo handles POST /api/v1/lounge-owner/register/business-info
func (h *LoungeOwnerHandler) SaveBusinessAndManagerInfo(c *gin.Context) {
	// Get user context from JWT middleware
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "User context not found",
		})
		return
	}

	var req SaveBusinessAndManagerInfoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "validation_error",
			Message: "Invalid request body: " + err.Error(),
		})
		return
	}

	// Get lounge owner record
	owner, err := h.loungeOwnerRepo.GetLoungeOwnerByUserID(userCtx.UserID)
	if err != nil {
		log.Printf("ERROR: Failed to get lounge owner for user %s: %v", userCtx.UserID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to retrieve lounge owner",
		})
		return
	}

	if owner == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "Lounge owner record not found",
		})
		return
	}

	// Update business and manager info (including optional NIC images)
	businessLicenseVal := ""
	if req.BusinessLicense != nil {
		businessLicenseVal = *req.BusinessLicense
	}

	// Validate and parse district_id
	var districtUUID *uuid.UUID
	if req.DistrictID != nil {
		parsedUUID, err := uuid.Parse(*req.DistrictID)
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "invalid_district_id",
				Message: "Invalid district_id format. Must be a valid UUID (e.g., 550e8400-e29b-41d4-a716-446655440000)",
			})
			return
		}
		districtUUID = &parsedUUID
	}

	err = h.loungeOwnerRepo.UpdateBusinessAndManagerInfoWithNIC(
		userCtx.UserID,
		req.BusinessName,
		businessLicenseVal,
		req.ManagerFullName,
		req.ManagerNICNumber,
		req.ManagerEmail,
		districtUUID,
		req.ManagerNICFrontURL,
		req.ManagerNICBackURL,
	)
	if err != nil {
		log.Printf("ERROR: Failed to update business/manager info for user %s: %v", userCtx.UserID, err)

		// Check if it's a duplicate key error
		errMsg := err.Error()
		if strings.Contains(errMsg, "duplicate key") && strings.Contains(errMsg, "business_license") {
			c.JSON(http.StatusConflict, ErrorResponse{
				Error:   "duplicate_business_license",
				Message: "This business license number is already registered. Please use a different license number or contact support if you believe this is an error.",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "update_failed",
			Message: "Failed to save business and manager information",
		})
		return
	}

	log.Printf("INFO: Business and manager info saved for lounge owner %s (step: business_info)", userCtx.UserID)

	c.JSON(http.StatusOK, gin.H{
		"message":           "Business and manager information saved successfully",
		"registration_step": models.RegStepBusinessInfo,
	})
}

// ===================================================================
// DEPRECATED: STEP 2: Upload Manager NIC Images
// ===================================================================
// This endpoint is DEPRECATED and kept for backward compatibility only.
// NIC images should now be uploaded as part of business_info step.
// This endpoint is no longer used in the new registration flow.

// UploadManagerNICRequest represents the manager NIC upload request
type UploadManagerNICRequest struct {
	ManagerNICFrontURL string `json:"manager_nic_front_url" binding:"required"` // Uploaded to Supabase
	ManagerNICBackURL  string `json:"manager_nic_back_url" binding:"required"`  // Uploaded to Supabase
}

// UploadManagerNIC handles POST /api/v1/lounge-owner/register/upload-manager-nic
// DEPRECATED: Use SaveBusinessAndManagerInfo with NIC URLs instead
func (h *LoungeOwnerHandler) UploadManagerNIC(c *gin.Context) {
	c.JSON(http.StatusGone, ErrorResponse{
		Error:   "deprecated_endpoint",
		Message: "This endpoint is deprecated. Please include NIC images in the business-info step.",
	})
}

// ===================================================================
// GET REGISTRATION PROGRESS
// ===================================================================

// GetRegistrationProgress handles GET /api/v1/lounge-owner/registration/progress
func (h *LoungeOwnerHandler) GetRegistrationProgress(c *gin.Context) {
	// Get user context from JWT middleware
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "User context not found",
		})
		return
	}

	// Get lounge owner record
	owner, err := h.loungeOwnerRepo.GetLoungeOwnerByUserID(userCtx.UserID)
	if err != nil {
		log.Printf("ERROR: Failed to get lounge owner for user %s: %v", userCtx.UserID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to retrieve registration progress",
		})
		return
	}

	if owner == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "Lounge owner record not found",
		})
		return
	}

	// Get dynamic counts
	loungeCount, _ := h.loungeOwnerRepo.GetLoungeCount(owner.ID)
	staffCount, _ := h.loungeOwnerRepo.GetStaffCount(owner.ID)

	response := gin.H{
		"registration_step":   owner.RegistrationStep,
		"profile_completed":   owner.ProfileCompleted,
		"verification_status": owner.VerificationStatus,
		"total_lounges":       loungeCount,
		"total_staff":         staffCount,
	}

	// Add step completion status (new flow: phone_verified -> business_info -> lounge_added -> completed)
	response["steps"] = gin.H{
		"phone_verified": true, // Always true if they have a record
		"business_info":  owner.RegistrationStep == models.RegStepBusinessInfo || owner.RegistrationStep == models.RegStepLoungeAdded || owner.RegistrationStep == models.RegStepCompleted,
		"lounge_added":   owner.RegistrationStep == models.RegStepLoungeAdded || owner.RegistrationStep == models.RegStepCompleted,
		"completed":      owner.RegistrationStep == models.RegStepCompleted,
	}

	// If completed but pending approval, add pending_approval flag
	if owner.RegistrationStep == models.RegStepCompleted && owner.VerificationStatus == models.LoungeVerificationPending {
		response["pending_approval"] = true
		response["approval_message"] = "Your registration is complete and awaiting admin approval"
	} else if owner.VerificationStatus == models.LoungeVerificationApproved {
		response["approved"] = true
		response["approval_message"] = "Your account has been approved"
	} else if owner.VerificationStatus == models.LoungeVerificationRejected {
		response["rejected"] = true
		response["approval_message"] = "Your account has been rejected"
		if owner.VerificationNotes.Valid {
			response["rejection_reason"] = owner.VerificationNotes.String
		}
	}

	c.JSON(http.StatusOK, response)
}

// ===================================================================
// GET LOUNGE OWNER PROFILE
// ===================================================================

// GetProfile handles GET /api/v1/lounge-owner/profile
func (h *LoungeOwnerHandler) GetProfile(c *gin.Context) {
	// Get user context from JWT middleware
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "User context not found",
		})
		return
	}

	// Get lounge owner record
	owner, err := h.loungeOwnerRepo.GetLoungeOwnerByUserID(userCtx.UserID)
	if err != nil {
		log.Printf("ERROR: Failed to get lounge owner for user %s: %v", userCtx.UserID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to retrieve profile",
		})
		return
	}

	if owner == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "Lounge owner profile not found",
		})
		return
	}

	// Get user record to fetch phone number
	user, err := h.userRepo.GetUserByID(userCtx.UserID)
	if err != nil {
		log.Printf("ERROR: Failed to get user for owner %s: %v", userCtx.UserID, err)
		// Continue anyway, phone is optional in response
	}

	// 🔍 DEBUG: Log what database returns
	log.Printf("🔍 GET PROFILE - User: %s, RegistrationStep: %s, ProfileCompleted: %t",
		userCtx.UserID, owner.RegistrationStep, owner.ProfileCompleted)

	// Get dynamic counts
	loungeCount, _ := h.loungeOwnerRepo.GetLoungeCount(owner.ID)
	staffCount, _ := h.loungeOwnerRepo.GetStaffCount(owner.ID)

	// Helper functions to extract values from sql.Null* types
	getNullableString := func(ns sql.NullString) interface{} {
		if ns.Valid {
			return ns.String
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

	managerEmail := owner.ManagerEmail
	if !managerEmail.Valid && owner.Email.Valid {
		managerEmail = owner.Email
	}

	c.JSON(http.StatusOK, gin.H{
		"id":      owner.ID,
		"user_id": owner.UserID,
		"phone": func() *string {
			if user != nil {
				return &user.Phone
			}
			return nil
		}(),
		"business_name":       getNullableString(owner.BusinessName),
		"business_license":    getNullableString(owner.BusinessLicense),
		"manager_full_name":   getNullableString(owner.ManagerFullName),
		"manager_nic_number":  getNullableString(owner.ManagerNICNumber),
		"manager_email":       getNullableString(managerEmail),
		"district_id":         owner.DistrictID,
		"registration_step":   owner.RegistrationStep,
		"profile_completed":   owner.ProfileCompleted,
		"verification_status": owner.VerificationStatus,
		"verification_notes":  getNullableString(owner.VerificationNotes),
		"verified_at":         getNullableTime(owner.VerifiedAt),
		"total_lounges":       loungeCount,
		"total_staff":         staffCount,
		"created_at":          owner.CreatedAt,
		"updated_at":          owner.UpdatedAt,
	})
}

// PUBLIC HANDLERS WHICH IS USED TO SEND LOUNGE OWNER AND LOUNGE DATA TO FRONTEND

// get approved lounge owners
func (h *LoungeOwnerHandler) GetApprovedLoungeOwners(c *gin.Context) {

	// Get approved lounge owners
	owners, err := h.loungeOwnerRepo.GetApprovedLoungeOwners()
	if err != nil {
		log.Printf("ERROR: Failed to get approved lounge owners: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to retrieve lounge owners",
		})
		return
	}

	// Helper function to extract values from sql.NullString types
	getNullableString := func(ns sql.NullString) *string {
		if ns.Valid {
			return &ns.String
		}
		return nil
	}

	// Convert to response format (only public info)
	response := make([]gin.H, 0, len(owners))
	for _, owner := range owners {
		// Get lounge count for this owner
		loungeCount, _ := h.loungeOwnerRepo.GetLoungeCount(owner.ID)

		response = append(response, gin.H{
			"id":            owner.ID,
			"business_name": getNullableString(owner.BusinessName),
			"manager_name":  getNullableString(owner.ManagerFullName),
			"total_lounges": loungeCount,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"lounge_owners": response,
	})
}

// GetApprovedLoungeOwnersByDistrict handles GET /api/v1/lounge-owner/approved/by-district/{district_id}
func (h *LoungeOwnerHandler) GetApprovedLoungeOwnersByDistrict(c *gin.Context) {
	districtIDStr := c.Param("district_id")
	if districtIDStr == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "validation_error",
			Message: "district_id is required",
		})
		return
	}

	districtID, err := uuid.Parse(districtIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_district_id",
			Message: "Invalid district_id format. Must be a valid UUID",
		})
		return
	}

	owners, err := h.loungeOwnerRepo.GetApprovedLoungeOwnersByDistrictID(districtID)
	if err != nil {
		log.Printf("ERROR: Failed to get approved lounge owners for district %s: %v", districtID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to retrieve lounge owners",
		})
		return
	}

	getNullableString := func(ns sql.NullString) *string {
		if ns.Valid {
			return &ns.String
		}
		return nil
	}

	ownerIDs := make([]uuid.UUID, 0, len(owners))
	for _, owner := range owners {
		ownerIDs = append(ownerIDs, owner.ID)
	}

	loungeCounts, err := h.loungeOwnerRepo.GetLoungeCountsByOwnerIDs(ownerIDs)
	if err != nil {
		log.Printf("ERROR: Failed to get lounge counts for district %s: %v", districtID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to retrieve lounge owners",
		})
		return
	}

	response := make([]gin.H, 0, len(owners))
	for _, owner := range owners {
		response = append(response, gin.H{
			"id":                  owner.ID,
			"user_id":             owner.UserID,
			"business_name":       getNullableString(owner.BusinessName),
			"business_license":    getNullableString(owner.BusinessLicense),
			"manager_name":        getNullableString(owner.ManagerFullName),
			"manager_email":       getNullableString(owner.ManagerEmail),
			"district_id":         owner.DistrictID,
			"total_lounges":       loungeCounts[owner.ID],
			"verification_status": owner.VerificationStatus,
			"registration_step":   owner.RegistrationStep,
			"profile_completed":   owner.ProfileCompleted,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"district_id":   districtID,
		"count":         len(response),
		"lounge_owners": response,
	})
}

// get lounges by ownerID
func (h *LoungeOwnerHandler) GetLoungesByOwnerID(c *gin.Context) {

	// Get owner ID from URL
	ownerIDStr := c.Param("owner_id")
	ownerID, err := uuid.Parse(ownerIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_owner_id",
			Message: "Invalid lounge owner ID format",
		})
		return
	}

	// Verify owner exists and is approved
	owner, err := h.loungeOwnerRepo.GetLoungeOwnerByID(ownerID)
	if err != nil {
		log.Printf("ERROR: Failed to get lounge owner %s: %v", ownerID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to retrieve lounge owner",
		})
		return
	}

	if owner == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "Lounge owner not found",
		})
		return
	}

	if owner.VerificationStatus != models.LoungeVerificationApproved {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "Lounge owner not found",
		})
		return
	}

	// Get lounges for this owner
	lounges, err := h.loungeRepo.GetLoungesByOwnerID(ownerID)
	if err != nil {
		log.Printf("ERROR: Failed to get lounges for owner %s: %v", ownerID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to retrieve lounges",
		})
		return
	}

	// Convert to response format
	response := make([]gin.H, 0, len(lounges))
	for _, lounge := range lounges {
		var districtValue interface{} = nil
		if lounge.District != nil {
			districtValue = lounge.District.String()
		}
		response = append(response, gin.H{
			"id":          lounge.ID,
			"lounge_name": lounge.LoungeName,
			"address":     lounge.Address,
			"district":    districtValue,
			"district_id": districtValue,
			"status":      lounge.Status,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"lounges": response,
	})

}

// GetLoungesByOwnerAndDistrict handles GET /api/v1/lounge-owner/{owner_id}/lounges/by-district/{district_id}
func (h *LoungeOwnerHandler) GetLoungesByOwnerAndDistrict(c *gin.Context) {
	ownerIDStr := c.Param("owner_id")
	ownerID, err := uuid.Parse(ownerIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_owner_id",
			Message: "Invalid lounge owner ID format",
		})
		return
	}

	districtIDStr := c.Param("district_id")
	districtID, err := uuid.Parse(districtIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_district_id",
			Message: "Invalid district ID format",
		})
		return
	}

	owner, err := h.loungeOwnerRepo.GetLoungeOwnerByID(ownerID)
	if err != nil {
		log.Printf("ERROR: Failed to get lounge owner %s: %v", ownerID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to retrieve lounge owner",
		})
		return
	}

	if owner == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "Lounge owner not found",
		})
		return
	}

	if owner.VerificationStatus != models.LoungeVerificationApproved {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "Lounge owner not found",
		})
		return
	}

	lounges, err := h.loungeRepo.GetLoungesByOwnerAndDistrict(ownerID, districtID)
	if err != nil {
		log.Printf("ERROR: Failed to get lounges for owner %s and district %s: %v", ownerID, districtID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to retrieve lounges",
		})
		return
	}

	response := make([]gin.H, 0, len(lounges))
	for _, lounge := range lounges {
		var districtValue interface{} = nil
		if lounge.District != nil {
			districtValue = lounge.District.String()
		}

		// Parse amenities from JSONB
		var amenities []string
		if lounge.Amenities != nil {
			json.Unmarshal(lounge.Amenities, &amenities)
		}

		// Parse images from JSONB
		var images []string
		if lounge.Images != nil {
			json.Unmarshal(lounge.Images, &images)
		}

		// Parse nullable string fields
		var description interface{} = nil
		if lounge.Description.Valid {
			description = lounge.Description.String
		}

		var contactPhone interface{} = nil
		if lounge.ContactPhone.Valid {
			contactPhone = lounge.ContactPhone.String
		}

		var capacity interface{} = nil
		if lounge.Capacity.Valid {
			capacity = lounge.Capacity.Int64
		}

		var latitude interface{} = nil
		if lounge.Latitude.Valid {
			latitude = lounge.Latitude.String
		}

		var longitude interface{} = nil
		if lounge.Longitude.Valid {
			longitude = lounge.Longitude.String
		}

		var price1Hour interface{} = nil
		if lounge.Price1Hour.Valid {
			price1Hour = lounge.Price1Hour.String
		}

		var price2Hours interface{} = nil
		if lounge.Price2Hours.Valid {
			price2Hours = lounge.Price2Hours.String
		}

		var price3Hours interface{} = nil
		if lounge.Price3Hours.Valid {
			price3Hours = lounge.Price3Hours.String
		}

		var priceUntilBus interface{} = nil
		if lounge.PriceUntilBus.Valid {
			priceUntilBus = lounge.PriceUntilBus.String
		}

		var averageRating interface{} = nil
		if lounge.AverageRating.Valid {
			averageRating = lounge.AverageRating.String
		}

		response = append(response, gin.H{
			"id":              lounge.ID,
			"lounge_owner_id": lounge.LoungeOwnerID,
			"lounge_name":     lounge.LoungeName,
			"description":     description,
			"address":         lounge.Address,
			"district":        districtValue,
			"district_id":     districtValue,
			"contact_phone":   contactPhone,
			"capacity":        capacity,
			"latitude":        latitude,
			"longitude":       longitude,
			"price_1_hour":    price1Hour,
			"price_2_hours":   price2Hours,
			"price_3_hours":   price3Hours,
			"price_until_bus": priceUntilBus,
			"amenities":       amenities,
			"images":          images,
			"status":          lounge.Status,
			"is_operational":  lounge.IsOperational,
			"average_rating":  averageRating,
			"created_at":      lounge.CreatedAt,
			"updated_at":      lounge.UpdatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"owner_id":    ownerID,
		"district_id": districtID,
		"count":       len(response),
		"lounges":     response,
	})
}

func (h *LoungeOwnerHandler) GetApprovedLoungeOwnersByDsitrict(c *gin.Context) {

	// get approved lounge owners grouped by district
	districtGroups, err := h.loungeOwnerRepo.GetApprovedLoungeOwnersByDistrict()
	if err != nil {
		log.Printf("ERROR: Failed to get approved lounge owners by district: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to retrieve lounge owners",
		})
		return
	}

	// Helper function to extract values from sql.NullString types
	getNullableString := func(ns sql.NullString) *string {
		if ns.Valid {
			return &ns.String
		}
		return nil
	}

	ownerIDs := make([]uuid.UUID, 0)
	for _, owners := range districtGroups {
		for _, owner := range owners {
			ownerIDs = append(ownerIDs, owner.ID)
		}
	}

	loungeCounts, err := h.loungeOwnerRepo.GetLoungeCountsByOwnerIDs(ownerIDs)
	if err != nil {
		log.Printf("ERROR: Failed to get lounge counts for approved owners: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to retrieve lounge owners",
		})
		return
	}

	// Convert to response format grouped by district
	response := make(map[string][]gin.H)
	for district, owners := range districtGroups {
		response[district] = make([]gin.H, 0, len(owners))
		for _, owner := range owners {
			loungeCount := loungeCounts[owner.ID]

			response[district] = append(response[district], gin.H{
				"id":            owner.ID,
				"business_name": getNullableString(owner.BusinessName),
				"manager_name":  getNullableString(owner.ManagerFullName),
				"total_lounges": loungeCount,
				"district_id":   district,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"lounge_owners_by_district": response,
	})

}

// ===================================================================
// UPDATE LOUNGE OWNER PROFILE
// ===================================================================

// UpdateLoungeOwnerProfileRequest represents the profile update request
type UpdateLoungeOwnerProfileRequest struct {
	BusinessName     *string `json:"business_name"`
	BusinessLicense  *string `json:"business_license"`
	ManagerFullName  *string `json:"manager_full_name"`
	ManagerNICNumber *string `json:"manager_nic_number"`
	ManagerEmail     *string `json:"manager_email"`
	DistrictID       *string `json:"district_id"` // UUID string that will be parsed and validated
}

// UpdateProfile handles PUT /api/v1/lounge-owner/profile
func (h *LoungeOwnerHandler) UpdateProfile(c *gin.Context) {
	// Get user context from JWT middleware
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "User context not found",
		})
		return
	}

	var req UpdateLoungeOwnerProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "validation_error",
			Message: "Invalid request body: " + err.Error(),
		})
		return
	}

	// Get lounge owner record
	owner, err := h.loungeOwnerRepo.GetLoungeOwnerByUserID(userCtx.UserID)
	if err != nil {
		log.Printf("ERROR: Failed to get lounge owner for user %s: %v", userCtx.UserID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to retrieve lounge owner",
		})
		return
	}

	if owner == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "Lounge owner profile not found",
		})
		return
	}

	// Update profile with provided fields
	var districtUUID *uuid.UUID
	if req.DistrictID != nil {
		parsedUUID, err := uuid.Parse(*req.DistrictID)
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "invalid_district_id",
				Message: "Invalid district_id format. Must be a valid UUID (e.g., 550e8400-e29b-41d4-a716-446655440000)",
			})
			return
		}
		districtUUID = &parsedUUID
	}

	err = h.loungeOwnerRepo.UpdateProfile(
		userCtx.UserID,
		req.BusinessName,
		req.BusinessLicense,
		req.ManagerFullName,
		req.ManagerNICNumber,
		req.ManagerEmail,
		districtUUID,
	)
	if err != nil {
		log.Printf("ERROR: Failed to update profile for user %s: %v", userCtx.UserID, err)

		// Check if it's a duplicate key error
		errMsg := err.Error()
		if strings.Contains(errMsg, "duplicate key") && strings.Contains(errMsg, "business_license") {
			c.JSON(http.StatusConflict, ErrorResponse{
				Error:   "duplicate_business_license",
				Message: "This business license number is already registered. Please use a different license number.",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "update_failed",
			Message: "Failed to update lounge owner profile",
		})
		return
	}

	// Fetch updated profile
	updatedOwner, err := h.loungeOwnerRepo.GetLoungeOwnerByUserID(userCtx.UserID)
	if err != nil {
		log.Printf("ERROR: Failed to fetch updated profile for user %s: %v", userCtx.UserID, err)
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

	log.Printf("INFO: Profile updated for lounge owner %s", userCtx.UserID)

	c.JSON(http.StatusOK, gin.H{
		"id":                  updatedOwner.ID,
		"user_id":             updatedOwner.UserID,
		"business_name":       getNullableString(updatedOwner.BusinessName),
		"business_license":    getNullableString(updatedOwner.BusinessLicense),
		"manager_full_name":   getNullableString(updatedOwner.ManagerFullName),
		"manager_nic_number":  getNullableString(updatedOwner.ManagerNICNumber),
		"manager_email":       getNullableString(updatedOwner.ManagerEmail),
		"district_id":         updatedOwner.DistrictID,
		"registration_step":   updatedOwner.RegistrationStep,
		"profile_completed":   updatedOwner.ProfileCompleted,
		"verification_status": updatedOwner.VerificationStatus,
		"verification_notes":  getNullableString(updatedOwner.VerificationNotes),
		"verified_at":         getNullableTime(updatedOwner.VerifiedAt),
		"created_at":          updatedOwner.CreatedAt,
		"updated_at":          updatedOwner.UpdatedAt,
	})
}
