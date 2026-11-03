package handlers

import (
	"database/sql"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/smarttransit/sms-auth-backend/internal/database"
	"github.com/smarttransit/sms-auth-backend/internal/middleware"
	"github.com/smarttransit/sms-auth-backend/internal/models"
)

// BusFuelLogHandler handles HTTP requests for bus fuel logs
type BusFuelLogHandler struct {
	fuelRepo  *database.BusFuelLogRepository
	maintRepo *database.BusMaintenanceLogRepository
	staffRepo *database.BusStaffRepository
	busRepo   *database.BusRepository
}

// NewBusFuelLogHandler creates a new BusFuelLogHandler
func NewBusFuelLogHandler(
	fuelRepo *database.BusFuelLogRepository,
	maintRepo *database.BusMaintenanceLogRepository,
	staffRepo *database.BusStaffRepository,
	busRepo *database.BusRepository,
) *BusFuelLogHandler {
	return &BusFuelLogHandler{
		fuelRepo:  fuelRepo,
		maintRepo: maintRepo,
		staffRepo: staffRepo,
		busRepo:   busRepo,
	}
}

// CreateFuelLog logs a new fuel refill
// @Summary Create fuel refill log
// @Description Log a new fuel refill for an assigned bus
// @Tags Staff Fuel & Maintenance
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body models.CreateBusFuelLogRequest true "Fuel log payload"
// @Success 201 {object} models.BusFuelLog
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/staff/fuel-logs [post]
func (h *BusFuelLogHandler) CreateFuelLog(c *gin.Context) {
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	staff, err := h.staffRepo.GetByUserID(userCtx.UserID.String())
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Staff profile not found"})
		return
	}

	var req models.CreateBusFuelLogRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Validation error", "details": err.Error()})
		return
	}

	if err := req.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	bus, err := h.busRepo.GetByID(req.BusID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Bus not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify bus"})
		return
	}

	log := &models.BusFuelLog{
		BusID:           bus.ID,
		StaffID:         staff.ID,
		BusOwnerID:      bus.BusOwnerID,
		Date:            req.Date,
		FuelType:        req.FuelType,
		Liters:          req.Liters,
		CostPerLiter:    req.CostPerLiter,
		TotalCost:       req.TotalCost,
		OdometerReading: req.OdometerReading,
		StationName:     req.StationName,
		Notes:           req.Notes,
	}

	if err := h.fuelRepo.Create(log); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save fuel log", "details": err.Error()})
		return
	}

	log.BusNumber = &bus.BusNumber
	c.JSON(http.StatusCreated, log)
}

// GetMyFuelLogs retrieves fuel logs for authenticated staff member
// @Summary List fuel refill logs
// @Description Get fuel refill logs logged by the current staff member
// @Tags Staff Fuel & Maintenance
// @Produce json
// @Security BearerAuth
// @Param bus_id query string false "Filter by Bus ID"
// @Param from query string false "Filter from date (YYYY-MM-DD)"
// @Param to query string false "Filter to date (YYYY-MM-DD)"
// @Success 200 {array} models.BusFuelLog
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/staff/fuel-logs [get]
func (h *BusFuelLogHandler) GetMyFuelLogs(c *gin.Context) {
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	staff, err := h.staffRepo.GetByUserID(userCtx.UserID.String())
	if err != nil {
		c.JSON(http.StatusOK, []models.BusFuelLog{})
		return
	}

	busID := c.Query("bus_id")
	fromDate := c.Query("from")
	toDate := c.Query("to")

	logs, err := h.fuelRepo.GetByStaffID(staff.ID, busID, fromDate, toDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch fuel logs"})
		return
	}

	if logs == nil {
		logs = []models.BusFuelLog{}
	}

	c.JSON(http.StatusOK, logs)
}

// GetFuelLogByID retrieves a specific fuel log by ID
// @Summary Get fuel refill log detail
// @Description Retrieve a specific fuel log by ID
// @Tags Staff Fuel & Maintenance
// @Produce json
// @Security BearerAuth
// @Param id path string true "Fuel Log ID"
// @Success 200 {object} models.BusFuelLog
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /api/v1/staff/fuel-logs/{id} [get]
func (h *BusFuelLogHandler) GetFuelLogByID(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID is required"})
		return
	}

	log, err := h.fuelRepo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Fuel log not found"})
		return
	}

	c.JSON(http.StatusOK, log)
}

// DeleteFuelLog deletes a fuel log
// @Summary Delete fuel refill log
// @Description Delete a fuel refill log previously submitted by current staff
// @Tags Staff Fuel & Maintenance
// @Produce json
// @Security BearerAuth
// @Param id path string true "Fuel Log ID"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /api/v1/staff/fuel-logs/{id} [delete]
func (h *BusFuelLogHandler) DeleteFuelLog(c *gin.Context) {
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	staff, err := h.staffRepo.GetByUserID(userCtx.UserID.String())
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Staff profile not found"})
		return
	}

	id := c.Param("id")
	if err := h.fuelRepo.Delete(id, staff.ID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Log not found or cannot be deleted"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Fuel log deleted successfully"})
}

// GetMyBuses returns buses available to the staff member
// @Summary List assigned buses
// @Description Get buses belonging to staff member's employer or active buses
// @Tags Staff Fuel & Maintenance
// @Produce json
// @Security BearerAuth
// @Success 200 {array} models.Bus
// @Router /api/v1/staff/buses [get]
func (h *BusFuelLogHandler) GetMyBuses(c *gin.Context) {
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	staff, err := h.staffRepo.GetByUserID(userCtx.UserID.String())
	if err == nil && staff != nil {
		// 1. Try directly retrieving assigned buses (active trips, assigned trips, or bus owner)
		buses, err := h.busRepo.GetBusesForStaff(staff.ID)
		if err == nil && len(buses) > 0 {
			c.JSON(http.StatusOK, buses)
			return
		}

		// 2. Try current employment
		employment, err := h.staffRepo.GetCurrentEmployment(staff.ID)
		if err == nil && employment != nil && employment.BusOwnerID != "" {
			buses, err := h.busRepo.GetByOwnerID(employment.BusOwnerID)
			if err == nil && len(buses) > 0 {
				c.JSON(http.StatusOK, buses)
				return
			}
		}

		// 3. Try any employment history
		employments, err := h.staffRepo.GetEmploymentHistory(staff.ID)
		if err == nil && len(employments) > 0 {
			for _, emp := range employments {
				if emp.BusOwnerID != "" {
					buses, err := h.busRepo.GetByOwnerID(emp.BusOwnerID)
					if err == nil && len(buses) > 0 {
						c.JSON(http.StatusOK, buses)
						return
					}
				}
			}
		}
	}

	// 4. Fallback: all active buses so staff is never left with an empty bus list
	buses, err := h.busRepo.GetAllActive()
	if err != nil {
		c.JSON(http.StatusOK, []models.Bus{})
		return
	}

	c.JSON(http.StatusOK, buses)
}

// GetVehicleSummary calculates vehicle summary stats
// @Summary Get vehicle summary
// @Description Aggregated stats for fuel and maintenance expenses
// @Tags Staff Fuel & Maintenance
// @Produce json
// @Security BearerAuth
// @Param bus_id query string false "Filter by Bus ID"
// @Success 200 {object} models.VehicleSummaryResponse
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/staff/vehicle-summary [get]
func (h *BusFuelLogHandler) GetVehicleSummary(c *gin.Context) {
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	staff, err := h.staffRepo.GetByUserID(userCtx.UserID.String())
	if err != nil {
		c.JSON(http.StatusOK, models.VehicleSummaryResponse{})
		return
	}

	busID := c.Query("bus_id")
	summary, err := h.maintRepo.GetVehicleSummary(staff.ID, busID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to get summary: %v", err)})
		return
	}

	c.JSON(http.StatusOK, summary)
}
