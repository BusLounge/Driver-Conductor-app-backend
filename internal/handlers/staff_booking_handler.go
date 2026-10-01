package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/smarttransit/sms-auth-backend/internal/database"
	"github.com/smarttransit/sms-auth-backend/internal/middleware"
	"github.com/smarttransit/sms-auth-backend/internal/models"
	"github.com/smarttransit/sms-auth-backend/internal/services"
)

type StaffBookingHandler struct {
	bookingRepo       *database.AppBookingRepository
	activeTripService *services.ActiveTripService
}

// NewStaffBookingHandler creates a new StaffBookingHandler
func NewStaffBookingHandler(bookingRepo *database.AppBookingRepository, activeTripService *services.ActiveTripService) *StaffBookingHandler {
	return &StaffBookingHandler{
		bookingRepo:       bookingRepo,
		activeTripService: activeTripService,
	}
}

// VerifyBookingRequest represents a request to verify a booking by QR
type VerifyBookingRequest struct {
	QRCode string `json:"qr_code" binding:"required"`
}

// extractQRIdentifiers parses a QR string which may be raw text, UUID, or JSON and returns candidate identifiers
func extractQRIdentifiers(input string) []string {
	var candidates []string
	input = strings.TrimSpace(input)
	if input == "" {
		return candidates
	}
	candidates = append(candidates, input)

	// If input is JSON, parse and extract fields
	if strings.HasPrefix(input, "{") && strings.HasSuffix(input, "}") {
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(input), &data); err == nil {
			keys := []string{
				"qrData", "qr_data", "qr_code", "qrCode", "qr",
				"referenceNo", "reference_no", "booking_reference", "bookingReference", "reference", "ref",
				"busBookingId", "bus_booking_id",
				"bookingId", "booking_id", "id",
			}
			for _, k := range keys {
				if val, ok := data[k]; ok {
					if strVal, isStr := val.(string); isStr && strings.TrimSpace(strVal) != "" {
						strVal = strings.TrimSpace(strVal)
						found := false
						for _, c := range candidates {
							if c == strVal {
								found = true
								break
							}
						}
						if !found {
							candidates = append([]string{strVal}, candidates...)
						}
					}
				}
			}
		}
	}
	return candidates
}

// VerifyBookingByQR verifies a booking by scanning QR code
// @Summary Verify booking by QR
// @Description Conductor/Driver scans QR to verify booking
// @Tags Staff Bookings
// @Accept json
// @Produce json
// @Param request body VerifyBookingRequest true "QR code data"
// @Success 200 {object} map[string]interface{} "Booking details"
// @Failure 400 {object} map[string]interface{} "Invalid QR"
// @Failure 401 {object} map[string]interface{} "Unauthorized"
// @Failure 404 {object} map[string]interface{} "Booking not found"
// @Security BearerAuth
// @Router /api/v1/staff/bookings/verify [post]
func (h *StaffBookingHandler) VerifyBookingByQR(c *gin.Context) {
	_, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// TODO: Add role check for conductor/driver

	var req VerifyBookingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request", "details": err.Error()})
		return
	}

	var busBooking *models.BusBooking
	var masterBooking *models.MasterBooking
	var allBusBookings []models.BusBooking

	candidates := extractQRIdentifiers(req.QRCode)

	// Search using candidate identifiers (handles raw QR, reference, UUID, or extracted JSON fields)
	for _, candidate := range candidates {
		// 1. Try finding bus booking directly by QR code / reference / UUID
		bb, err := h.bookingRepo.GetBusBookingByQRCode(candidate)
		if err == nil && bb != nil {
			busBooking = bb
			masterBooking, _ = h.bookingRepo.GetBookingByID(bb.BookingID)
			allBusBookings, _ = h.bookingRepo.GetAllBusBookingsByBookingID(bb.BookingID)
			break
		}

		// 2. Try finding master booking by reference / QR / UUID
		mb, masterErr := h.bookingRepo.GetBookingByReference(candidate)
		if masterErr == nil && mb != nil {
			masterBooking = mb
			fetchedBookings, fetchErr := h.bookingRepo.GetAllBusBookingsByBookingID(mb.ID)
			if fetchErr == nil && len(fetchedBookings) > 0 {
				allBusBookings = fetchedBookings
				busBooking = &allBusBookings[0]
				break
			}
		}
	}

	// Smart route matching across all bus bookings
	if len(allBusBookings) > 0 {
		// 1. Prioritize leg matching conductor's active trip
		if h.activeTripService != nil {
			userCtx, exists := middleware.GetUserContext(c)
			if exists {
				activeTrip, _ := h.activeTripService.GetMyActiveTrip(userCtx.UserID.String())
				if activeTrip != nil {
					for i := range allBusBookings {
						if allBusBookings[i].ScheduledTripID == activeTrip.ScheduledTripID {
							busBooking = &allBusBookings[i]
							break
						}
					}
				}
			}
		}

		// 2. Fallback: Find the first leg that isn't completed/boarded/checked_in
		if busBooking == nil {
			for i := range allBusBookings {
				status := allBusBookings[i].Status
				if status != models.BusBookingCompleted && status != models.BusBookingCancelled && 
				   status != models.BusBookingNoShow && status != models.BusBookingBoarded && 
				   status != models.BusBookingCheckedIn {
					busBooking = &allBusBookings[i]
					break
				}
			}
		}

		// 3. Fallback: if all legs are completed, use first leg
		if busBooking == nil {
			busBooking = &allBusBookings[0]
		}
	}

	if busBooking == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Booking not found or no valid legs available"})
		return
	}

	// Ensure master booking is loaded
	if masterBooking == nil {
		masterBooking, _ = h.bookingRepo.GetBookingByID(busBooking.BookingID)
	}

	// Build response with master booking details
	response := gin.H{
		"valid":              true,
		"bus_booking_id":     busBooking.ID,
		"route_name":         busBooking.RouteName,
		"boarding_stop":      busBooking.BoardingStopName,
		"alighting_stop":     busBooking.AlightingStopName,
		"departure_datetime": busBooking.DepartureDatetime,
		"number_of_seats":    busBooking.NumberOfSeats,
		"status":             busBooking.Status,
		"is_checked_in":      busBooking.CheckedInAt != nil,
		"check_in_time":      busBooking.CheckedInAt,
		"total_fare":         busBooking.TotalFare,
		"seats":              busBooking.Seats,
	}

	// Add master booking details if available
	if masterBooking != nil {
		response["passenger_name"] = masterBooking.PassengerName
		response["booking_reference"] = masterBooking.BookingReference
		response["payment_status"] = masterBooking.PaymentStatus
	} else {
		response["payment_status"] = "paid" // default assumption
	}

	// Add transit information (all legs) if there are multiple bus bookings
	if len(allBusBookings) > 1 {
		var transitLegs []gin.H
		for _, leg := range allBusBookings {
			transitLegs = append(transitLegs, gin.H{
				"bus_booking_id":     leg.ID,
				"route_name":         leg.RouteName,
				"boarding_stop":      leg.BoardingStopName,
				"alighting_stop":     leg.AlightingStopName,
				"departure_datetime": leg.DepartureDatetime,
				"status":             leg.Status,
				"number_of_seats":    leg.NumberOfSeats,
			})
		}
		response["is_transit"] = true
		response["transit_legs"] = transitLegs
		response["total_legs"] = len(allBusBookings)
	} else {
		response["is_transit"] = false
	}

	c.JSON(http.StatusOK, response)
}

// CheckInRequest represents a check-in request
type CheckInRequest struct {
	BusBookingID string `json:"bus_booking_id" binding:"required"`
	// Optional: specific seat to check in
	SeatID string `json:"seat_id,omitempty"`
}

// CheckInPassenger marks passenger as checked-in
// @Summary Check in passenger
// @Description Conductor marks passenger as checked-in (verified ticket)
// @Tags Staff Bookings
// @Accept json
// @Produce json
// @Param request body CheckInRequest true "Check-in details"
// @Success 200 {object} map[string]interface{} "Checked in successfully"
// @Failure 400 {object} map[string]interface{} "Invalid request"
// @Failure 401 {object} map[string]interface{} "Unauthorized"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Security BearerAuth
// @Router /api/v1/staff/bookings/check-in [post]
func (h *StaffBookingHandler) CheckInPassenger(c *gin.Context) {
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req CheckInRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request", "details": err.Error()})
		return
	}

	// If specific seat, check in that seat
	if req.SeatID != "" {
		err := h.bookingRepo.CheckInPassenger(req.SeatID, userCtx.UserID.String())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check in", "details": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"message": "Seat checked in successfully",
			"seat_id": req.SeatID,
		})
		return
	}

	// Otherwise check in the whole bus booking
	err := h.bookingRepo.CheckInBusBooking(req.BusBookingID, userCtx.UserID.String())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check in", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":        "Booking checked in successfully",
		"bus_booking_id": req.BusBookingID,
	})
}

// BoardRequest represents a boarding request
type BoardRequest struct {
	SeatID string `json:"seat_id" binding:"required"`
}

// BoardPassenger marks passenger as boarded
// @Summary Board passenger
// @Description Conductor marks passenger as boarded (on the bus)
// @Tags Staff Bookings
// @Accept json
// @Produce json
// @Param request body BoardRequest true "Boarding details"
// @Success 200 {object} map[string]interface{} "Boarded successfully"
// @Failure 400 {object} map[string]interface{} "Invalid request"
// @Failure 401 {object} map[string]interface{} "Unauthorized"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Security BearerAuth
// @Router /api/v1/staff/bookings/board [post]
func (h *StaffBookingHandler) BoardPassenger(c *gin.Context) {
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req BoardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request", "details": err.Error()})
		return
	}

	err := h.bookingRepo.BoardPassenger(req.SeatID, userCtx.UserID.String())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to board passenger", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Passenger boarded successfully",
		"seat_id": req.SeatID,
	})
}

// NoShowRequest represents a no-show request
type NoShowRequest struct {
	SeatID string `json:"seat_id" binding:"required"`
}

// MarkNoShow marks passenger as no-show
// @Summary Mark no-show
// @Description Conductor marks passenger as no-show
// @Tags Staff Bookings
// @Accept json
// @Produce json
// @Param request body NoShowRequest true "No-show details"
// @Success 200 {object} map[string]interface{} "Marked as no-show"
// @Failure 400 {object} map[string]interface{} "Invalid request"
// @Failure 401 {object} map[string]interface{} "Unauthorized"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Security BearerAuth
// @Router /api/v1/staff/bookings/no-show [post]
func (h *StaffBookingHandler) MarkNoShow(c *gin.Context) {
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req NoShowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request", "details": err.Error()})
		return
	}

	err := h.bookingRepo.MarkNoShow(req.SeatID, userCtx.UserID.String())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to mark no-show", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Passenger marked as no-show",
		"seat_id": req.SeatID,
	})
}

// GetTripBookings gets all bookings for a trip
// @Summary Get trip bookings
// @Description Get all bookings for a scheduled trip (for staff)
// @Tags Staff Bookings
// @Produce json
// @Param trip_id path string true "Scheduled Trip ID"
// @Success 200 {array} models.BusBooking "List of bookings"
// @Failure 401 {object} map[string]interface{} "Unauthorized"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Security BearerAuth
// @Router /api/v1/staff/trips/{trip_id}/bookings [get]
func (h *StaffBookingHandler) GetTripBookings(c *gin.Context) {
	_, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	tripID := c.Param("trip_id")
	if tripID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Trip ID is required"})
		return
	}

	bookings, err := h.bookingRepo.GetBusBookingsByTripID(tripID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get bookings"})
		return
	}

	// Calculate stats (boarding is now tracked at bus_bookings level, not seat level)
	var totalBooked, checkedIn, boarded, noShow int
	for _, b := range bookings {
		totalBooked += b.NumberOfSeats
		if b.CheckedInAt != nil {
			checkedIn += b.NumberOfSeats
		}
		if b.BoardedAt != nil {
			boarded += b.NumberOfSeats
		}
		for _, seat := range b.Seats {
			if seat.Status == "no_show" {
				noShow++
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"bookings":      bookings,
		"total_booked":  totalBooked,
		"checked_in":    checkedIn,
		"boarded":       boarded,
		"no_show":       noShow,
		"booking_count": len(bookings),
	})
}
