package utils

import (
	"fmt"
	"math/rand"
	"time"
)

// GenerateTripID generates a unique trip ID in format: TRIP-YYYYMMDD-XXXXX
// Example: TRIP-20260518-A7K9M
func GenerateTripID() string {
	now := time.Now()
	date := now.Format("20060102") // YYYYMMDD format

	// Generate 5 random alphanumeric characters (uppercase)
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	randomPart := make([]byte, 5)
	for i := range randomPart {
		randomPart[i] = charset[rand.Intn(len(charset))]
	}

	return fmt.Sprintf("TRIP-%s-%s", date, string(randomPart))
}

// GenerateTripIDWithBookingRef generates a Trip ID based on booking reference
// Example: TRIP-BR123ABC-001 (useful for tracking)
func GenerateTripIDWithBookingRef(bookingRef string) string {
	const charset = "0123456789"
	randomPart := make([]byte, 3)
	for i := range randomPart {
		randomPart[i] = charset[rand.Intn(len(charset))]
	}

	// Take first 6 characters of booking reference if available
	if len(bookingRef) > 6 {
		bookingRef = bookingRef[:6]
	}

	return fmt.Sprintf("TRIP-%s-%s", bookingRef, string(randomPart))
}
