package onesignal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
)

// OneSignalService handles push notification operations via OneSignal
type OneSignalService struct {
	appID      string
	restAPIKey string
	apiURL     string
}

// NewOneSignalService creates a new OneSignal service
func NewOneSignalService(appID, restAPIKey string) *OneSignalService {
	return &OneSignalService{
		appID:      appID,
		restAPIKey: restAPIKey,
		apiURL:     "https://onesignal.com/api/v1/notifications",
	}
}

// NotificationPayload represents the notification structure
type NotificationPayload struct {
	AppID            string                 `json:"app_id"`
	IncludePlayerIDs []string               `json:"include_player_ids,omitempty"`
	Headings         map[string]string      `json:"headings"`
	Contents         map[string]string      `json:"contents"`
	Data             map[string]interface{} `json:"data,omitempty"`
	Priority         int                    `json:"priority,omitempty"`
	IOSBadgeType     string                 `json:"ios_badgeType,omitempty"`
	IOSBadgeCount    int                    `json:"ios_badgeCount,omitempty"`
}

// OneSignalResponse represents the response from OneSignal API
type OneSignalResponse struct {
	ID         string   `json:"id"`
	Recipients int      `json:"recipients"`
	Errors     []string `json:"errors,omitempty"`
}

// SendToPlayers sends a notification to specific player IDs
func (s *OneSignalService) SendToPlayers(
	playerIDs []string,
	title string,
	body string,
	data map[string]interface{},
) (*OneSignalResponse, error) {
	if len(playerIDs) == 0 {
		return nil, fmt.Errorf("no player IDs provided")
	}

	payload := NotificationPayload{
		AppID:            s.appID,
		IncludePlayerIDs: playerIDs,
		Headings: map[string]string{
			"en": title,
		},
		Contents: map[string]string{
			"en": body,
		},
		Data:          data,
		Priority:      10, // High priority
		IOSBadgeType:  "Increase",
		IOSBadgeCount: 1,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal notification payload: %w", err)
	}

	req, err := http.NewRequest("POST", s.apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Basic "+s.restAPIKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send OneSignal request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read OneSignal response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OneSignal returned non-200 status: %d, body: %s", resp.StatusCode, string(respBody))
	}

	var osResp OneSignalResponse
	if err := json.Unmarshal(respBody, &osResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal OneSignal response: %w", err)
	}

	// Check for errors in response
	if len(osResp.Errors) > 0 {
		return &osResp, fmt.Errorf("OneSignal errors: %v", osResp.Errors)
	}

	return &osResp, nil
}

// SendToSinglePlayer sends a notification to a single player
func (s *OneSignalService) SendToSinglePlayer(
	playerID string,
	title string,
	body string,
	data map[string]interface{},
) error {
	_, err := s.SendToPlayers([]string{playerID}, title, body, data)
	return err
}
