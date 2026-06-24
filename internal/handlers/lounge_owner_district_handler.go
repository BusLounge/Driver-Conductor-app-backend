package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/smarttransit/sms-auth-backend/internal/database"
	"github.com/smarttransit/sms-auth-backend/internal/models"
)

// LoungeOwnerDistrictHandler handles lounge owner district endpoints.
type LoungeOwnerDistrictHandler struct {
	repo *database.LoungeOwnerDistrictRepository
}

// NewLoungeOwnerDistrictHandler creates a new handler instance.
func NewLoungeOwnerDistrictHandler(repo *database.LoungeOwnerDistrictRepository) *LoungeOwnerDistrictHandler {
	return &LoungeOwnerDistrictHandler{repo: repo}
}

// CreateLoungeOwnerDistrictRequest represents create payload.
type CreateLoungeOwnerDistrictRequest struct {
	OwnerID      string `json:"owner_id" binding:"required"`
	DistrictID   string `json:"district_id" binding:"required"`
	OwnerName    string `json:"owner_name" binding:"required"`
	BusinessName string `json:"business_name" binding:"required"`
}

// LoungeOwnerDistrictFetchItem is the response item shape for district search results.
type LoungeOwnerDistrictFetchItem struct {
	OwnerID      uuid.UUID `json:"owner_id"`
	DistrictID   uuid.UUID `json:"district_id"`
	OwnerName    string    `json:"owner_name"`
	BusinessName string    `json:"business_name"`
}

// CheckLoungeOwnerDistrictRequest represents payload for pair existence checks.
type CheckLoungeOwnerDistrictRequest struct {
	OwnerID    string `json:"owner_id" binding:"required"`
	DistrictID string `json:"district_id" binding:"required"`
}

// Create handles POST /api/v1/lounge-owner-districts
func (h *LoungeOwnerDistrictHandler) Create(c *gin.Context) {
	var req CreateLoungeOwnerDistrictRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "validation_error",
			Message: "Invalid request body: " + err.Error(),
		})
		return
	}

	ownerID, err := uuid.Parse(req.OwnerID)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_owner_id",
			Message: "owner_id must be a valid UUID",
		})
		return
	}

	districtID, err := uuid.Parse(req.DistrictID)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_district_id",
			Message: "district_id must be a valid UUID",
		})
		return
	}

	record := &models.LoungeOwnerDistrict{
		ID:           uuid.New(),
		OwnerID:      ownerID,
		DistrictID:   districtID,
		OwnerName:    req.OwnerName,
		BusinessName: req.BusinessName,
	}

	created, err := h.repo.Create(record)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "create_failed",
			Message: "Failed to create lounge owner district",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Lounge owner district created successfully",
		"data":    created,
	})
}

// GetByDistrict handles GET /api/v1/lounge-owner-districts/by-district/:district_id
func (h *LoungeOwnerDistrictHandler) GetByDistrict(c *gin.Context) {
	districtIDStr := c.Param("district_id")
	districtID, err := uuid.Parse(districtIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_district_id",
			Message: "district_id must be a valid UUID",
		})
		return
	}

	records, err := h.repo.GetByDistrictID(districtID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "fetch_failed",
			Message: "Failed to fetch lounge owner districts",
		})
		return
	}

	items := make([]LoungeOwnerDistrictFetchItem, 0, len(records))
	for _, record := range records {
		items = append(items, LoungeOwnerDistrictFetchItem{
			OwnerID:      record.OwnerID,
			DistrictID:   record.DistrictID,
			OwnerName:    record.OwnerName,
			BusinessName: record.BusinessName,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"district_id": districtID,
		"count":       len(items),
		"data":        items,
	})
}

// CheckExists handles POST /api/v1/lounge-owner-districts/check-exists
// Returns already_stored=true if the owner_id + district_id pair exists, else false.
func (h *LoungeOwnerDistrictHandler) CheckExists(c *gin.Context) {
	var req CheckLoungeOwnerDistrictRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "validation_error",
			Message: "Invalid request body: " + err.Error(),
		})
		return
	}

	ownerID, err := uuid.Parse(req.OwnerID)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_owner_id",
			Message: "owner_id must be a valid UUID",
		})
		return
	}

	districtID, err := uuid.Parse(req.DistrictID)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_district_id",
			Message: "district_id must be a valid UUID",
		})
		return
	}

	exists, err := h.repo.ExistsByOwnerAndDistrict(ownerID, districtID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "check_failed",
			Message: "Failed to check lounge owner district pair",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"owner_id":       ownerID,
		"district_id":    districtID,
		"already_stored": exists,
	})
}
