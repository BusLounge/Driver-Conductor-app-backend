package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/smarttransit/sms-auth-backend/internal/database"
)

// DistrictHandler handles district lookup endpoints.
type DistrictHandler struct {
	districtRepo *database.DistrictRepository
}

// NewDistrictHandler creates a new district handler.
func NewDistrictHandler(districtRepo *database.DistrictRepository) *DistrictHandler {
	return &DistrictHandler{districtRepo: districtRepo}
}

// ListDistricts handles GET /api/v1/districts
func (h *DistrictHandler) ListDistricts(c *gin.Context) {
	districts, err := h.districtRepo.GetAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch districts"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"districts": districts,
		"count":     len(districts),
	})
}

// GetDistrictByID handles GET /api/v1/districts/:id
func (h *DistrictHandler) GetDistrictByID(c *gin.Context) {
	districtID := c.Param("id")

	district, err := h.districtRepo.GetByID(districtID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch district"})
		return
	}

	if district == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "District not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"district": district})
}
