package ses

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/ses"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
)

//go:generate mockery --name Service --filename service.go
type Service interface {
	Send(ctx context.Context, e Email) error
}

type Config struct {
	ARN    string `mapstructure:"arn" json:"arn"`
	Sender string `mapstructure:"sender" json:"sender"`
}

type Dependencies struct {
	Config    Config
	SESClient *ses.Client
	Log       logger.Service
}

type Email struct {
	To      []string
	Subject string
	ReplyTo []string
	HTML    *string
}

var (
	ErrPrepareEmail = errors.New("failed to prepare email")
	ErrSendEmail    = errors.New("failed to send email")
	ErrExecution    = errors.New("ses execution failed")
)
