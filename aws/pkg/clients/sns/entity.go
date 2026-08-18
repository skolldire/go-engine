package sns

import (
	"context"
	"errors"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/skolldire/go-engine/pkg/core/client"
	"github.com/skolldire/go-engine/pkg/utilities/resilience"
)

const (
	DefaultTimeout = 5 * time.Second
)

var (
	ErrPublication            = errors.New("error publishing message")
	ErrSubscription           = errors.New("error creating subscription")
	ErrCreateTopic            = errors.New("error creating topic")
	ErrDeleteTopic            = errors.New("error deleting topic")
	ErrListTopics             = errors.New("error listing topics")
	ErrInvalidInput           = errors.New("invalid input")
	ErrSMSFailed              = errors.New("error sending SMS")
	ErrCreatePlatformApp      = errors.New("error creating platform application")
	ErrCreatePlatformEndpoint = errors.New("error creating platform endpoint")
	ErrDeleteEndpoint         = errors.New("error deleting endpoint")
)

type Config struct {
	BaseEndpoint   string            `mapstructure:"base_endpoint" json:"base_endpoint"`
	EnableLogging  bool              `mapstructure:"enable_logging" json:"enable_logging"`
	WithResilience bool              `mapstructure:"with_resilience" json:"with_resilience"`
	Resilience     resilience.Config `mapstructure:"resilience" json:"resilience"`
}

type BulkSMSResult struct {
	Successful []SMSResult
	Failed     []SMSResult
}

type SMSResult struct {
	PhoneNumber string
	MessageID   string
	Status      string
	Error       error
}

type PlatformApplication struct {
	PlatformApplicationArn string
	Attributes             map[string]string
}

type Endpoint struct {
	EndpointArn string
	Attributes  map[string]string
}

// The interface is split by concern rather than exposed as one block of
// twenty-four methods. SNS covers four unrelated jobs — pub/sub topics, SMS,
// and mobile push — and a caller that only publishes to a topic should not
// depend on endpoint management to do it.
//
// Service still composes all of them, so nothing that held a Service breaks;
// what changes is that a consumer can now depend on the two methods it uses.

// TopicManager creates and lists SNS topics.
type TopicManager interface {
	CreateTopic(ctx context.Context, name string, atributos map[string]string) (string, error)
	DeleteTopic(ctx context.Context, arn string) error
	GetTopics(ctx context.Context) ([]string, error)
}

// Publisher publishes messages to a topic. This is the interface most
// applications need, and the one worth depending on.
type Publisher interface {
	PublishMessage(ctx context.Context, temaArn string, msj string, atributos map[string]types.MessageAttributeValue) (string, error)
	PublishJSON(ctx context.Context, temaArn string, msj any, atributos map[string]types.MessageAttributeValue) (string, error)
}

// SubscriptionManager attaches and detaches endpoints from a topic.
type SubscriptionManager interface {
	CreateSubscription(ctx context.Context, temaArn, protocolo, endpoint string) (string, error)
	DeleteSubscription(ctx context.Context, subscriptionArn string) error
}

// SMSSender sends text messages and manages the account's SMS settings.
type SMSSender interface {
	SendSMS(ctx context.Context, phoneNumber, message string, attributes map[string]types.MessageAttributeValue) (string, error)
	SendBulkSMS(ctx context.Context, phoneNumbers []string, message string, attributes map[string]types.MessageAttributeValue) (*BulkSMSResult, error)
	SetSMSAttributes(ctx context.Context, attributes map[string]string) error
	GetSMSAttributes(ctx context.Context) (map[string]string, error)
	CheckPhoneNumberOptedOut(ctx context.Context, phoneNumber string) (bool, error)
	ListOptedOutPhoneNumbers(ctx context.Context) ([]string, error)
	OptInPhoneNumber(ctx context.Context, phoneNumber string) error
}

// PushNotifier manages mobile push applications and their device endpoints.
type PushNotifier interface {
	CreatePlatformApplication(ctx context.Context, name, platform string, credentials map[string]string) (string, error)
	CreatePlatformEndpoint(ctx context.Context, platformApplicationArn, token string, customUserData string, attributes map[string]string) (string, error)
	PublishToEndpoint(ctx context.Context, endpointArn string, message string, messageAttributes map[string]types.MessageAttributeValue) (string, error)
	SetEndpointAttributes(ctx context.Context, endpointArn string, attributes map[string]string) error
	GetEndpointAttributes(ctx context.Context, endpointArn string) (map[string]string, error)
	DeleteEndpoint(ctx context.Context, endpointArn string) error
	DeletePlatformApplication(ctx context.Context, platformApplicationArn string) error
	ListPlatformApplications(ctx context.Context) ([]PlatformApplication, error)
	ListEndpointsByPlatformApplication(ctx context.Context, platformApplicationArn string) ([]Endpoint, error)
}

// Service is the full client, composed of the concerns above. The provider
// returns it, so an application can narrow to whichever part it uses.
type Service interface {
	TopicManager
	Publisher
	SubscriptionManager
	SMSSender
	PushNotifier

	// EnableLogging toggles per-operation logging at runtime.
	EnableLogging(activar bool)
}

type Cliente struct {
	cliente *sns.Client
	*client.BaseClient
}
