package services

import (
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/smarttransit/sms-auth-backend/internal/database"
	"github.com/smarttransit/sms-auth-backend/pkg/onesignal"
)

// NotificationService handles notification operations
type NotificationService struct {
	oneSignalService  *onesignal.OneSignalService
	userSessionRepo   *database.UserSessionRepository
	loungeRepo        *database.LoungeRepository
	loungeOwnerRepo   *database.LoungeOwnerRepository
}

// NewNotificationService creates a new notification service
func NewNotificationService(
	oneSignalService *onesignal.OneSignalService,
	userSessionRepo *database.UserSessionRepository,
	loungeRepo *database.LoungeRepository,
	loungeOwnerRepo *database.LoungeOwnerRepository,
) *NotificationService {
	return &NotificationService{
		oneSignalService: oneSignalService,
		userSessionRepo:  userSessionRepo,
		loungeRepo:       loungeRepo,
		loungeOwnerRepo:  loungeOwnerRepo,
	}
}

// NotifyLoungeOwnerNewStaff sends notification to lounge owner when new staff registers
func (s *NotificationService) NotifyLoungeOwnerNewStaff(
	loungeID uuid.UUID,
	staffName string,
	staffPhone string,
	staffNIC string,
) error {
	// 1. Get lounge details
	lounge, err := s.loungeRepo.GetLoungeByID(loungeID)
	if err != nil || lounge == nil {
		return fmt.Errorf("failed to get lounge: %w", err)
	}

	// 2. Get lounge owner details
	owner, err := s.loungeOwnerRepo.GetLoungeOwnerByID(lounge.LoungeOwnerID)
	if err != nil || owner == nil {
		return fmt.Errorf("failed to get lounge owner: %w", err)
	}

	// 3. Get owner's active sessions (OneSignal player IDs)
	sessions, err := s.userSessionRepo.GetActiveSessionsByUserID(owner.UserID)
	if err != nil {
		return fmt.Errorf("failed to get owner sessions: %w", err)
	}

	if len(sessions) == 0 {
		log.Printf("INFO: No active sessions found for lounge owner %s", owner.UserID)
		return nil // Not an error - owner might not be logged in
	}

	// 4. Extract OneSignal player IDs (stored in onesignal_player_id column)
	var playerIDs []string
	for _, session := range sessions {
		if session.OneSignalPlayerID.Valid && session.OneSignalPlayerID.String != "" {
			playerIDs = append(playerIDs, session.OneSignalPlayerID.String)
		}
	}

	if len(playerIDs) == 0 {
		log.Printf("INFO: No OneSignal player IDs found for lounge owner %s", owner.UserID)
		return nil // Not an error - owner might not have enabled notifications
	}

	// 5. Prepare notification
	title := "New Staff Registration"
	body := fmt.Sprintf("%s has registered as staff for your lounge", staffName)

	// 6. Prepare data payload (for custom handling in app)
	data := map[string]interface{}{
		"type":        "new_staff_registration",
		"lounge_id":   loungeID.String(),
		"staff_name":  staffName,
		"staff_phone": staffPhone,
		"staff_nic":   staffNIC,
		"action":      "review_staff",
		"redirect_to": "staff_pending_list",
	}

	// 7. Send notification via OneSignal
	resp, err := s.oneSignalService.SendToPlayers(playerIDs, title, body, data)
	if err != nil {
		log.Printf("ERROR: Failed to send OneSignal notification: %v", err)
		return fmt.Errorf("failed to send notification: %w", err)
	}

	log.Printf("INFO: Sent notification to lounge owner %s - Recipients: %d, Notification ID: %s",
		owner.UserID, resp.Recipients, resp.ID)

	return nil
}