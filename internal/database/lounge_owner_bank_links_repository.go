package database

import (
    "fmt"
    "time"

    "github.com/google/uuid"
    "github.com/jmoiron/sqlx"
    "github.com/smarttransit/sms-auth-backend/internal/models"
)

// LoungeOwnerBankLinkRepository handles bank links
type LoungeOwnerBankLinkRepository struct {
    db *sqlx.DB
}

// NewLoungeOwnerBankLinkRepository creates repository
func NewLoungeOwnerBankLinkRepository(db *sqlx.DB) *LoungeOwnerBankLinkRepository {
    return &LoungeOwnerBankLinkRepository{db: db}
}

// Create inserts a new link
func (r *LoungeOwnerBankLinkRepository) Create(link *models.LoungeOwnerBankLink) (*models.LoungeOwnerBankLink, error) {
    id := uuid.New()
    query := `INSERT INTO lounge_owner_bank_links (id, owner_id, lounge_id, bank_details_id, created_at)
        VALUES ($1,$2,$3,$4,NOW()) RETURNING created_at`

    var createdAt time.Time
    err := r.db.QueryRowx(query, id, link.OwnerID, link.LoungeID, link.BankDetailsID).Scan(&createdAt)
    if err != nil {
        return nil, fmt.Errorf("failed to create bank link: %w", err)
    }
    link.ID = id
    link.CreatedAt = createdAt
    return link, nil
}

// GetByID retrieves a bank link
func (r *LoungeOwnerBankLinkRepository) GetByID(id uuid.UUID) (*models.LoungeOwnerBankLink, error) {
    var l models.LoungeOwnerBankLink
    query := `SELECT id, owner_id, lounge_id, bank_details_id, created_at FROM lounge_owner_bank_links WHERE id = $1`
    err := r.db.QueryRowx(query, id).Scan(&l.ID, &l.OwnerID, &l.LoungeID, &l.BankDetailsID, &l.CreatedAt)
    if err != nil {
        return nil, nil
    }
    return &l, nil
}

// ListByOwner lists bank links for an owner
func (r *LoungeOwnerBankLinkRepository) ListByOwner(ownerID uuid.UUID) ([]models.LoungeOwnerBankLink, error) {
    var links []models.LoungeOwnerBankLink
    query := `SELECT id, owner_id, lounge_id, bank_details_id, created_at FROM lounge_owner_bank_links WHERE owner_id = $1 ORDER BY created_at DESC`
    err := r.db.Select(&links, query, ownerID)
    if err != nil {
        return nil, fmt.Errorf("failed to list bank links: %w", err)
    }
    return links, nil
}

// Delete deletes a link by id
func (r *LoungeOwnerBankLinkRepository) Delete(id uuid.UUID) error {
    query := `DELETE FROM lounge_owner_bank_links WHERE id = $1`
    _, err := r.db.Exec(query, id)
    if err != nil {
        return fmt.Errorf("failed to delete bank link: %w", err)
    }
    return nil
}
