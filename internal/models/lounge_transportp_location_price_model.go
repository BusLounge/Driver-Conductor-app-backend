package models

import (
	 "time"
    "github.com/google/uuid"
)


type LoungeTransportLocationPrice struct{

	LoungeID uuid.UUID `db:"lounge_id" json:"lounge_id"`
	LocationID uuid.UUID `db:"location_id" json:"location_id"`
	ThreeWheelerPrice float64 `db:"three_wheeler_price" json:"three_wheeler_price"`
	CarPrice float64 `db:"car_price" json:"car_price"`
	VanPrice float64 `db:"van_price" json:"van_price"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`

}

