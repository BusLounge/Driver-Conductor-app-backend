package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/smarttransit/sms-auth-backend/internal/database"
	"github.com/smarttransit/sms-auth-backend/internal/middleware"
	"github.com/smarttransit/sms-auth-backend/internal/models"
	"github.com/smarttransit/sms-auth-backend/pkg/sms"
)

// LoungeBookingDriverAssignmentHandler handles driver assignment operations
type LoungeBookingDriverAssignmentHandler struct {
	assignmentRepo   *database.LoungeBookingDriverAssignmentRepository
	loungeOwnerRepo  *database.LoungeOwnerRepository
	loungeRepo       *database.LoungeRepository
	bookingRepo      *database.LoungeBookingRepository
	loungeDriverRepo *database.LoungeDriverRepository
	smsGateway       sms.SMSGateway
}

// NewLoungeBookingDriverAssignmentHandler creates a new handler
func NewLoungeBookingDriverAssignmentHandler(
	assignmentRepo *database.LoungeBookingDriverAssignmentRepository,
	loungeOwnerRepo *database.LoungeOwnerRepository,
	loungeRepo *database.LoungeRepository,
	bookingRepo *database.LoungeBookingRepository,
	loungeDriverRepo *database.LoungeDriverRepository,
	smsGateway sms.SMSGateway,
) *LoungeBookingDriverAssignmentHandler {
	return &LoungeBookingDriverAssignmentHandler{
		assignmentRepo:   assignmentRepo,
		loungeOwnerRepo:  loungeOwnerRepo,
		loungeRepo:       loungeRepo,
		bookingRepo:      bookingRepo,
		loungeDriverRepo: loungeDriverRepo,
		smsGateway:       smsGateway,
	}
}

// CreateAssignment handles POST /api/v1/lounge-booking-driver-assignments
func (h *LoungeBookingDriverAssignmentHandler) CreateAssignment(c *gin.Context) {
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "User context not found",
		})
		return
	}

	var req models.CreateLoungeBookingDriverAssignmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
		return
	}

	// Verify lounge ownership
	owner, err := h.loungeOwnerRepo.GetLoungeOwnerByUserID(userCtx.UserID)
	if err != nil || owner == nil {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Error:   "forbidden",
			Message: "Not a lounge owner",
		})
		return
	}

	lounge, err := h.loungeRepo.GetLoungeByID(req.LoungeID)
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
			Message: "You don't own this lounge",
		})
		return
	}

	// Enforce one active driver assignment per booking
	existingAssignment, err := h.assignmentRepo.CheckIfDriverAssigned(req.LoungeBookingID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		log.Printf("ERROR: Failed to check existing assignment: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to check existing assignment",
		})
		return
	}

	if existingAssignment != nil {
		c.JSON(http.StatusConflict, ErrorResponse{
			Error:   "already_assigned",
			Message: "A driver is already assigned to this booking",
		})
		return
	}

	assignment := &models.LoungeBookingDriverAssignment{
		ID:              uuid.New(),
		LoungeID:        req.LoungeID,
		DriverID:        req.DriverID,
		LoungeBookingID: req.LoungeBookingID,
		GuestName:       req.GuestName,
		GuestContact:    req.GuestContact,
		DriverContact:   req.DriverContact,
		Status:          models.DriverAssignmentStatusPending,
	}

	if err := h.assignmentRepo.CreateAssignment(assignment); err != nil {
		log.Printf("ERROR: Failed to create driver assignment: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to create assignment",
		})
		return
	}

	// Update transport_bookings status to confirmed
	if err := h.assignmentRepo.UpdateTransportBookingStatus(assignment.LoungeBookingID, "confirmed"); err != nil {
		log.Printf("ERROR: Failed to update transport booking status to confirmed: %v", err)
	}

	// Fetch driver details
	var driverName string = "Assigned Driver"
	var vehicleNo string = "N/A"
	var driverPhone string = req.DriverContact
	driver, err := h.loungeDriverRepo.GetDriverByID(req.DriverID)
	if err == nil && driver != nil {
		driverName = driver.Name
		vehicleNo = driver.VehicleNumber
		driverPhone = driver.ContactNumber
	}

	// Fetch booking details for pickup location name and primary guest phone
	var pickupLocation string = "N/A"
	var passengerPhone string = req.GuestContact
	booking, err := h.bookingRepo.GetLoungeBookingByID(req.LoungeBookingID)
	if err == nil && booking != nil {
		if booking.PickupLocationName != "" {
			pickupLocation = booking.PickupLocationName
		}
		if strings.TrimSpace(booking.PrimaryGuestPhone) != "" {
			passengerPhone = booking.PrimaryGuestPhone
		}
	}

	// Send driver assignment SMS including pickup location name
	h.sendDriverAssignmentSMS(req.DriverContact, req.GuestName, req.GuestContact, pickupLocation)

	// Send passenger/guest assignment SMS
	h.sendPassengerAssignmentSMS(passengerPhone, driverName, vehicleNo, driverPhone)

	c.JSON(http.StatusCreated, gin.H{
		"message":    "Driver assignment created successfully",
		"assignment": assignment,
	})
}

func (h *LoungeBookingDriverAssignmentHandler) sendDriverAssignmentSMS(driverContact, guestName, guestContact, pickupLocation string) {
	if h.smsGateway == nil {
		return
	}

	phone := strings.TrimSpace(driverContact)
	if phone == "" {
		return
	}

	name := strings.TrimSpace(guestName)
	if name == "" {
		name = "Guest"
	}

	if pickupLocation == "" {
		pickupLocation = "N/A"
	}

	contact := strings.TrimSpace(guestContact)
	if contact == "" {
		message := "Your vehicle is booked for: " + name + ". Pickup Location: " + pickupLocation + "."
		if _, err := h.smsGateway.SendMessage(phone, message); err != nil {
			log.Printf("WARN: Failed to send assignment SMS to %s: %v", phone, err)
			return
		}

		log.Printf("INFO: Assignment SMS sent to driver %s", phone)
		return
	}

	message := "Your vehicle is booked for: " + name + ". Pickup Location: " + pickupLocation + ". Contact the passenger at " + contact + " and get directions."
	if _, err := h.smsGateway.SendMessage(phone, message); err != nil {
		log.Printf("WARN: Failed to send assignment SMS to %s: %v", phone, err)
		return
	}

	log.Printf("INFO: Assignment SMS sent to driver %s", phone)
}

func (h *LoungeBookingDriverAssignmentHandler) sendPassengerAssignmentSMS(passengerContact, driverName, vehicleNo, driverPhone string) {
	if h.smsGateway == nil {
		return
	}

	phone := strings.TrimSpace(passengerContact)
	if phone == "" {
		return
	}

	message := fmt.Sprintf("your booking for transportation is confirmed. Driver: %s, Vehicle: %s, Phone: %s", driverName, vehicleNo, driverPhone)
	if _, err := h.smsGateway.SendMessage(phone, message); err != nil {
		log.Printf("WARN: Failed to send assignment SMS to passenger %s: %v", phone, err)
		return
	}

	log.Printf("INFO: Assignment SMS sent to passenger %s", phone)
}

// GetAssignmentByID handles GET /api/v1/lounge-booking-driver-assignments/:id
func (h *LoungeBookingDriverAssignmentHandler) GetAssignmentByID(c *gin.Context) {
	assignmentIDStr := c.Param("id")
	assignmentID, err := uuid.Parse(assignmentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid assignment ID format",
		})
		return
	}

	assignment, err := h.assignmentRepo.GetAssignmentByID(assignmentID)
	if err != nil || assignment == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "Assignment not found",
		})
		return
	}

	c.JSON(http.StatusOK, assignment)
}

// GetAssignmentsByBooking handles GET /api/v1/lounge-bookings/:id/driver-assignments
func (h *LoungeBookingDriverAssignmentHandler) GetAssignmentsByBooking(c *gin.Context) {
	bookingIDStr := c.Param("id")
	bookingID, err := uuid.Parse(bookingIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid booking ID format",
		})
		return
	}

	assignments, err := h.assignmentRepo.GetAssignmentByBookingID(bookingID)
	if err != nil {
		log.Printf("ERROR: Failed to get assignments for booking: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to retrieve assignments",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"assignments": assignments,
		"total":       len(assignments),
	})
}

// GetAssignmentsByDriver handles GET /api/v1/drivers/:driver_id/assignments
func (h *LoungeBookingDriverAssignmentHandler) GetAssignmentsByDriver(c *gin.Context) {
	driverIDStr := c.Param("driver_id")
	driverID, err := uuid.Parse(driverIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid driver ID format",
		})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	statusQuery := c.Query("status")
	var status *string
	if statusQuery != "" {
		status = &statusQuery
	}

	assignments, err := h.assignmentRepo.GetAssignmentsByDriverID(driverID, status, limit, offset)
	if err != nil {
		log.Printf("ERROR: Failed to get driver assignments: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to retrieve assignments",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"assignments": assignments,
		"limit":       limit,
		"offset":      offset,
	})
}

// GetAssignmentsByLounge handles GET /api/v1/lounges/:id/driver-assignments
func (h *LoungeBookingDriverAssignmentHandler) GetAssignmentsByLounge(c *gin.Context) {
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

	// Verify lounge ownership
	owner, err := h.loungeOwnerRepo.GetLoungeOwnerByUserID(userCtx.UserID)
	if err != nil || owner == nil {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Error:   "forbidden",
			Message: "Not a lounge owner",
		})
		return
	}

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
			Message: "You don't own this lounge",
		})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	statusQuery := c.Query("status")
	var status *string
	if statusQuery != "" {
		status = &statusQuery
	}

	assignments, err := h.assignmentRepo.GetAssignmentsByLoungeID(loungeID, status, limit, offset)
	if err != nil {
		log.Printf("ERROR: Failed to get lounge driver assignments: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to retrieve assignments",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"assignments": assignments,
		"lounge_id":   loungeID,
		"limit":       limit,
		"offset":      offset,
	})
}

// UpdateAssignment handles PUT /api/v1/lounge-booking-driver-assignments/:id
func (h *LoungeBookingDriverAssignmentHandler) UpdateAssignment(c *gin.Context) {
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "User context not found",
		})
		return
	}

	assignmentIDStr := c.Param("id")
	assignmentID, err := uuid.Parse(assignmentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid assignment ID format",
		})
		return
	}

	// Get the assignment
	assignment, err := h.assignmentRepo.GetAssignmentByID(assignmentID)
	if err != nil || assignment == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "Assignment not found",
		})
		return
	}

	// Verify lounge ownership
	owner, err := h.loungeOwnerRepo.GetLoungeOwnerByUserID(userCtx.UserID)
	if err != nil || owner == nil {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Error:   "forbidden",
			Message: "Not a lounge owner",
		})
		return
	}

	lounge, err := h.loungeRepo.GetLoungeByID(assignment.LoungeID)
	if err != nil || lounge == nil || lounge.LoungeOwnerID != owner.ID {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Error:   "forbidden",
			Message: "You don't own this lounge",
		})
		return
	}

	var req models.UpdateLoungeBookingDriverAssignmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
		return
	}

	if err := h.assignmentRepo.UpdateAssignment(assignmentID, &req); err != nil {
		log.Printf("ERROR: Failed to update assignment: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to update assignment",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "Assignment updated successfully",
		"assignment_id": assignmentID,
	})
}

// DeleteAssignment handles DELETE /api/v1/lounge-booking-driver-assignments/:id
func (h *LoungeBookingDriverAssignmentHandler) DeleteAssignment(c *gin.Context) {
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "User context not found",
		})
		return
	}

	assignmentIDStr := c.Param("id")
	assignmentID, err := uuid.Parse(assignmentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid assignment ID format",
		})
		return
	}

	// Get the assignment
	assignment, err := h.assignmentRepo.GetAssignmentByID(assignmentID)
	if err != nil || assignment == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "Assignment not found",
		})
		return
	}

	// Verify lounge ownership
	owner, err := h.loungeOwnerRepo.GetLoungeOwnerByUserID(userCtx.UserID)
	if err != nil || owner == nil {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Error:   "forbidden",
			Message: "Not a lounge owner",
		})
		return
	}

	lounge, err := h.loungeRepo.GetLoungeByID(assignment.LoungeID)
	if err != nil || lounge == nil || lounge.LoungeOwnerID != owner.ID {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Error:   "forbidden",
			Message: "You don't own this lounge",
		})
		return
	}

	if err := h.assignmentRepo.DeleteAssignment(assignmentID); err != nil {
		log.Printf("ERROR: Failed to delete assignment: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to delete assignment",
		})
		return
	}

	// Update transport_bookings status to canceled
	if err := h.assignmentRepo.UpdateTransportBookingStatus(assignment.LoungeBookingID, "canceled"); err != nil {
		log.Printf("ERROR: Failed to update transport booking status to canceled: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "Assignment deleted successfully",
		"assignment_id": assignmentID,
	})
}

// CancelAssignment handles POST /api/v1/lounge-booking-driver-assignments/:id/cancel
func (h *LoungeBookingDriverAssignmentHandler) CancelAssignment(c *gin.Context) {
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "User context not found",
		})
		return
	}

	assignmentIDStr := c.Param("id")
	assignmentID, err := uuid.Parse(assignmentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid assignment ID format",
		})
		return
	}

	// Get the assignment
	assignment, err := h.assignmentRepo.GetAssignmentByID(assignmentID)
	if err != nil || assignment == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "Assignment not found",
		})
		return
	}

	// Verify lounge ownership
	owner, err := h.loungeOwnerRepo.GetLoungeOwnerByUserID(userCtx.UserID)
	if err != nil || owner == nil {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Error:   "forbidden",
			Message: "Not a lounge owner",
		})
		return
	}

	lounge, err := h.loungeRepo.GetLoungeByID(assignment.LoungeID)
	if err != nil || lounge == nil || lounge.LoungeOwnerID != owner.ID {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Error:   "forbidden",
			Message: "You don't own this lounge",
		})
		return
	}

	if err := h.assignmentRepo.CancelAssignment(assignmentID); err != nil {
		log.Printf("ERROR: Failed to cancel assignment: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to cancel assignment",
		})
		return
	}

	// Update transport_bookings status to canceled
	if err := h.assignmentRepo.UpdateTransportBookingStatus(assignment.LoungeBookingID, "canceled"); err != nil {
		log.Printf("ERROR: Failed to update transport booking status to canceled: %v", err)
	}

	// Fetch booking details for primary guest phone
	var passengerPhone string = assignment.GuestContact
	booking, err := h.bookingRepo.GetLoungeBookingByID(assignment.LoungeBookingID)
	if err == nil && booking != nil && strings.TrimSpace(booking.PrimaryGuestPhone) != "" {
		passengerPhone = booking.PrimaryGuestPhone
	}

	// Send cancellation SMS to driver
	h.sendDriverCancellationSMS(assignment.DriverContact, assignment.GuestName)

	// Send cancellation SMS to passenger
	h.sendPassengerCancellationSMS(passengerPhone)

	c.JSON(http.StatusOK, gin.H{
		"message":       "Assignment cancelled successfully",
		"assignment_id": assignmentID,
	})
}

func (h *LoungeBookingDriverAssignmentHandler) sendDriverCancellationSMS(driverContact, guestName string) {
	if h.smsGateway == nil {
		return
	}

	phone := strings.TrimSpace(driverContact)
	if phone == "" {
		return
	}

	name := strings.TrimSpace(guestName)
	if name == "" {
		name = "Guest"
	}

	message := "The booking for passenger " + name + " has been cancelled."
	if _, err := h.smsGateway.SendMessage(phone, message); err != nil {
		log.Printf("WARN: Failed to send cancellation SMS to %s: %v", phone, err)
		return
	}

	log.Printf("INFO: Cancellation SMS sent to driver %s", phone)
}

func (h *LoungeBookingDriverAssignmentHandler) sendPassengerCancellationSMS(passengerContact string) {
	if h.smsGateway == nil {
		return
	}

	phone := strings.TrimSpace(passengerContact)
	if phone == "" {
		return
	}

	message := "unfortunatly we had to cancel the transport booking but we will assign a driver quickly as possible thank you for your understanding."
	if _, err := h.smsGateway.SendMessage(phone, message); err != nil {
		log.Printf("WARN: Failed to send cancellation SMS to passenger %s: %v", phone, err)
		return
	}

	log.Printf("INFO: Cancellation SMS sent to passenger %s", phone)
}

// CheckDriverAssignment handles GET /api/v1/lounge-booking-driver-assignments/check/:booking_id
// Checks if a driver is already assigned to a specific booking
func (h *LoungeBookingDriverAssignmentHandler) CheckDriverAssignment(c *gin.Context) {
	h.getAssignedDriverByBooking(c)
}

// GetAssignedDriverByBooking handles GET /api/v1/lounge-bookings/:id/assigned-driver
// Returns the active driver assignment for a booking, if one exists.
func (h *LoungeBookingDriverAssignmentHandler) GetAssignedDriverByBooking(c *gin.Context) {
	h.getAssignedDriverByBooking(c)
}

func (h *LoungeBookingDriverAssignmentHandler) getAssignedDriverByBooking(c *gin.Context) {
	bookingIDStr := c.Param("booking_id")
	if bookingIDStr == "" {
		bookingIDStr = c.Param("id")
	}
	bookingID, err := uuid.Parse(bookingIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid booking ID format",
		})
		return
	}

	assignment, err := h.assignmentRepo.CheckIfDriverAssigned(bookingID)
	if err != nil {
		// No assignment found or other error - treat as not assigned
		c.JSON(http.StatusOK, gin.H{
			"assigned":           false,
			"assignment":         nil,
			"assignment_id":      nil,
			"assigned_driver_id": nil,
			"booking_id":         bookingID,
		})
		return
	}

	if assignment == nil {
		c.JSON(http.StatusOK, gin.H{
			"assigned":           false,
			"assignment":         nil,
			"assignment_id":      nil,
			"assigned_driver_id": nil,
			"booking_id":         bookingID,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"assigned":           true,
		"assignment_id":      assignment.ID,
		"assigned_driver_id": assignment.DriverID,
		"booking_id":         assignment.LoungeBookingID,
		"assignment":         assignment,
	})
}

// CompleteAssignment handles POST /api/v1/lounge-booking-driver-assignments/:id/complete
func (h *LoungeBookingDriverAssignmentHandler) CompleteAssignment(c *gin.Context) {
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "User context not found",
		})
		return
	}

	assignmentIDStr := c.Param("id")
	assignmentID, err := uuid.Parse(assignmentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid assignment ID format",
		})
		return
	}

	// Get the assignment
	assignment, err := h.assignmentRepo.GetAssignmentByID(assignmentID)
	if err != nil || assignment == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "Assignment not found",
		})
		return
	}

	// Verify lounge ownership
	owner, err := h.loungeOwnerRepo.GetLoungeOwnerByUserID(userCtx.UserID)
	if err != nil || owner == nil {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Error:   "forbidden",
			Message: "Not a lounge owner",
		})
		return
	}

	lounge, err := h.loungeRepo.GetLoungeByID(assignment.LoungeID)
	if err != nil || lounge == nil || lounge.LoungeOwnerID != owner.ID {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Error:   "forbidden",
			Message: "You don't own this lounge",
		})
		return
	}

	if err := h.assignmentRepo.CompleteAssignment(assignmentID); err != nil {
		log.Printf("ERROR: Failed to complete assignment: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to complete assignment",
		})
		return
	}

	// Update transport_bookings status to completed
	if err := h.assignmentRepo.UpdateTransportBookingStatus(assignment.LoungeBookingID, "completed"); err != nil {
		log.Printf("ERROR: Failed to update transport booking status to completed: %v", err)
	}

	// Fetch booking details for primary guest phone
	var passengerPhone string = assignment.GuestContact
	booking, err := h.bookingRepo.GetLoungeBookingByID(assignment.LoungeBookingID)
	if err == nil && booking != nil && strings.TrimSpace(booking.PrimaryGuestPhone) != "" {
		passengerPhone = booking.PrimaryGuestPhone
	}

	// Send completion SMS to driver
	h.sendDriverCompletionSMS(assignment.DriverContact)

	// Send completion SMS to passenger
	h.sendPassengerCompletionSMS(passengerPhone)

	c.JSON(http.StatusOK, gin.H{
		"message":       "Assignment completed successfully",
		"assignment_id": assignmentID,
	})
}

func (h *LoungeBookingDriverAssignmentHandler) sendDriverCompletionSMS(driverContact string) {
	if h.smsGateway == nil {
		return
	}

	phone := strings.TrimSpace(driverContact)
	if phone == "" {
		return
	}

	message := "the trip is completed successfully have a nice day!"
	if _, err := h.smsGateway.SendMessage(phone, message); err != nil {
		log.Printf("WARN: Failed to send completion SMS to driver %s: %v", phone, err)
		return
	}

	log.Printf("INFO: Completion SMS sent to driver %s", phone)
}

func (h *LoungeBookingDriverAssignmentHandler) sendPassengerCompletionSMS(passengerContact string) {
	if h.smsGateway == nil {
		return
	}

	phone := strings.TrimSpace(passengerContact)
	if phone == "" {
		return
	}

	message := "the trip is completed successfully have a nice day!"
	if _, err := h.smsGateway.SendMessage(phone, message); err != nil {
		log.Printf("WARN: Failed to send completion SMS to passenger %s: %v", phone, err)
		return
	}

	log.Printf("INFO: Completion SMS sent to passenger %s", phone)
}

