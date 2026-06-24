package handlers

import (
	"errors"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/smarttransit/sms-auth-backend/internal/database"
	"github.com/smarttransit/sms-auth-backend/internal/middleware"
	"github.com/smarttransit/sms-auth-backend/internal/services"
)

// StorageHandler manages image uploads backed by Cloudinary.
type StorageHandler struct {
	cloudinaryService *services.CloudinaryStorageService
	loungeOwnerRepo   *database.LoungeOwnerRepository
	loungeRepo        *database.LoungeRepository
}

// NewStorageHandler creates a new storage handler.
func NewStorageHandler(
	cloudinaryService *services.CloudinaryStorageService,
	loungeOwnerRepo *database.LoungeOwnerRepository,
	loungeRepo *database.LoungeRepository,
) *StorageHandler {
	return &StorageHandler{
		cloudinaryService: cloudinaryService,
		loungeOwnerRepo:   loungeOwnerRepo,
		loungeRepo:        loungeRepo,
	}
}

type deleteImageRequest struct {
	URL string `json:"url" binding:"required"`
}

func (h *StorageHandler) UploadLoungePhoto(c *gin.Context) {
	if err := h.authorizeLoungeUpload(c); err != nil {
		return
	}
	fileHeader, err := c.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "validation_error", Message: "image file is required"})
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "upload_failed", Message: "failed to read uploaded file"})
		return
	}
	defer closeMultipartFile(file)

	loungeID := c.Param("lounge_id")
	result, err := h.cloudinaryService.UploadLoungePhoto(c.Request.Context(), file, fileHeader.Filename, loungeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "upload_failed", Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"url":       result.SecureURL,
		"public_id": result.PublicID,
		"asset_id":  result.AssetID,
		"entity":    "lounge_photo",
		"lounge_id": loungeID,
		"filename":  fileHeader.Filename,
	})
}

func (h *StorageHandler) UploadProductImage(c *gin.Context) {
	if err := h.authorizeLoungeUpload(c); err != nil {
		return
	}
	fileHeader, err := c.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "validation_error", Message: "image file is required"})
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "upload_failed", Message: "failed to read uploaded file"})
		return
	}
	defer closeMultipartFile(file)

	loungeID := c.Param("lounge_id")
	result, err := h.cloudinaryService.UploadProductImage(c.Request.Context(), file, fileHeader.Filename, loungeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "upload_failed", Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"url":       result.SecureURL,
		"public_id": result.PublicID,
		"asset_id":  result.AssetID,
		"entity":    "product_image",
		"lounge_id": loungeID,
		"filename":  fileHeader.Filename,
	})
}

func (h *StorageHandler) UploadSpecialPackageImage(c *gin.Context) {
	if err := h.authorizeLoungeUpload(c); err != nil {
		return
	}
	fileHeader, err := c.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "validation_error", Message: "image file is required"})
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "upload_failed", Message: "failed to read uploaded file"})
		return
	}
	defer closeMultipartFile(file)

	loungeID := c.Param("lounge_id")
	result, err := h.cloudinaryService.UploadSpecialPackageImage(c.Request.Context(), file, fileHeader.Filename, loungeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "upload_failed", Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"url":       result.SecureURL,
		"public_id": result.PublicID,
		"asset_id":  result.AssetID,
		"entity":    "special_package_image",
		"lounge_id": loungeID,
		"filename":  fileHeader.Filename,
	})
}

func (h *StorageHandler) UploadManagerNICImage(c *gin.Context) {
	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "unauthorized", Message: "User context not found"})
		return
	}

	userID := c.Param("user_id")
	if userID != userCtx.UserID.String() {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "forbidden", Message: "You can only upload NIC images for your own account"})
		return
	}

	side := strings.TrimSpace(c.Param("side"))
	if side == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "validation_error", Message: "side is required"})
		return
	}

	fileHeader, err := c.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "validation_error", Message: "image file is required"})
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "upload_failed", Message: "failed to read uploaded file"})
		return
	}
	defer closeMultipartFile(file)

	result, err := h.cloudinaryService.UploadNICImage(c.Request.Context(), file, fileHeader.Filename, userID, side)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "upload_failed", Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"url":       result.SecureURL,
		"public_id": result.PublicID,
		"asset_id":  result.AssetID,
		"entity":    "manager_nic",
		"user_id":   userID,
		"side":      side,
	})
}

func (h *StorageHandler) DeleteImage(c *gin.Context) {
	var req deleteImageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "validation_error", Message: "Invalid request body"})
		return
	}

	if h.cloudinaryService == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "storage_unavailable", Message: "Cloudinary storage is not configured"})
		return
	}

	if err := h.cloudinaryService.DeleteImageByURL(c.Request.Context(), req.URL); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "delete_failed", Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (h *StorageHandler) authorizeLoungeUpload(c *gin.Context) error {
	if h.cloudinaryService == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "storage_unavailable", Message: "Cloudinary storage is not configured"})
		return errors.New("cloudinary storage not configured")
	}

	userCtx, exists := middleware.GetUserContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "unauthorized", Message: "User context not found"})
		return errors.New("user context not found")
	}

	loungeID := strings.TrimSpace(c.Param("lounge_id"))
	parsedLoungeID, err := uuid.Parse(loungeID)
	if err != nil {
		return nil
	}

	if h.loungeRepo == nil || h.loungeOwnerRepo == nil {
		return nil
	}

	lounge, err := h.loungeRepo.GetLoungeByID(parsedLoungeID)
	if err != nil || lounge == nil {
		return nil
	}

	owner, err := h.loungeOwnerRepo.GetLoungeOwnerByUserID(userCtx.UserID)
	if err != nil || owner == nil || lounge.LoungeOwnerID != owner.ID {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "forbidden", Message: "Not authorized to upload images for this lounge"})
		return errors.New("forbidden")
	}

	return nil
}

func closeMultipartFile(file multipart.File) {
	_ = file.Close()
}
