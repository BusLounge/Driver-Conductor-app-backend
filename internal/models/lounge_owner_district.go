package models

import "github.com/google/uuid"

// LoungeOwnerDistrict represents owner-business mapping by district.
type LoungeOwnerDistrict struct {
	ID           uuid.UUID `db:"id" json:"id"`
	OwnerID      uuid.UUID `db:"owner_id" json:"owner_id"`
	DistrictID   uuid.UUID `db:"district_id" json:"district_id"`
	OwnerName    string    `db:"owner_name" json:"owner_name"`
	BusinessName string    `db:"business_name" json:"business_name"`
}
