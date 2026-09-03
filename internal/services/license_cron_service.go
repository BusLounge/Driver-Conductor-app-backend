package services

import (
	"log"
	"time"

	"github.com/smarttransit/sms-auth-backend/internal/database"
)

// LicenseCronService handles daily license expiry checks and sends notifications
type LicenseCronService struct {
	staffRepo           *database.BusStaffRepository
	notificationService *NotificationService
	stopCh              chan struct{}
}

// NewLicenseCronService creates a new LicenseCronService
func NewLicenseCronService(
	staffRepo *database.BusStaffRepository,
	notificationService *NotificationService,
) *LicenseCronService {
	return &LicenseCronService{
		staffRepo:           staffRepo,
		notificationService: notificationService,
		stopCh:              make(chan struct{}),
	}
}

// Start begins the background license expiry check job
// It runs daily at 8:00 AM Sri Lanka time (Asia/Colombo)
func (s *LicenseCronService) Start() {
	log.Println("📋 Starting License Expiry Cron Service")
	go s.run()
}

// Stop stops the background license expiry check job
func (s *LicenseCronService) Stop() {
	log.Println("🛑 Stopping License Expiry Cron Service")
	close(s.stopCh)
}

func (s *LicenseCronService) run() {
	// Run an initial check on startup
	s.CheckAndNotifyExpiringLicenses()

	for {
		// Calculate duration until next 8:00 AM Sri Lanka time
		nextRun := s.nextRunTime()
		waitDuration := time.Until(nextRun)

		log.Printf("📋 License Expiry Cron: Next check scheduled at %s (in %s)",
			nextRun.Format("2006-01-02 15:04:05 MST"), waitDuration)

		select {
		case <-time.After(waitDuration):
			s.CheckAndNotifyExpiringLicenses()
		case <-s.stopCh:
			log.Println("License Expiry Cron Service stopped")
			return
		}
	}
}

// nextRunTime calculates the next 8:00 AM in Asia/Colombo timezone
func (s *LicenseCronService) nextRunTime() time.Time {
	loc, err := time.LoadLocation("Asia/Colombo")
	if err != nil {
		loc = time.FixedZone("Asia/Colombo", 5*3600+30*60) // UTC+5:30
	}

	now := time.Now().In(loc)
	next := time.Date(now.Year(), now.Month(), now.Day(), 8, 0, 0, 0, loc)

	// If 8:00 AM has already passed today, schedule for tomorrow
	if now.After(next) {
		next = next.AddDate(0, 0, 1)
	}

	return next
}

// CheckAndNotifyExpiringLicenses checks for staff with expiring licenses and sends notifications
func (s *LicenseCronService) CheckAndNotifyExpiringLicenses() {
	log.Println("📋 License Expiry Check: Starting daily check...")

	loc, err := time.LoadLocation("Asia/Colombo")
	if err != nil {
		loc = time.FixedZone("Asia/Colombo", 5*3600+30*60)
	}
	today := time.Now().In(loc)

	// 1. Check for licenses expiring in 30 days
	thirtyDaysFromNow := today.AddDate(0, 0, 30)
	s.processExpiryNotifications(thirtyDaysFromNow, LicenseExpiryReminder30Days)

	// 2. Check for licenses expiring tomorrow (1 day)
	tomorrow := today.AddDate(0, 0, 1)
	s.processExpiryNotifications(tomorrow, LicenseExpiryReminderFinal)

	// 3. Check for licenses that expired today
	s.processExpiryNotifications(today, LicenseExpired)

	log.Println("📋 License Expiry Check: Daily check completed")
}

// processExpiryNotifications queries staff with licenses expiring on targetDate and sends notifications
func (s *LicenseCronService) processExpiryNotifications(targetDate time.Time, notificationType LicenseExpiryNotificationType) {
	staffList, err := s.staffRepo.GetStaffWithLicenseExpiringOn(targetDate)
	if err != nil {
		log.Printf("ERROR: License Expiry Check: Failed to query staff for date %s: %v",
			targetDate.Format("2006-01-02"), err)
		return
	}

	if len(staffList) == 0 {
		log.Printf("📋 License Expiry Check (%s): No staff found with license expiring on %s",
			notificationType, targetDate.Format("2006-01-02"))
		return
	}

	log.Printf("📋 License Expiry Check (%s): Found %d staff with license expiring on %s",
		notificationType, len(staffList), targetDate.Format("2006-01-02"))

	for _, staff := range staffList {
		// Build staff name
		staffName := "Staff Member"
		if staff.FirstName != nil {
			staffName = *staff.FirstName
			if staff.LastName != nil {
				staffName += " " + *staff.LastName
			}
		}

		err := s.notificationService.SendLicenseExpiryNotification(
			staff.UserID,
			staffName,
			notificationType,
		)
		if err != nil {
			log.Printf("ERROR: License Expiry Check: Failed to notify staff %s (%s): %v",
				staff.ID, staffName, err)
			// Continue with other staff members
		}
	}
}
