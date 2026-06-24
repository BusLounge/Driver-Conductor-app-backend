package handlers

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/smarttransit/sms-auth-backend/internal/database"
	"github.com/smarttransit/sms-auth-backend/internal/middleware"
	"github.com/smarttransit/sms-auth-backend/internal/models"
)

// lounge driver handles the HTTP requests related to lounge driver
type LoungeDriverHandler struct {
	loungeOwnerRepo  *database.LoungeOwnerRepository
	loungeRepo       *database.LoungeRepository
	loungeDriverRepo *database.LoungeDriverRepository
}

// createing a new lounge staff handler instance
func NewLoungeDriverHandler(
	loungeOwnerRepo *database.LoungeOwnerRepository,
	loungeRepo *database.LoungeRepository,
	loungeDriverRepo *database.LoungeDriverRepository,
) *LoungeDriverHandler {
	return &LoungeDriverHandler{
		loungeOwnerRepo:  loungeOwnerRepo,
		loungeRepo:       loungeRepo,
		loungeDriverRepo: loungeDriverRepo,
	}
}

// add drivers to lounge
type AddDriverRequest struct {
	// getting user data to the struct by using binding for safety(some editing neede to be done need to add lounge_id in frontend)
	Name          string                   `json:"name" binding:"required,min=2,max=100"`
	NIC           string                   `json:"nic_number" binding:"required"`
	ContactNumber string                   `json:"contact_no" binding:"required"`
	VehicleNumber string                   `json:"vehicle_no" binding:"required"`
	VehicleType   models.DriverVehicleType `json:"vehicle_type" binding:"required,oneof=three_wheeler car van"`
	// NEWLY ADDED LoungeID (ID is coming from the frontend the owner will select it so no parsing of loungeID in request)
	LoungeID uuid.UUID `json:"lounge_id" binding:"required"`
}

// update drivers in the lounge
type UpdateDriverRequest struct {
	// getting user data to the struct by using binding for safety(for update purposes)
	Name          *string                   `json:"name" binding:"omitempty,min=2,max=100"`
	NIC           *string                   `json:"nic_number" binding:"omitempty"`
	ContactNumber *string                   `json:"contact_no" binding:"omitempty"`
	VehicleNumber *string                   `json:"vehicle_no" binding:"omitempty"`
	VehicleType   *models.DriverVehicleType `json:"vehicle_type" binding:"omitempty,oneof=three_wheeler car van"`
	Status        *models.DriverStatus      `json:"status" binding:"omitempty,oneof=active inactive"`
}

// adding drivers to the lounge
func (h *LoungeDriverHandler) AddDriver(c *gin.Context) {
	// creating the req struct instance inorder to get the data inside the struct
	var req AddDriverRequest

	// binding the request to the struct if error comes returning the error
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Validation failed",
			Message: err.Error(),
		})
		return
	}

	// validating the user who is sending the request
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "User context not found",
		})
		return
	}

	// get lounge owner record
	owner, err := h.loungeOwnerRepo.GetLoungeOwnerByUserID(userCtx.UserID)
	if err != nil {
		log.Printf("ERROR: Failed to get owner for user %s: %v", userCtx.UserID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to verify ownership",
		})
		return
	}
	if owner == nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "Lounge owner not found",
		})
		return
	}

	// verify lounge ownership using LoungeID from request only
	lounge, err := h.loungeRepo.GetLoungeByID(req.LoungeID)
	if err != nil {
		log.Printf("ERROR: Failed to get lounge %s: %v", req.LoungeID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to verify lounge",
		})
		return
	}
	if lounge == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "Lounge not found",
		})
		return
	}
	if lounge.LoungeOwnerID != owner.ID {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Error:   "forbidden",
			Message: "You don't have permission to add drivers to this lounge",
		})
		return
	}

	// converting the data into the model to feed into repository function
	driver := &models.LoungeDriver{
		LoungeID:      req.LoungeID,
		Name:          req.Name,
		NIC:           req.NIC,
		ContactNumber: req.ContactNumber,
		VehicleNumber: req.VehicleNumber,
		VehicleType:   req.VehicleType,
		Status:        models.DriverStatusActive, //default status is active for driver
	}

	// saving using the repository
	savedDriver, err := h.loungeDriverRepo.AddDriver(driver)

	// handling the errors gracefully
	if err != nil {
		log.Printf("ERROR: Failed to add driver for lounge %s: %v", req.LoungeID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "add_failed",
			Message: "Failed to add driver",
		})
		return
	}

	// if all good then sending statusCreated with the data struct for reference
	c.JSON(http.StatusCreated, savedDriver)
}

// This handles all driver fetching parts according to loungeID
func (h *LoungeDriverHandler) GetDriversByLounge(c *gin.Context) {

	// validating the user who is sending the request
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "User context not found",
		})
		return
	}

	// getting the loungeID
	loungeID := uuid.MustParse(c.Param("id"))

	// extracting the lounge owner
	owner, err := h.loungeOwnerRepo.GetLoungeOwnerByUserID(userCtx.UserID)
	if err != nil {
		log.Printf("ERROR: Failed to get owner for user %s: %v", userCtx.UserID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to verify ownership",
		})
		return
	}
	if owner == nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "Lounge owner not found",
		})
		return
	}

	//verifying the lounge ownership
	lounge, err := h.loungeRepo.GetLoungeByID(loungeID)
	if err != nil {
		log.Printf("ERROR: Failed to get lounge %s: %v", loungeID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to verify lounge",
		})
		return
	}
	if lounge == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "Lounge not found",
		})
		return
	}
	if lounge.LoungeOwnerID != owner.ID {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Error:   "forbidden",
			Message: "You don't have permission to get drivers in this lounge",
		})
		return
	}

	// getDrivers by loungeID with error handling
	drivers, err := h.loungeDriverRepo.GetDriversByLoungeID(loungeID)
	if err != nil {
		log.Printf("ERROR: Failed to get drivers for lounge %s: %v", loungeID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to retrieve drivers",
		})
		return
	}

	// converting the drivers list inorder to send back (setting the format)
	response := make([]gin.H, 0, len(drivers))

	// looping and feeding the data
	for _, s := range drivers {

		response = append(response, gin.H{
			"id":           s.ID,
			"lounge_id":    s.LoungeID,
			"name":         s.Name,
			"nic":          s.NIC,
			"contact_no":   s.ContactNumber,
			"vehicle_no":   s.VehicleNumber,
			"vehicle_type": s.VehicleType,
			"status":       s.Status,
			"created_at":   s.CreatedAt,
			"updated_at":   s.UpdatedAt,
		})
	}

	// sending back the dataset with the length
	c.JSON(http.StatusOK, gin.H{
		"drivers": response,
		"total":   len(response),
	})

}

// This handles all the HTTP requests related to removing (permenently removing drivers from a specific lounge)
func (h *LoungeDriverHandler) DeleteDriver(c *gin.Context) {

	// validating the user who is sending the request
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "User context not found",
		})
		return
	}

	// getting the loungeID
	loungeID := uuid.MustParse(c.Param("id"))
	// getting the driverID
	driverID := uuid.MustParse(c.Param("driver_id"))

	// extracting the lounge owner
	owner, err := h.loungeOwnerRepo.GetLoungeOwnerByUserID(userCtx.UserID)
	if err != nil {
		log.Printf("ERROR: Failed to get owner for user %s: %v", userCtx.UserID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to verify ownership",
		})
		return
	}
	if owner == nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "Lounge owner not found",
		})
		return
	}

	//verifying the lounge ownership
	lounge, err := h.loungeRepo.GetLoungeByID(loungeID)
	if err != nil {
		log.Printf("ERROR: Failed to get lounge %s: %v", loungeID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to verify lounge",
		})
		return
	}
	if lounge == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "Lounge not found",
		})
		return
	}
	if lounge.LoungeOwnerID != owner.ID {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Error:   "forbidden",
			Message: "You don't have permission to delete drivers in this lounge",
		})
		return
	}

	// extracting the driver by driverID
	driverData, err := h.loungeDriverRepo.GetDriverByID(driverID)
	if err != nil {
		log.Printf("ERROR: Failed to get driver %s: %v", driverID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to get driver",
		})
		return
	}
	if driverData == nil {
		log.Printf("ERROR: No driver found %s:", driverID)
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "driver not found",
		})
		return
	}

	//verifying if the driver actually belongs to the lounge
	if driverData.LoungeID != loungeID {
		log.Printf("ERROR:Driver does not belong to this lounge %s:", loungeID)
		c.JSON(http.StatusForbidden, ErrorResponse{
			Error:   "forbidden",
			Message: "Driver does not belong to this lounge",
		})
		return
	}

	// removing drivers by id
	delErr := h.loungeDriverRepo.DeleteDriver(driverID)
	if delErr != nil {
		log.Printf("ERROR: Failed to remove driver %s: %v", driverID, delErr)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "delete_failed",
			Message: "failed to delete driver",
		})
		return
	}

	// success message
	c.Status(http.StatusNoContent)

}

// This handles all the HTTP requests related to updating drivers in a certain lounge
func (h *LoungeDriverHandler) UpdateDriver(c *gin.Context) {

	// validating the user who is sending the request
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "User context not found",
		})
		return
	}

	// getting the loungeID
	loungeID := uuid.MustParse(c.Param("id"))
	// getting the driverID
	driverID := uuid.MustParse(c.Param("driver_id"))

	// extracting the lounge owner
	owner, err := h.loungeOwnerRepo.GetLoungeOwnerByUserID(userCtx.UserID)
	if err != nil {
		log.Printf("ERROR: Failed to get owner for user %s: %v", userCtx.UserID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to verify ownership",
		})
		return
	}
	if owner == nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "Lounge owner not found",
		})
		return
	}

	//verifying the lounge ownership
	lounge, err := h.loungeRepo.GetLoungeByID(loungeID)
	if err != nil {
		log.Printf("ERROR: Failed to get lounge %s: %v", loungeID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to verify lounge",
		})
		return
	}
	if lounge == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "Lounge not found",
		})
		return
	}
	if lounge.LoungeOwnerID != owner.ID {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Error:   "forbidden",
			Message: "You don't have permission to update drivers in this lounge",
		})
		return
	}

	// extracting the driver by driverID
	driverData, err := h.loungeDriverRepo.GetDriverByID(driverID)
	if err != nil {
		log.Printf("ERROR: Failed to get driver %s: %v", driverID, err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to get driver",
		})
		return
	}
	if driverData == nil {
		log.Printf("ERROR: No driver found %s: ", driverID)
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "not_found",
			Message: "driver not found",
		})
		return
	}

	//verifying if the driver actually belongs to the lounge
	if driverData.LoungeID != loungeID {
		log.Printf("ERROR:Driver does not belong to this lounge %s:", loungeID)
		c.JSON(http.StatusForbidden, ErrorResponse{
			Error:   "forbidden",
			Message: "Driver does not belong to this lounge",
		})
		return
	}

	// creating a UpdateRequest Struct for driver
	var req UpdateDriverRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "validation_failed",
			Message: err.Error(),
		})
		return
	}

	// creating a map to convert data into the required format
	updates := make(map[string]interface{})

	// storing only the values that are changed
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.NIC != nil {
		updates["nic"] = *req.NIC
	}
	if req.ContactNumber != nil {
		updates["contact_no"] = *req.ContactNumber
	}
	if req.VehicleNumber != nil {
		updates["vehicle_no"] = *req.VehicleNumber
	}
	if req.VehicleType != nil {
		updates["vehicle_type"] = *req.VehicleType
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}

	// update the driver
	if err := h.loungeDriverRepo.UpdateDriver(driverID, updates); err != nil {
		// if error occured returning the server error
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "update_failed",
			Message: "Failed to update driver",
		})
		return
	}

	// returning the updated driver
	updatedDriver, _ := h.loungeDriverRepo.GetDriverByID(driverID)
	c.JSON(http.StatusOK, updatedDriver)

}
