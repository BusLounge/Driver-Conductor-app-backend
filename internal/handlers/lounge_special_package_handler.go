package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/smarttransit/sms-auth-backend/internal/database"
	"github.com/smarttransit/sms-auth-backend/internal/middleware"
	"github.com/smarttransit/sms-auth-backend/internal/models"
)

// LoungeSpecialPackageHandler handles special package HTTP requests
type LoungeSpecialPackageHandler struct {
	pkgRepo         *database.LoungeSpecialPackageRepository
	loungeRepo      *database.LoungeRepository
	loungeOwnerRepo *database.LoungeOwnerRepository
}

// NewLoungeSpecialPackageHandler creates a new handler
func NewLoungeSpecialPackageHandler(
	pkgRepo *database.LoungeSpecialPackageRepository,
	loungeRepo *database.LoungeRepository,
	loungeOwnerRepo *database.LoungeOwnerRepository,
) *LoungeSpecialPackageHandler {
	return &LoungeSpecialPackageHandler{
		pkgRepo:         pkgRepo,
		loungeRepo:      loungeRepo,
		loungeOwnerRepo: loungeOwnerRepo,
	}
}

// ============================================================================
// REQUEST STRUCTS
// ============================================================================

// CreateSpecialPackageRequest is the request body for creating a package
type CreateSpecialPackageRequest struct {
	PackageName string  `json:"package_name" binding:"required"`
	ImageURL    *string `json:"image_url,omitempty"`
	PackageType string  `json:"package_type" binding:"required"`
	Description string  `json:"description"  binding:"required"`
	Price       string  `json:"price"        binding:"required"`

	// Extended fields
	Pax                *int64          `json:"pax,omitempty"`
	TransportStatus    *bool           `json:"transport_status,omitempty"`
	TransportMode      *string         `json:"transport_mode,omitempty"`
	MealStatus         *bool           `json:"meal_status,omitempty"`
	BreakfastStatus    *bool           `json:"breakfast_status,omitempty"`
	BreakfastType      json.RawMessage `json:"breakfast_type,omitempty"`
	LunchStatus        *bool           `json:"lunch_status,omitempty"`
	LunchType          json.RawMessage `json:"lunch_type,omitempty"`
	EveningSnackStatus *bool           `json:"evening_snack_status,omitempty"`
	EveningSnackType   json.RawMessage `json:"evening_snack_type,omitempty"`
	DinnerStatus       *bool           `json:"dinner_status,omitempty"`
	DinnerType         json.RawMessage `json:"dinner_type,omitempty"`
	Places             json.RawMessage `json:"places,omitempty"`
}

// UpdateSpecialPackageRequest is the request body for updating a package
type UpdateSpecialPackageRequest struct {
	PackageName string  `json:"package_name"`
	ImageURL    *string `json:"image_url,omitempty"`
	PackageType string  `json:"package_type"`
	Description string  `json:"description"`
	Price       string  `json:"price"`

	// Extended fields
	Pax                *int64          `json:"pax,omitempty"`
	TransportStatus    *bool           `json:"transport_status,omitempty"`
	TransportMode      *string         `json:"transport_mode,omitempty"`
	MealStatus         *bool           `json:"meal_status,omitempty"`
	BreakfastStatus    *bool           `json:"breakfast_status,omitempty"`
	BreakfastType      json.RawMessage `json:"breakfast_type,omitempty"`
	LunchStatus        *bool           `json:"lunch_status,omitempty"`
	LunchType          json.RawMessage `json:"lunch_type,omitempty"`
	EveningSnackStatus *bool           `json:"evening_snack_status,omitempty"`
	EveningSnackType   json.RawMessage `json:"evening_snack_type,omitempty"`
	DinnerStatus       *bool           `json:"dinner_status,omitempty"`
	DinnerType         json.RawMessage `json:"dinner_type,omitempty"`
	Places             json.RawMessage `json:"places,omitempty"`
}

// ============================================================================
// HELPERS
// ============================================================================

func isValidPackageType(t string) bool {
	switch models.LoungeSpecialPackageType(t) {
	case models.LoungeSpecialPackageTypePlatinum,
		models.LoungeSpecialPackageTypeGold,
		models.LoungeSpecialPackageTypeStandard:
		return true
	}
	return false
}

func isValidTransportMode(m string) bool {
	return models.IsValidTransportMode(m)
}

func (h *LoungeSpecialPackageHandler) verifyLoungeOwnership(c *gin.Context, loungeID uuid.UUID) (bool, error) {
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "unauthorized", Message: "User context not found"})
		return false, nil
	}

	owner, err := h.loungeOwnerRepo.GetLoungeOwnerByUserID(userCtx.UserID)
	if err != nil || owner == nil {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "forbidden", Message: "Not a lounge owner"})
		return false, nil
	}

	lounge, err := h.loungeRepo.GetLoungeByID(loungeID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "not_found", Message: "Lounge not found"})
			return false, nil
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "database_error", Message: "Failed to retrieve lounge"})
		return false, err
	}
	if lounge == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "not_found", Message: "Lounge not found"})
		return false, nil
	}

	if lounge.LoungeOwnerID != owner.ID {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "forbidden", Message: "You don't own this lounge"})
		return false, nil
	}

	return true, nil
}

func packageToResponse(pkg *models.LoungeSpecialPackage) gin.H {
	resp := gin.H{
		"id":           pkg.ID.String(),
		"lounge_id":    pkg.LoungeID.String(),
		"package_name": pkg.PackageName,
		"image_url":    pkg.ImageURL,
		"package_type": string(pkg.PackageType),
		"description":  pkg.Description,
		"price":        pkg.Price,
		"is_active":    pkg.IsActive,
		"created_at":   pkg.CreatedAt,
		"updated_at":   pkg.UpdatedAt,
		// Extended fields
		"pax":                  pkg.Pax,
		"transport_status":     pkg.TransportStatus,
		"transport_mode":       pkg.TransportMode,
		"meal_status":          pkg.MealStatus,
		"breakfast_status":     pkg.BreakfastStatus,
		"breakfast_type":       pkg.BreakfastType,
		"lunch_status":         pkg.LunchStatus,
		"lunch_type":           pkg.LunchType,
		"evening_snack_status": pkg.EveningSnackStatus,
		"evening_snack_type":   pkg.EveningSnackType,
		"dinner_status":        pkg.DinnerStatus,
		"dinner_type":          pkg.DinnerType,
		"places":               pkg.Places,
	}
	return resp
}

// applyExtendedFields copies extended request fields into the model
func applyExtendedFields(pkg *models.LoungeSpecialPackage,
	pax *int64,
	transportStatus *bool, transportMode *string,
	mealStatus *bool,
	breakfastStatus *bool, breakfastType json.RawMessage,
	lunchStatus *bool, lunchType json.RawMessage,
	eveningSnackStatus *bool, eveningSnackType json.RawMessage,
	dinnerStatus *bool, dinnerType json.RawMessage,
	places json.RawMessage,
) {
	pkg.Pax = pax
	pkg.TransportStatus = transportStatus
	pkg.TransportMode = transportMode
	pkg.MealStatus = mealStatus
	pkg.BreakfastStatus = breakfastStatus
	pkg.BreakfastType = breakfastType
	pkg.LunchStatus = lunchStatus
	pkg.LunchType = lunchType
	pkg.EveningSnackStatus = eveningSnackStatus
	pkg.EveningSnackType = eveningSnackType
	pkg.DinnerStatus = dinnerStatus
	pkg.DinnerType = dinnerType
	pkg.Places = places
}

// ============================================================================
// HANDLERS
// ============================================================================

// GetSpecialPackages handles GET /api/v1/lounges/:id/special-packages
func (h *LoungeSpecialPackageHandler) GetSpecialPackages(c *gin.Context) {
	loungeIDStr := c.Param("id")
	loungeID, err := uuid.Parse(loungeIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid_id", Message: "Invalid lounge ID format"})
		return
	}

	packages, err := h.pkgRepo.GetSpecialPackagesByLoungeID(loungeID)
	if err != nil {
		log.Printf("ERROR: Failed to get special packages for lounge %s: %v", loungeID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "database_error", Message: "Failed to retrieve special packages"})
		return
	}

	// Convert to response slice
	response := make([]gin.H, 0, len(packages))
	for i := range packages {
		response = append(response, packageToResponse(&packages[i]))
	}

	c.JSON(http.StatusOK, gin.H{
		"special_packages": response,
		"lounge_id":        loungeID,
		"total":            len(response),
	})
}

// CreateSpecialPackage handles POST /api/v1/lounges/:id/special-packages
func (h *LoungeSpecialPackageHandler) CreateSpecialPackage(c *gin.Context) {
	loungeIDStr := c.Param("id")
	loungeID, err := uuid.Parse(loungeIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid_id", Message: "Invalid lounge ID format"})
		return
	}

	ok, _ := h.verifyLoungeOwnership(c, loungeID)
	if !ok {
		return
	}

	var req CreateSpecialPackageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "validation_error", Message: "Invalid request body: " + err.Error()})
		return
	}

	if !isValidPackageType(req.PackageType) {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "validation_error",
			Message: "Invalid package_type. Must be one of: platinum, gold, standard",
		})
		return
	}

	// Validate transport mode if transport is enabled
	if req.TransportStatus != nil && *req.TransportStatus && req.TransportMode != nil {
		if !isValidTransportMode(*req.TransportMode) {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "validation_error",
				Message: "Invalid transport_mode. Must be one of: three-wheeler, van, car",
			})
			return
		}
	}

	pkg := &models.LoungeSpecialPackage{
		LoungeID:    loungeID,
		PackageName: req.PackageName,
		ImageURL:    req.ImageURL,
		PackageType: models.LoungeSpecialPackageType(req.PackageType),
		Description: req.Description,
		Price:       req.Price,
	}

	applyExtendedFields(pkg,
		req.Pax,
		req.TransportStatus, req.TransportMode,
		req.MealStatus,
		req.BreakfastStatus, req.BreakfastType,
		req.LunchStatus, req.LunchType,
		req.EveningSnackStatus, req.EveningSnackType,
		req.DinnerStatus, req.DinnerType,
		req.Places,
	)

	if err := h.pkgRepo.CreateSpecialPackage(pkg); err != nil {
		log.Printf("ERROR: Failed to create special package: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "creation_failed", Message: "Failed to create special package"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":         "Special package created successfully",
		"special_package": packageToResponse(pkg),
	})
}

// UpdateSpecialPackage handles PUT /api/v1/lounges/:id/special-packages/:package_id
func (h *LoungeSpecialPackageHandler) UpdateSpecialPackage(c *gin.Context) {
	loungeIDStr := c.Param("id")
	loungeID, err := uuid.Parse(loungeIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid_id", Message: "Invalid lounge ID format"})
		return
	}

	pkgIDStr := c.Param("package_id")
	pkgID, err := uuid.Parse(pkgIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid_id", Message: "Invalid package ID format"})
		return
	}

	ok, _ := h.verifyLoungeOwnership(c, loungeID)
	if !ok {
		return
	}

	// Fetch existing package
	pkg, err := h.pkgRepo.GetSpecialPackageByID(pkgID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "not_found", Message: "Special package not found"})
		return
	}

	if pkg.LoungeID != loungeID {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "forbidden", Message: "Package doesn't belong to this lounge"})
		return
	}

	var req UpdateSpecialPackageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "validation_error", Message: "Invalid request body: " + err.Error()})
		return
	}

	// Apply base field updates
	if req.PackageName != "" {
		pkg.PackageName = req.PackageName
	}
	if req.ImageURL != nil {
		pkg.ImageURL = req.ImageURL
	}
	if req.PackageType != "" {
		if !isValidPackageType(req.PackageType) {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "validation_error",
				Message: "Invalid package_type. Must be one of: platinum, gold, standard",
			})
			return
		}
		pkg.PackageType = models.LoungeSpecialPackageType(req.PackageType)
	}
	if req.Description != "" {
		pkg.Description = req.Description
	}
	if req.Price != "" {
		pkg.Price = req.Price
	}

	// Validate transport mode if provided and enabled
	if req.TransportStatus != nil && *req.TransportStatus && req.TransportMode != nil {
		if !isValidTransportMode(*req.TransportMode) {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "validation_error",
				Message: "Invalid transport_mode. Must be one of: three-wheeler, van, car",
			})
			return
		}
	}

	// Apply extended fields (always overwrite with whatever was sent)
	applyExtendedFields(pkg,
		req.Pax,
		req.TransportStatus, req.TransportMode,
		req.MealStatus,
		req.BreakfastStatus, req.BreakfastType,
		req.LunchStatus, req.LunchType,
		req.EveningSnackStatus, req.EveningSnackType,
		req.DinnerStatus, req.DinnerType,
		req.Places,
	)

	if err := h.pkgRepo.UpdateSpecialPackage(pkg); err != nil {
		log.Printf("ERROR: Failed to update special package %s: %v", pkgID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "update_failed", Message: "Failed to update special package"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":         "Special package updated successfully",
		"special_package": packageToResponse(pkg),
	})
}

// DeleteSpecialPackage handles DELETE /api/v1/lounges/:id/special-packages/:package_id
func (h *LoungeSpecialPackageHandler) DeleteSpecialPackage(c *gin.Context) {
	loungeIDStr := c.Param("id")
	loungeID, err := uuid.Parse(loungeIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid_id", Message: "Invalid lounge ID format"})
		return
	}

	pkgIDStr := c.Param("package_id")
	pkgID, err := uuid.Parse(pkgIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid_id", Message: "Invalid package ID format"})
		return
	}

	ok, _ := h.verifyLoungeOwnership(c, loungeID)
	if !ok {
		return
	}

	// Verify package belongs to this lounge
	pkg, err := h.pkgRepo.GetSpecialPackageByID(pkgID)
	if err != nil || pkg == nil || pkg.LoungeID != loungeID {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "not_found", Message: "Special package not found"})
		return
	}

	if err := h.pkgRepo.DeleteSpecialPackage(pkgID); err != nil {
		log.Printf("ERROR: Failed to delete special package %s: %v", pkgID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "delete_failed", Message: "Failed to delete special package"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Special package deleted successfully"})
}
