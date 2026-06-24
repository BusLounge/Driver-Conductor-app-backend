package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// LOUNGE SPECIAL PACKAGE (lounge_special_packages table)
// ============================================================================

// LoungeSpecialPackageType represents the package tier
type LoungeSpecialPackageType string

const (
	LoungeSpecialPackageTypePlatinum LoungeSpecialPackageType = "platinum"
	LoungeSpecialPackageTypeGold     LoungeSpecialPackageType = "gold"
	LoungeSpecialPackageTypeStandard LoungeSpecialPackageType = "standard"
)

// TransportMode represents the transport mode options
type TransportMode string

const (
	TransportModeThreeWheeler TransportMode = "three-wheeler"
	TransportModeVan          TransportMode = "van"
	TransportModeCar          TransportMode = "car"
)

// IsValidTransportMode checks if a transport mode string is valid
func IsValidTransportMode(m string) bool {
	switch TransportMode(m) {
	case TransportModeThreeWheeler, TransportModeVan, TransportModeCar:
		return true
	}
	return false
}

// LoungeSpecialPackage represents a special package offered by a lounge
type LoungeSpecialPackage struct {
	ID          uuid.UUID                `db:"id"           json:"id"`
	LoungeID    uuid.UUID                `db:"lounge_id"    json:"lounge_id"`
	PackageName string                   `db:"package_name" json:"package_name"`
	ImageURL    *string                  `db:"image_url"    json:"image_url,omitempty"`
	PackageType LoungeSpecialPackageType `db:"package_type" json:"package_type"`
	Description string                   `db:"description"  json:"description"`
	Price       string                   `db:"price"        json:"price"` // DECIMAL(10,2) as string
	IsActive    bool                     `db:"is_active"    json:"is_active"`
	CreatedAt   time.Time                `db:"created_at"   json:"created_at"`
	UpdatedAt   time.Time                `db:"updated_at"   json:"updated_at"`

	// Extended fields (new schema)
	// Note: meal/breakfast/lunch/snack/dinner columns use hyphens in the DB (e.g. "meal-status")
	Pax                *int64           `db:"pax"                json:"pax,omitempty"`
	TransportStatus    *bool            `db:"transport_status"   json:"transport_status,omitempty"`
	TransportMode      *string          `db:"transport_mode"     json:"transport_mode,omitempty"`
	MealStatus         *bool            `db:"meal-status"        json:"meal_status,omitempty"`
	BreakfastStatus    *bool            `db:"breakfast-status"   json:"breakfast_status,omitempty"`
	BreakfastType      json.RawMessage  `db:"breakfast-type"     json:"breakfast_type,omitempty"`
	LunchStatus        *bool            `db:"lunch-status"       json:"lunch_status,omitempty"`
	LunchType          json.RawMessage  `db:"lunch-type"         json:"lunch_type,omitempty"`
	EveningSnackStatus *bool            `db:"evening-snack-status" json:"evening_snack_status,omitempty"`
	EveningSnackType   json.RawMessage  `db:"evening-snack-type"   json:"evening_snack_type,omitempty"`
	DinnerStatus       *bool            `db:"dinner-status"      json:"dinner_status,omitempty"`
	DinnerType         json.RawMessage  `db:"dinner-type"        json:"dinner_type,omitempty"`
	Places             json.RawMessage  `db:"places"             json:"places,omitempty"`
}
