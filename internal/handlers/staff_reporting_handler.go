package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/smarttransit/sms-auth-backend/internal/database"
	"github.com/smarttransit/sms-auth-backend/internal/middleware"
)

// StaffReportingHandler handles staff issue reporting HTTP requests
type StaffReportingHandler struct {
	reportRepo *database.ReportIssuesRepository
}

// NewStaffReportingHandler creates a new StaffReportingHandler
func NewStaffReportingHandler(reportRepo *database.ReportIssuesRepository) *StaffReportingHandler {
	return &StaffReportingHandler{
		reportRepo: reportRepo,
	}
}

// ReportIssueRequest represents the request body from the Flutter frontend
type ReportIssueRequest struct {
	IssueType        string  `json:"issue_type" binding:"required"`
	Description      string  `json:"description" binding:"required"`
	LocationAddress  *string `json:"location_address"`
	ScheduledTripID  *string `json:"scheduled_trip_id"`
	ActiveTripID     *string `json:"active_trip_id"`
}

// determinePriority assigns a priority level based on issue type
func determinePriority(issueType string) string {
	switch strings.ToLower(issueType) {
	case "safety_concern", "flat_wheel":
		return "high"
	case "bus_delay", "maintenance_issue", "equipment_malfunction":
		return "medium"
	case "passenger_complaint":
		return "low"
	default:
		return "medium"
	}
}

// ReportIssue handles POST /api/v1/staff/reporting
func (h *StaffReportingHandler) ReportIssue(c *gin.Context) {
	// Get authenticated user
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "unauthorized",
			"message": "User not authenticated",
		})
		return
	}

	// Parse request body
	var req ReportIssueRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "validation_error",
			"message": fmt.Sprintf("Invalid request body: %v", err),
		})
		return
	}

	// Build the database record
	report := &database.ReportIssue{
		ReportedByID: userCtx.UserID.String(),
		IssueType:    req.IssueType,
		Priority:     determinePriority(req.IssueType),
		Status:       "reported",
		Description:  req.Description,
	}

	// Set optional nullable fields
	if req.ScheduledTripID != nil && *req.ScheduledTripID != "" {
		report.ScheduledTripID = sql.NullString{String: *req.ScheduledTripID, Valid: true}
	}
	if req.ActiveTripID != nil && *req.ActiveTripID != "" {
		report.ActiveTripID = sql.NullString{String: *req.ActiveTripID, Valid: true}
	}
	if req.LocationAddress != nil && *req.LocationAddress != "" {
		report.LocationAddress = sql.NullString{String: *req.LocationAddress, Valid: true}
	}

	// Insert into database
	if err := h.reportRepo.Create(report); err != nil {
		fmt.Printf("[StaffReporting] Error creating report: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "create_failed",
			"message": fmt.Sprintf("Failed to create report: %v", err),
		})
		return
	}

	// Fetch the full record back (with joined reporter name) to return to the client
	fullReport, err := h.reportRepo.GetByID(report.ID)
	if err != nil {
		// Report was created but we couldn't re-fetch it. Return success with basic data.
		fmt.Printf("[StaffReporting] Warning: created report %s but failed to re-fetch: %v\n", report.ID, err)
		c.JSON(http.StatusCreated, gin.H{
			"success": true,
			"message": "Issue report submitted successfully",
			"report":  report.ToJSON(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Issue report submitted successfully",
		"report":  fullReport.ToJSON(),
	})
}

// GetMyReports handles GET /api/v1/staff/reporting/my-reports
func (h *StaffReportingHandler) GetMyReports(c *gin.Context) {
	// Get authenticated user
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "unauthorized",
			"message": "User not authenticated",
		})
		return
	}

	reports, err := h.reportRepo.GetByReporterID(userCtx.UserID.String())
	if err != nil {
		fmt.Printf("[StaffReporting] Error fetching reports for user %s: %v\n", userCtx.UserID.String(), err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "fetch_failed",
			"message": fmt.Sprintf("Failed to fetch reports: %v", err),
		})
		return
	}

	// Convert to JSON-safe format
	data := make([]map[string]interface{}, 0, len(reports))
	for _, r := range reports {
		data = append(data, r.ToJSON())
	}

	c.JSON(http.StatusOK, gin.H{
		"data": data,
	})
}

// GetReportByID handles GET /api/v1/staff/reporting/:id
func (h *StaffReportingHandler) GetReportByID(c *gin.Context) {
	reportID := c.Param("id")
	if reportID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "missing_id",
			"message": "Report ID is required",
		})
		return
	}

	report, err := h.reportRepo.GetByID(reportID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "not_found",
				"message": "Report not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "fetch_failed",
			"message": fmt.Sprintf("Failed to fetch report: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": report.ToJSON(),
	})
}

// GetReportsByTrip handles GET /api/v1/staff/reporting/trip/:tripId
func (h *StaffReportingHandler) GetReportsByTrip(c *gin.Context) {
	tripID := c.Param("tripId")
	if tripID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "missing_trip_id",
			"message": "Trip ID is required",
		})
		return
	}

	reports, err := h.reportRepo.GetByTripID(tripID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "fetch_failed",
			"message": fmt.Sprintf("Failed to fetch trip reports: %v", err),
		})
		return
	}

	// Convert to JSON-safe format
	data := make([]map[string]interface{}, 0, len(reports))
	for _, r := range reports {
		data = append(data, r.ToJSON())
	}

	c.JSON(http.StatusOK, gin.H{
		"reports": data,
	})
}
