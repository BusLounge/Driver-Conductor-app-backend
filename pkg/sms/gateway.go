package sms

// SMSGateway defines the interface for sending SMS messages
type SMSGateway interface {
	// SendOTP sends an OTP code via SMS
	// Returns a transaction ID and an error if the send failed
	SendOTP(phone, otpCode, appType string) (int64, error)

	// SendMessage sends a plain SMS message
	// Returns a transaction ID and an error if the send failed
	SendMessage(phone, message string) (int64, error)

	// GetName returns the name of the SMS gateway implementation
	GetName() string
}
