package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/smarttransit/sms-auth-backend/internal/database"
	"github.com/smarttransit/sms-auth-backend/internal/middleware"
	"github.com/smarttransit/sms-auth-backend/internal/models"

	// "github.com/smarttransit/sms-auth-backend/internal/models"
	"log"
	"net/http"
	"time"
	"github.com/google/uuid"
)

// LoungeTransportLocationPriceHandler handles HTTP requests related to lounge transport location prices
type LoungeTransportLocationPriceHandler struct {
	loungeOwnerRepo                  *database.LoungeOwnerRepository
	loungeRepo                       *database.LoungeRepository
	loungeTransportLocationRepo      *database.LoungeTransportLocationRepository
	loungeTransportLocationPriceRepo *database.LoungeTransportLocationPriceRepository
}

// NewLoungeTransportLocationPriceHandler create new lounge transport location price handler instance
func NewLoungeTransportLocationPriceHandler(
	loungeOwnerRepo *database.LoungeOwnerRepository,
	loungeRepo *database.LoungeRepository,
	loungeTransportLocationRepo *database.LoungeTransportLocationRepository,
	loungeTransportLocationPriceRepo *database.LoungeTransportLocationPriceRepository,
) *LoungeTransportLocationPriceHandler {
	return &LoungeTransportLocationPriceHandler{
		loungeOwnerRepo:                  loungeOwnerRepo,
		loungeRepo:                       loungeRepo,
		loungeTransportLocationRepo:      loungeTransportLocationRepo,
		loungeTransportLocationPriceRepo: loungeTransportLocationPriceRepo,
	}
}

// add lounge transport location price request
type SetLoungeTransportLocationPriceRequest struct {
	ThreeWheelerPrice *float64 `json:"three_wheeler_price" binding:"omitempty,min=50"`
	CarPrice          *float64 `json:"car_price" binding:"omitempty,min=50"`
	VanPrice          *float64 `json:"van_price" binding:"omitempty,min=50"`
}

// handles both inserting and updating prices for lounge transport location
func (h *LoungeTransportLocationPriceHandler) SetLoungeTransportLocationPrices(c *gin.Context) {

	// auth check
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "User context not found",
		})
		return
	}

	// Parse IDs from URL
	loungeID := uuid.MustParse(c.Param("id"))
	locationID := uuid.MustParse(c.Param("location_id"))

	// Verify owner
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
			Message: "You do not have permission to set transport location prices for this lounge",
		})
		return
	}

	// Verify location exists and belongs to lounge
	location, err := h.loungeTransportLocationRepo.GetLoungeTransportLocationByID(locationID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to verify location",
		})
		return
	}
	if location == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "Transport location not found",
		})
		return
	}
	if location.LoungeID != loungeID {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Error:   "forbidden",
			Message: "Location does not belong to this lounge",
		})
		return
	}

	// creating the req struct instnace inorder to get the data inside the struct
	var req SetLoungeTransportLocationPriceRequest
	// binding the request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "validation_failed",
			Message: err.Error(),
		})
		return
	}

	// Check if prices already exist
	existing, _ := h.loungeTransportLocationPriceRepo.GetLoungeTransportLocationPrices(loungeID, locationID)
	if existing == nil {
		// First time — require all prices
		if req.ThreeWheelerPrice == nil || req.CarPrice == nil || req.VanPrice == nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "validation_failed",
				Message: "three_wheeler_price, car_price, and van_price are required on first set",
			})
			return
		}
	}

	// Handle nil pointers (default to 0)
	threeWheeler := 0.0
	if req.ThreeWheelerPrice != nil {
		threeWheeler = *req.ThreeWheelerPrice
	}
	car := 0.0
	if req.CarPrice != nil {
		car = *req.CarPrice
	}
	van := 0.0
	if req.VanPrice != nil {
		van = *req.VanPrice
	}

	// Save prices (UPSERT + RETURNING)
	savedPrices, err := h.loungeTransportLocationPriceRepo.SetLoungeTransportLocationPrices(loungeID, locationID, threeWheeler, car, van)
	if err != nil {
		log.Printf("ERROR: Failed to set prices for location \"%s\" in lounge %s: %v", locationID, loungeID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "save_failed",
			Message: "Failed to save prices",
		})
		return
	}

	// Success — return saved prices
	c.JSON(http.StatusOK, savedPrices)

}


// get transport location prices related to a lounge (three_wheeler,car,van) prices
func (h *LoungeTransportLocationPriceHandler) GetLoungeTransportLocationPrices(c *gin.Context) {

	// auth check
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "User context not found",
		})
		return
	}

	// Parse IDs from URL
	loungeID := uuid.MustParse(c.Param("id"))
	locationID := uuid.MustParse(c.Param("location_id"))

	// Verify owner
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
			Message: "You do not have permission to view transport location prices for this lounge",
		})
		return
	}

	// Verify location exists and belongs to lounge
	location, err := h.loungeTransportLocationRepo.GetLoungeTransportLocationByID(locationID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to verify location",
		})
		return
	}
	if location == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "Transport location not found",
		})
		return
	}
	if location.LoungeID != loungeID {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Error:   "forbidden",
			Message: "Location does not belong to this lounge",
		})
		return
	}

	// get the prices by using the repository
	receivedPrices,err:=h.loungeTransportLocationPriceRepo.GetLoungeTransportLocationPrices(loungeID,locationID)
	if err != nil {
		log.Printf("ERROR: Failed to get transport location prices for location %s in lounge %s: %v", locationID, loungeID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to retrieve prices",
		})
		return
	}
	if receivedPrices == nil {
		// No prices set yet — return defaults
    defaultPrices := models.LoungeTransportLocationPrice{
        LoungeID:          loungeID,
        LocationID:        locationID,
        ThreeWheelerPrice: 0,
        CarPrice:          0,
        VanPrice:          0,
        UpdatedAt:         time.Time{}, // zero time
    }
    c.JSON(http.StatusOK, defaultPrices)
    return
	}

	// sending back the dataset with the length
	c.JSON(http.StatusOK,receivedPrices)

}
