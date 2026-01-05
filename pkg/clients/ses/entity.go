package ses

import (
	"context"
	"errors"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ses"
	"github.com/skolldire/go-engine/pkg/core/client"
	"github.com/skolldire/go-engine/pkg/utilities/resilience"
)

const (
	DefaultTimeout = 10 * time.Second
)

var (
	ErrSendEmail      = errors.New("error sending email")
	ErrInvalidInput   = errors.New("invalid input")
	ErrInvalidAddress = errors.New("invalid email address")
)

type Config struct {
	Region         string            `mapstructure:"region" json:"region"`
	EnableLogging  bool              `mapstructure:"enable_logging" json:"enable_logging"`
	WithResilience bool              `mapstructure:"with_resilience" json:"with_resilience"`
	Resilience     resilience.Config `mapstructure:"resilience" json:"resilience"`
	Timeout        time.Duration     `mapstructure:"timeout" json:"timeout"`
}

type EmailAddress struct {
	Email string
	Name  string
}

type EmailMessage struct {
	Subject     string
	BodyHTML    string
	BodyText    string
	From        EmailAddress
	To          []EmailAddress
	Cc          []EmailAddress
	Bcc         []EmailAddress
	ReplyTo     []EmailAddress
	Attachments []Attachment
}

type Attachment struct {
	Filename    string
	Content     []byte
	ContentType string
}

type SendEmailResult struct {
	MessageID string
}

type RecipientResult struct {
	EmailAddress string
	MessageID    string
	Error        error
}

type BulkSendResult struct {
	Recipients      []RecipientResult
	SuccessCount    int
	FailureCount    int
	FailedRecipients []string
}

type Service interface {
	// SendEmail sends a single email with HTML and/or text content.
	// Email addresses are validated before sending.
	SendEmail(ctx context.Context, message EmailMessage) (*SendEmailResult, error)

	// SendRawEmail sends a raw email message (RFC 2822 format).
	SendRawEmail(ctx context.Context, rawMessage []byte, destinations []string) (*SendEmailResult, error)

	// SendBulkEmail sends emails to multiple recipients.
	// Returns detailed results for each recipient including success/failure status.
	SendBulkEmail(ctx context.Context, from EmailAddress, subject string, htmlBody, textBody string, destinations []EmailAddress) (*BulkSendResult, error)

	// GetSendQuota retrieves the sending quota and rate limits.
	GetSendQuota(ctx context.Context) (*SendQuota, error)

	// GetSendStatistics retrieves sending statistics for the last 24 hours.
	GetSendStatistics(ctx context.Context) ([]SendDataPoint, error)

	// VerifyEmailAddress initiates verification of an email address.
	VerifyEmailAddress(ctx context.Context, email string) error

	// DeleteVerifiedEmailAddress removes a verified email address.
	DeleteVerifiedEmailAddress(ctx context.Context, email string) error

	// ListVerifiedEmailAddresses lists all verified email addresses.
	ListVerifiedEmailAddresses(ctx context.Context) ([]string, error)

	// EnableLogging enables or disables logging for this client.
	EnableLogging(enable bool)
}

type SendQuota struct {
	Max24HourSend   float64
	MaxSendRate     float64
	SentLast24Hours float64
}

type SendDataPoint struct {
	Timestamp        time.Time
	DeliveryAttempts int64
	Bounces          int64
	Complaints       int64
	Rejects          int64
}

type SESClient struct {
	*client.BaseClient
	sesClient *ses.Client
	region    string
}
