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

// LoungeOwnerBankHandler handles bank details and link APIs
type LoungeOwnerBankHandler struct {
    bankRepo        *database.LoungeOwnerBankDetailsRepository
    linkRepo        *database.LoungeOwnerBankLinkRepository
    loungeOwnerRepo *database.LoungeOwnerRepository
}

// NewLoungeOwnerBankHandler creates handler
func NewLoungeOwnerBankHandler(
    bankRepo *database.LoungeOwnerBankDetailsRepository,
    linkRepo *database.LoungeOwnerBankLinkRepository,
    loungeOwnerRepo *database.LoungeOwnerRepository,
) *LoungeOwnerBankHandler {
    return &LoungeOwnerBankHandler{
        bankRepo:        bankRepo,
        linkRepo:        linkRepo,
        loungeOwnerRepo: loungeOwnerRepo,
    }
}

func (h *LoungeOwnerBankHandler) getOwnerIDFromUser(c *gin.Context) (uuid.UUID, bool) {
    userCtx, ok := middleware.GetUserContext(c)
    if !ok {
        c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "unauthorized", Message: "User context not found"})
        return uuid.Nil, false
    }

    owner, err := h.loungeOwnerRepo.GetLoungeOwnerByUserID(userCtx.UserID)
    if err != nil {
        log.Printf("ERROR: failed to get lounge owner by user id %s: %v", userCtx.UserID, err)
        c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "database_error", Message: "Failed to resolve lounge owner"})
        return uuid.Nil, false
    }

    if owner == nil {
        c.JSON(http.StatusForbidden, ErrorResponse{Error: "not_lounge_owner", Message: "Lounge owner account not found"})
        return uuid.Nil, false
    }

    return owner.ID, true
}

type createBankDetailsRequest struct {
    BankName      string  `json:"bank_name" binding:"required"`
    BranchName    string  `json:"branch_name" binding:"required"`
    BranchCode    string  `json:"branch_code" binding:"required"`
    ACType        string  `json:"ac_type" binding:"required"`
    ACHolderName  string  `json:"ac_holder_name" binding:"required"`
    ACNumber      string  `json:"ac_number" binding:"required"`
    SwiftCode     *string `json:"swift_code"`
}

// CreateBankDetails handles POST /api/v1/lounge-owner/bank-details
func (h *LoungeOwnerBankHandler) CreateBankDetails(c *gin.Context) {
    // require auth
    userCtx, ok := middleware.GetUserContext(c)
    if !ok {
        c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "unauthorized", Message: "User context not found"})
        return
    }

    var req createBankDetailsRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, ErrorResponse{Error: "validation_error", Message: err.Error()})
        return
    }

    // Create model (plaintext fields)
    model := &models.LoungeOwnerBankDetails{
        BankName: req.BankName,
        BranchName: req.BranchName,
        BranchCode: req.BranchCode,
        ACType: req.ACType,
        ACHolderName: req.ACHolderName,
        ACNumber: req.ACNumber,
        SwiftCode: req.SwiftCode,
    }

    created, err := h.bankRepo.Create(model)
    if err != nil {
        log.Printf("ERROR: failed to create bank details for user %s: %v", userCtx.UserID, err)
        c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "database_error", Message: "Failed to create bank details"})
        return
    }

    c.JSON(http.StatusCreated, gin.H{"bank_details": created})
}

// GetBankDetails handles GET /api/v1/lounge-owner/bank-details/:id
func (h *LoungeOwnerBankHandler) GetBankDetails(c *gin.Context) {
    idStr := c.Param("id")
    id, err := uuid.Parse(idStr)
    if err != nil {
        c.JSON(http.StatusBadRequest, ErrorResponse{Error: "validation_error", Message: "invalid id"})
        return
    }

    d, err := h.bankRepo.GetByID(id)
    if err != nil {
        log.Printf("ERROR: get bank details: %v", err)
        c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "database_error", Message: "Failed to get bank details"})
        return
    }
    if d == nil {
        c.JSON(http.StatusNotFound, ErrorResponse{Error: "not_found", Message: "bank details not found"})
        return
    }

    c.JSON(http.StatusOK, gin.H{"bank_details": d})
}

// UpdateBankDetails handles PUT /api/v1/lounge-owner/bank-details/:id
func (h *LoungeOwnerBankHandler) UpdateBankDetails(c *gin.Context) {
    userCtx, ok := middleware.GetUserContext(c)
    if !ok {
        c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "unauthorized", Message: "User context not found"})
        return
    }

    idStr := c.Param("id")
    id, err := uuid.Parse(idStr)
    if err != nil {
        c.JSON(http.StatusBadRequest, ErrorResponse{Error: "validation_error", Message: "invalid id"})
        return
    }

    var req createBankDetailsRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, ErrorResponse{Error: "validation_error", Message: err.Error()})
        return
    }

    model := &models.LoungeOwnerBankDetails{
        ID: id,
        BankName: req.BankName,
        BranchName: req.BranchName,
        BranchCode: req.BranchCode,
        ACType: req.ACType,
        ACHolderName: req.ACHolderName,
        ACNumber: req.ACNumber,
        SwiftCode: req.SwiftCode,
    }

    if err := h.bankRepo.Update(model); err != nil {
        log.Printf("ERROR: update bank details for user %s: %v", userCtx.UserID, err)
        c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "database_error", Message: "Failed to update bank details"})
        return
    }

    c.JSON(http.StatusOK, gin.H{"message": "updated"})
}

// DeleteBankDetails handles DELETE /api/v1/lounge-owner/bank-details/:id
func (h *LoungeOwnerBankHandler) DeleteBankDetails(c *gin.Context) {
    userCtx, ok := middleware.GetUserContext(c)
    if !ok {
        c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "unauthorized", Message: "User context not found"})
        return
    }

    idStr := c.Param("id")
    id, err := uuid.Parse(idStr)
    if err != nil {
        c.JSON(http.StatusBadRequest, ErrorResponse{Error: "validation_error", Message: "invalid id"})
        return
    }

    if err := h.bankRepo.Delete(id); err != nil {
        log.Printf("ERROR: delete bank details for user %s: %v", userCtx.UserID, err)
        c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "database_error", Message: "Failed to delete bank details"})
        return
    }

    c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// CreateBankLinkRequest represents creating a link between owner/lounge and bank details
type CreateBankLinkRequest struct {
    LoungeID      *string `json:"lounge_id"`
    BankDetailsID string  `json:"bank_details_id" binding:"required"`
}

// CreateBankLink handles POST /api/v1/lounge-owner/bank-links
func (h *LoungeOwnerBankHandler) CreateBankLink(c *gin.Context) {
    ownerID, ok := h.getOwnerIDFromUser(c)
    if !ok {
        return
    }

    var req CreateBankLinkRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, ErrorResponse{Error: "validation_error", Message: err.Error()})
        return
    }

    bankID, err := uuid.Parse(req.BankDetailsID)
    if err != nil {
        c.JSON(http.StatusBadRequest, ErrorResponse{Error: "validation_error", Message: "invalid bank_details_id"})
        return
    }

    var loungeUUID *uuid.UUID
    if req.LoungeID != nil && *req.LoungeID != "" {
        parsed, err := uuid.Parse(*req.LoungeID)
        if err != nil {
            c.JSON(http.StatusBadRequest, ErrorResponse{Error: "validation_error", Message: "invalid lounge_id"})
            return
        }
        loungeUUID = &parsed
    }

    link := &models.LoungeOwnerBankLink{
        OwnerID: ownerID,
        LoungeID: loungeUUID,
        BankDetailsID: bankID,
    }

    created, err := h.linkRepo.Create(link)
    if err != nil {
        log.Printf("ERROR: create bank link for owner %s: %v", ownerID, err)
        c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "database_error", Message: "Failed to create bank link"})
        return
    }

    c.JSON(http.StatusCreated, gin.H{"bank_link": created})
}

// ListBankLinks handles GET /api/v1/lounge-owner/bank-links
func (h *LoungeOwnerBankHandler) ListBankLinks(c *gin.Context) {
    ownerID, ok := h.getOwnerIDFromUser(c)
    if !ok {
        return
    }

    links, err := h.linkRepo.ListByOwner(ownerID)
    if err != nil {
        log.Printf("ERROR: list bank links for owner %s: %v", ownerID, err)
        c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "database_error", Message: "Failed to list bank links"})
        return
    }

    // Enrich with bank details
    resp := make([]gin.H, 0, len(links))
    for _, l := range links {
        bd, err := h.bankRepo.GetByID(l.BankDetailsID)
        if err != nil {
            log.Printf("WARN: failed to fetch bank details for link %s: %v", l.ID, err)
            continue
        }
        resp = append(resp, gin.H{"link": l, "bank_details": bd})
    }

    c.JSON(http.StatusOK, gin.H{"bank_links": resp})
}

// DeleteBankLink handles DELETE /api/v1/lounge-owner/bank-links/:id
func (h *LoungeOwnerBankHandler) DeleteBankLink(c *gin.Context) {
    userCtx, ok := middleware.GetUserContext(c)
    if !ok {
        c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "unauthorized", Message: "User context not found"})
        return
    }

    idStr := c.Param("id")
    id, err := uuid.Parse(idStr)
    if err != nil {
        c.JSON(http.StatusBadRequest, ErrorResponse{Error: "validation_error", Message: "invalid id"})
        return
    }

    if err := h.linkRepo.Delete(id); err != nil {
        log.Printf("ERROR: delete bank link for user %s: %v", userCtx.UserID, err)
        c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "database_error", Message: "Failed to delete bank link"})
        return
    }

    c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}
