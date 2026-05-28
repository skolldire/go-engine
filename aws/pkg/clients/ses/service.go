package ses

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ses"
	"github.com/aws/aws-sdk-go-v2/service/ses/types"
	"github.com/skolldire/go-engine/pkg/core/client"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/validation"
)

func NewClient(acf aws.Config, cfg Config, log logger.Service) Service {
	sesClient := ses.NewFromConfig(acf, func(o *ses.Options) {
		if cfg.Region != "" {
			o.Region = cfg.Region
		}
	})

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	baseConfig := client.BaseConfig{
		EnableLogging:  cfg.EnableLogging,
		WithResilience: cfg.WithResilience,
		Resilience:     cfg.Resilience,
		Timeout:        timeout,
	}

	c := &SESClient{
		BaseClient: client.NewBaseClientWithName(baseConfig, log, "SES"),
		sesClient:  sesClient,
		region:     cfg.Region,
	}

	if c.IsLoggingEnabled() {
		log.Debug(context.Background(), "SES client initialized",
			map[string]interface{}{
				"region": cfg.Region,
			})
	}

	return c
}

func (c *SESClient) SendEmail(ctx context.Context, message EmailMessage) (*SendEmailResult, error) {
	if message.From.Email == "" || len(message.To) == 0 {
		return nil, ErrInvalidInput
	}

	if err := validation.GetGlobalValidator().Var(message.From.Email, "required,email"); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidAddress, err)
	}

	for _, to := range message.To {
		if err := validation.GetGlobalValidator().Var(to.Email, "required,email"); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidAddress, err)
		}
	}

	for _, cc := range message.Cc {
		if err := validation.GetGlobalValidator().Var(cc.Email, "required,email"); err != nil {
			return nil, fmt.Errorf("%w: invalid Cc address: %v", ErrInvalidAddress, err)
		}
	}

	for _, bcc := range message.Bcc {
		if err := validation.GetGlobalValidator().Var(bcc.Email, "required,email"); err != nil {
			return nil, fmt.Errorf("%w: invalid Bcc address: %v", ErrInvalidAddress, err)
		}
	}

	for _, replyTo := range message.ReplyTo {
		if err := validation.GetGlobalValidator().Var(replyTo.Email, "required,email"); err != nil {
			return nil, fmt.Errorf("%w: invalid ReplyTo address: %v", ErrInvalidAddress, err)
		}
	}

	destination := &types.Destination{
		ToAddresses:  make([]string, len(message.To)),
		CcAddresses:  make([]string, len(message.Cc)),
		BccAddresses: make([]string, len(message.Bcc)),
	}

	for i, addr := range message.To {
		destination.ToAddresses[i] = addr.Email
	}
	for i, addr := range message.Cc {
		destination.CcAddresses[i] = addr.Email
	}
	for i, addr := range message.Bcc {
		destination.BccAddresses[i] = addr.Email
	}

	fromAddr := message.From.Email
	if message.From.Name != "" {
		fromAddr = fmt.Sprintf("%s <%s>", message.From.Name, message.From.Email)
	}

	emailContent := &types.Message{
		Subject: &types.Content{
			Data:    aws.String(message.Subject),
			Charset: aws.String("UTF-8"),
		},
		Body: &types.Body{},
	}

	if message.BodyHTML != "" {
		emailContent.Body.Html = &types.Content{
			Data:    aws.String(message.BodyHTML),
			Charset: aws.String("UTF-8"),
		}
	}

	if message.BodyText != "" {
		emailContent.Body.Text = &types.Content{
			Data:    aws.String(message.BodyText),
			Charset: aws.String("UTF-8"),
		}
	}

	replyTo := make([]string, len(message.ReplyTo))
	for i, addr := range message.ReplyTo {
		replyTo[i] = addr.Email
	}

	input := &ses.SendEmailInput{
		Source:           aws.String(fromAddr),
		Destination:      destination,
		Message:          emailContent,
		ReplyToAddresses: replyTo,
	}

	result, err := c.Execute(ctx, "SendEmail", func() (interface{}, error) {
		return c.sesClient.SendEmail(ctx, input)
	})

	if err != nil {
		return nil, c.GetLogger().WrapError(err, ErrSendEmail.Error())
	}

	response, err := client.SafeTypeAssert[*ses.SendEmailOutput](result)
	if err != nil {
		return nil, c.GetLogger().WrapError(err, ErrSendEmail.Error())
	}
	if response == nil || response.MessageId == nil {
		return nil, c.GetLogger().WrapError(ErrSendEmail, "SES response or MessageId is nil")
	}
	return &SendEmailResult{
		MessageID: *response.MessageId,
	}, nil
}

func (c *SESClient) SendRawEmail(ctx context.Context, rawMessage []byte, destinations []string) (*SendEmailResult, error) {
	if len(rawMessage) == 0 || len(destinations) == 0 {
		return nil, ErrInvalidInput
	}

	result, err := c.Execute(ctx, "SendRawEmail", func() (interface{}, error) {
		return c.sesClient.SendRawEmail(ctx, &ses.SendRawEmailInput{
			RawMessage: &types.RawMessage{
				Data: rawMessage,
			},
			Destinations: destinations,
		})
	})

	if err != nil {
		return nil, c.GetLogger().WrapError(err, ErrSendEmail.Error())
	}

	response, err := client.SafeTypeAssert[*ses.SendRawEmailOutput](result)
	if err != nil {
		return nil, c.GetLogger().WrapError(err, ErrSendEmail.Error())
	}
	if response == nil || response.MessageId == nil {
		return nil, c.GetLogger().WrapError(ErrSendEmail, "SES response or MessageId is nil")
	}
	return &SendEmailResult{
		MessageID: *response.MessageId,
	}, nil
}

func (c *SESClient) SendBulkEmail(ctx context.Context, from EmailAddress, subject string, htmlBody, textBody string, destinations []EmailAddress) (*BulkSendResult, error) {
	if from.Email == "" || len(destinations) == 0 {
		return nil, ErrInvalidInput
	}

	if err := validation.GetGlobalValidator().Var(from.Email, "required,email"); err != nil {
		return nil, fmt.Errorf("%w: invalid sender email: %v", ErrInvalidAddress, err)
	}

	result := &BulkSendResult{
		Recipients:       make([]RecipientResult, 0, len(destinations)),
		FailedRecipients: make([]string, 0),
	}

	for _, dest := range destinations {
		recipientResult := RecipientResult{
			EmailAddress: dest.Email,
		}

		if err := validation.GetGlobalValidator().Var(dest.Email, "required,email"); err != nil {
			recipientResult.Error = fmt.Errorf("invalid email address: %w", err)
			result.Recipients = append(result.Recipients, recipientResult)
			result.FailureCount++
			result.FailedRecipients = append(result.FailedRecipients, dest.Email)

			if c.IsLoggingEnabled() {
				c.GetLogger().Warn(ctx, "skipping invalid email address", map[string]interface{}{
					"email": dest.Email,
					"error": err.Error(),
				})
			}
			continue
		}

		message := EmailMessage{
			Subject:  subject,
			BodyHTML: htmlBody,
			BodyText: textBody,
			From:     from,
			To:       []EmailAddress{dest},
		}

		sendResult, err := c.SendEmail(ctx, message)
		if err != nil {
			recipientResult.Error = err
			result.Recipients = append(result.Recipients, recipientResult)
			result.FailureCount++
			result.FailedRecipients = append(result.FailedRecipients, dest.Email)
			continue
		}

		recipientResult.MessageID = sendResult.MessageID
		result.Recipients = append(result.Recipients, recipientResult)
		result.SuccessCount++
	}

	return result, nil
}

func (c *SESClient) GetSendQuota(ctx context.Context) (*SendQuota, error) {
	result, err := c.Execute(ctx, "GetSendQuota", func() (interface{}, error) {
		return c.sesClient.GetSendQuota(ctx, &ses.GetSendQuotaInput{})
	})

	if err != nil {
		return nil, c.GetLogger().WrapError(err, "error getting send quota")
	}

	response, err := client.SafeTypeAssert[*ses.GetSendQuotaOutput](result)
	if err != nil {
		return nil, c.GetLogger().WrapError(err, "error getting send quota")
	}
	return &SendQuota{
		Max24HourSend:   response.Max24HourSend,
		MaxSendRate:     response.MaxSendRate,
		SentLast24Hours: response.SentLast24Hours,
	}, nil
}

func (c *SESClient) GetSendStatistics(ctx context.Context) ([]SendDataPoint, error) {
	result, err := c.Execute(ctx, "GetSendStatistics", func() (interface{}, error) {
		return c.sesClient.GetSendStatistics(ctx, &ses.GetSendStatisticsInput{})
	})

	if err != nil {
		return nil, c.GetLogger().WrapError(err, "error getting send statistics")
	}

	response, err := client.SafeTypeAssert[*ses.GetSendStatisticsOutput](result)
	if err != nil {
		return nil, c.GetLogger().WrapError(err, "error getting send statistics")
	}
	dataPoints := make([]SendDataPoint, len(response.SendDataPoints))

	for i, dp := range response.SendDataPoints {
		timestamp := time.Time{}
		if dp.Timestamp != nil {
			timestamp = *dp.Timestamp
		}
		dataPoints[i] = SendDataPoint{
			Timestamp:        timestamp,
			DeliveryAttempts: dp.DeliveryAttempts,
			Bounces:          dp.Bounces,
			Complaints:       dp.Complaints,
			Rejects:          dp.Rejects,
		}
	}

	return dataPoints, nil
}

func (c *SESClient) VerifyEmailAddress(ctx context.Context, email string) error {
	if email == "" {
		return ErrInvalidInput
	}

	_, err := c.Execute(ctx, "VerifyEmailAddress", func() (interface{}, error) {
		return c.sesClient.VerifyEmailAddress(ctx, &ses.VerifyEmailAddressInput{
			EmailAddress: aws.String(email),
		})
	})

	if err != nil {
		return c.GetLogger().WrapError(err, "error verifying email address")
	}

	return nil
}

func (c *SESClient) DeleteVerifiedEmailAddress(ctx context.Context, email string) error {
	if email == "" {
		return ErrInvalidInput
	}

	_, err := c.Execute(ctx, "DeleteVerifiedEmailAddress", func() (interface{}, error) {
		return c.sesClient.DeleteVerifiedEmailAddress(ctx, &ses.DeleteVerifiedEmailAddressInput{
			EmailAddress: aws.String(email),
		})
	})

	if err != nil {
		return c.GetLogger().WrapError(err, "error deleting verified email address")
	}

	return nil
}

func (c *SESClient) ListVerifiedEmailAddresses(ctx context.Context) ([]string, error) {
	result, err := c.Execute(ctx, "ListVerifiedEmailAddresses", func() (interface{}, error) {
		return c.sesClient.ListVerifiedEmailAddresses(ctx, &ses.ListVerifiedEmailAddressesInput{})
	})

	if err != nil {
		return nil, c.GetLogger().WrapError(err, "error listing verified email addresses")
	}

	response, err := client.SafeTypeAssert[*ses.ListVerifiedEmailAddressesOutput](result)
	if err != nil {
		return nil, c.GetLogger().WrapError(err, "error listing verified email addresses")
	}
	return response.VerifiedEmailAddresses, nil
}

func (c *SESClient) EnableLogging(enable bool) {
	c.SetLogging(enable)
}
