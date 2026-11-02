package handlers

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/smarttransit/sms-auth-backend/internal/database"
	"github.com/smarttransit/sms-auth-backend/internal/middleware"
	"github.com/smarttransit/sms-auth-backend/internal/models"
)

// BusMaintenanceLogHandler handles HTTP requests for bus maintenance logs
type BusMaintenanceLogHandler struct {
	maintRepo *database.BusMaintenanceLogRepository
	staffRepo *database.BusStaffRepository
	busRepo   *database.BusRepository
}

// NewBusMaintenanceLogHandler creates a new BusMaintenanceLogHandler
func NewBusMaintenanceLogHandler(
	maintRepo *database.BusMaintenanceLogRepository,
	staffRepo *database.BusStaffRepository,
	busRepo *database.BusRepository,
) *BusMaintenanceLogHandler {
	return &BusMaintenanceLogHandler{
		maintRepo: maintRepo,
		staffRepo: staffRepo,
		busRepo:   busRepo,
	}
}

// CreateMaintenanceLog logs a new maintenance record
// @Summary Create maintenance log
// @Description Log a maintenance or repair event for an assigned bus
// @Tags Staff Fuel & Maintenance
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body models.CreateBusMaintenanceLogRequest true "Maintenance log payload"
// @Success 201 {object} models.BusMaintenanceLog
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/staff/maintenance-logs [post]
func (h *BusMaintenanceLogHandler) CreateMaintenanceLog(c *gin.Context) {
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

	var req models.CreateBusMaintenanceLogRequest
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

	log := &models.BusMaintenanceLog{
		BusID:              bus.ID,
		StaffID:            staff.ID,
		BusOwnerID:         bus.BusOwnerID,
		Date:               req.Date,
		MaintenanceType:    req.MaintenanceType,
		Description:        req.Description,
		TotalCost:          req.TotalCost,
		VendorName:         req.VendorName,
		OdometerReading:    req.OdometerReading,
		NextServiceDueKm:   req.NextServiceDueKm,
		NextServiceDueDate: req.NextServiceDueDate,
		Notes:              req.Notes,
	}

	if err := h.maintRepo.Create(log); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save maintenance log", "details": err.Error()})
		return
	}

	log.BusNumber = &bus.BusNumber
	c.JSON(http.StatusCreated, log)
}

// GetMyMaintenanceLogs retrieves maintenance logs for authenticated staff member
// @Summary List maintenance logs
// @Description Get maintenance logs recorded by the current staff member
// @Tags Staff Fuel & Maintenance
// @Produce json
// @Security BearerAuth
// @Param bus_id query string false "Filter by Bus ID"
// @Param from query string false "Filter from date (YYYY-MM-DD)"
// @Param to query string false "Filter to date (YYYY-MM-DD)"
// @Success 200 {array} models.BusMaintenanceLog
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/staff/maintenance-logs [get]
func (h *BusMaintenanceLogHandler) GetMyMaintenanceLogs(c *gin.Context) {
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	staff, err := h.staffRepo.GetByUserID(userCtx.UserID.String())
	if err != nil {
		c.JSON(http.StatusOK, []models.BusMaintenanceLog{})
		return
	}

	busID := c.Query("bus_id")
	fromDate := c.Query("from")
	toDate := c.Query("to")

	logs, err := h.maintRepo.GetByStaffID(staff.ID, busID, fromDate, toDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch maintenance logs"})
		return
	}

	if logs == nil {
		logs = []models.BusMaintenanceLog{}
	}

	c.JSON(http.StatusOK, logs)
}

// GetMaintenanceLogByID retrieves a specific maintenance log by ID
// @Summary Get maintenance log detail
// @Description Retrieve a specific maintenance log by ID
// @Tags Staff Fuel & Maintenance
// @Produce json
// @Security BearerAuth
// @Param id path string true "Maintenance Log ID"
// @Success 200 {object} models.BusMaintenanceLog
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /api/v1/staff/maintenance-logs/{id} [get]
func (h *BusMaintenanceLogHandler) GetMaintenanceLogByID(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID is required"})
		return
	}

	log, err := h.maintRepo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Maintenance log not found"})
		return
	}

	c.JSON(http.StatusOK, log)
}

// DeleteMaintenanceLog deletes a maintenance log
// @Summary Delete maintenance log
// @Description Delete a maintenance log previously submitted by current staff
// @Tags Staff Fuel & Maintenance
// @Produce json
// @Security BearerAuth
// @Param id path string true "Maintenance Log ID"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /api/v1/staff/maintenance-logs/{id} [delete]
func (h *BusMaintenanceLogHandler) DeleteMaintenanceLog(c *gin.Context) {
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
	if err := h.maintRepo.Delete(id, staff.ID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Log not found or cannot be deleted"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Maintenance log deleted successfully"})
}

