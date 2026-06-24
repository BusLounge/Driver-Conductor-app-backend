//go:build ignore
// +build ignore

package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type Claims struct {
	UserID           uuid.UUID `json:"user_id"`
	Phone            string    `json:"phone"`
	Roles            []string  `json:"roles"`
	ProfileCompleted bool      `json:"profile_completed"`
	TokenType        string    `json:"token_type"`
	jwt.RegisteredClaims
}

func main() {
	userID := uuid.MustParse("4d54f70d-b75c-46fd-9f7a-82423e8a0c60")
	now := time.Now()
	claims := Claims{
		UserID:           userID,
		Phone:            "94771234567",
		Roles:            []string{"lounge_owner"},
		ProfileCompleted: true,
		TokenType:        "access",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "smarttransit-sms-auth",
			Subject:   userID.String(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte("your-256-bit-secret-key-here"))
	if err != nil {
		log.Fatalf("Error signing token: %v", err)
	}

	url := "http://localhost:8080/api/v1/lounges/e528941b-3c29-4b4e-87e8-6c2ae4ea8c34/special-packages"
	jsonBody := []byte(`{
		"package_name": "Premium Lounge Package",
		"package_type": "platinum",
		"description": "Premium lounge access with meals and transport",
		"price": "1500.00",
		"pax": 2,
		"transport_status": true,
		"transport_mode": "three-wheeler",
		"meal_status": true,
		"breakfast_status": true,
		"breakfast_type": ["Sri Lankan Breakfast", "Continental"],
		"places": ["Temple", "Lake"]
	}`)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		log.Fatalf("Error creating request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenString)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Error executing request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Error reading response body: %v", err)
	}

	fmt.Printf("Local Response Status: %d\n", resp.StatusCode)
	fmt.Printf("Local Response Body: %s\n", string(body))
}
