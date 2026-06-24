package database

import (
	"database/sql"
	"fmt"
	// "time"
	"log"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/smarttransit/sms-auth-backend/internal/models"
)

// LoungeTransportLocationPriceRepository handles database operations for lounge transport location prices
type LoungeTransportLocationPriceRepository struct{
	db *sqlx.DB
}

//NewLoungeTransportLocationPriceRepository creates a new lounge staff repository
func NewLoungeTransportLocationPriceRepository(db *sqlx.DB) *LoungeTransportLocationPriceRepository{
	return &LoungeTransportLocationPriceRepository{db: db}
}

// add lounge transport location prices
func (r *LoungeTransportLocationPriceRepository) SetLoungeTransportLocationPrices(
	loungeID, locationID uuid.UUID,
	threeWheelerPrice, carPrice, vanPrice float64,
)(*models.LoungeTransportLocationPrice,error){

	query :=`
			INSERT INTO lounge_transport_location_prices
			(lounge_id,location_id,three_wheeler_price,car_price,van_price)
			VALUES($1,$2,$3,$4,$5)
			ON CONFLICT (lounge_id, location_id) 
            DO UPDATE SET
			three_wheeler_price = EXCLUDED.three_wheeler_price,
            car_price = EXCLUDED.car_price,
            van_price = EXCLUDED.van_price,
            updated_at = NOW()
			RETURNING lounge_id, location_id, three_wheeler_price, car_price, van_price, updated_at`

	// creating the variable to store returning struct
	var price models.LoungeTransportLocationPrice
	// approaching the return and query execution in a new and proffesional way (use this way in AddDriver and AddTransportLocation repos as well)
	err := r.db.Get(&price, query, loungeID, locationID, threeWheelerPrice, carPrice, vanPrice)
	if err != nil {
		log.Printf("ERROR: Failed to set prices for location %s in lounge %s: %v", locationID, loungeID, err)
        return nil, fmt.Errorf("failed to set prices: %w", err)
	}

	return &price, nil
}



// get lounge transport location prices for a specific lounge
func (r *LoungeTransportLocationPriceRepository) GetLoungeTransportLocationPrices(
	loungeID,locationID uuid.UUID,
)(*models.LoungeTransportLocationPrice,error){
	// creating the variable to store returning struct
	var price models.LoungeTransportLocationPrice
	query := `
        SELECT lounge_id, location_id, three_wheeler_price, car_price, van_price, updated_at
        FROM lounge_transport_location_prices
        WHERE lounge_id = $1 AND location_id = $2`


	err := r.db.Get(&price, query, loungeID, locationID)
	if err == sql.ErrNoRows {
        return nil, nil // No prices set yet
    }
	if err != nil {
        log.Printf("ERROR: Failed to get prices for location %s in lounge %s: %v", locationID, loungeID, err)
        return nil, fmt.Errorf("failed to get prices: %w", err)
    }

	return &price,nil
}






















