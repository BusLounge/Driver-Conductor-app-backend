package models

import (
    "time"

    "github.com/google/uuid"
)

type LoungeTransportLocation struct {
    // Basic details of the transport location
    ID        uuid.UUID      `db:"id" json:"id"`
    LoungeID  uuid.UUID      `db:"lounge_id" json:"lounge_id"`
    Location  string         `db:"location" json:"location"`        // e.g., "Colombo Fort Station"
    Latitude  float64        `db:"latitude" json:"latitude"`        // e.g., 6.9339
    Longitude float64        `db:"longitude" json:"longitude"`      // e.g., 79.8500
    
    // LocationStatus defines the availability of a transport location
    Status    LoungeTransportLocationStatus `db:"status" json:"status"`

    // adding estimated duration in minutes(NEW)
    EstDuration int          `db:"est_duration" json:"est_duration"`

    // adding distance from lounge 
    Distance float64        `db:"distance" json:"distance"`

    // CreatedAt and UpdatedAt for maintenance
    CreatedAt time.Time      `db:"created_at" json:"created_at"`
    UpdatedAt time.Time      `db:"updated_at" json:"updated_at"`
}

// Location status enum
type LoungeTransportLocationStatus string

const (
    LoungeTransportLocationStatusActive   LoungeTransportLocationStatus = "active"
    LoungeTransportLocationStatusInactive LoungeTransportLocationStatus = "inactive"
)