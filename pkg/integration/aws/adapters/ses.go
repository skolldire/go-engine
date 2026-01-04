package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ses"
	"github.com/aws/aws-sdk-go-v2/service/ses/types"
	"github.com/skolldire/go-engine/pkg/integration/cloud"
)

type sesAdapter struct {
	client  *ses.Client
	timeout time.Duration
	retries RetryPolicy
}

func newSESAdapter(cfg aws.Config, timeout time.Duration, retries RetryPolicy) cloud.Client {
	return &sesAdapter{
		client:  ses.NewFromConfig(cfg),
		timeout: timeout,
		retries: retries,
	}
}

func (a *sesAdapter) Do(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	switch req.Operation {
	case "ses.send_email":
		return a.sendEmail(ctx, req)
	case "ses.send_raw_email":
		return a.sendRawEmail(ctx, req)
	case "ses.get_send_quota":
		return a.getSendQuota(ctx, req)
	case "ses.get_send_statistics":
		return a.getSendStatistics(ctx, req)
	case "ses.verify_email_identity":
		return a.verifyEmailIdentity(ctx, req)
	case "ses.delete_verified_email_address":
		return a.deleteVerifiedEmailAddress(ctx, req)
	case "ses.list_verified_email_addresses":
		return a.listVerifiedEmailAddresses(ctx, req)
	default:
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, fmt.Sprintf("unsupported SES operation: %s", req.Operation))
	}
}

func (a *sesAdapter) sendEmail(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	// Parse body as JSON email message
	var emailMsg map[string]interface{}
	if err := json.Unmarshal(req.Body, &emailMsg); err != nil {
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, fmt.Sprintf("invalid JSON body: %v", err))
	}

	// Extract from address
	from, ok := emailMsg["from"].(map[string]interface{})
	if !ok {
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, "from address is required")
	}
	fromEmail, _ := from["email"].(string)
	if fromEmail == "" {
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, "from.email is required")
	}
	fromName, _ := from["name"].(string)
	fromAddr := fromEmail
	if fromName != "" {
		fromAddr = fmt.Sprintf("%s <%s>", fromName, fromEmail)
	}

	// Extract destination
	destination := &types.Destination{}
	if to, ok := emailMsg["to"].([]interface{}); ok {
		destination.ToAddresses = make([]string, len(to))
		for i, addr := range to {
			if addrMap, ok := addr.(map[string]interface{}); ok {
				if email, ok := addrMap["email"].(string); ok {
					destination.ToAddresses[i] = email
				}
			}
		}
	}
	if cc, ok := emailMsg["cc"].([]interface{}); ok {
		destination.CcAddresses = make([]string, len(cc))
		for i, addr := range cc {
			if addrMap, ok := addr.(map[string]interface{}); ok {
				if email, ok := addrMap["email"].(string); ok {
					destination.CcAddresses[i] = email
				}
			}
		}
	}
	if bcc, ok := emailMsg["bcc"].([]interface{}); ok {
		destination.BccAddresses = make([]string, len(bcc))
		for i, addr := range bcc {
			if addrMap, ok := addr.(map[string]interface{}); ok {
				if email, ok := addrMap["email"].(string); ok {
					destination.BccAddresses[i] = email
				}
			}
		}
	}

	// Extract subject and body
	subject, _ := emailMsg["subject"].(string)
	bodyHTML, _ := emailMsg["body_html"].(string)
	bodyText, _ := emailMsg["body_text"].(string)

	emailContent := &types.Message{
		Subject: &types.Content{
			Data:    aws.String(subject),
			Charset: aws.String("UTF-8"),
		},
		Body: &types.Body{},
	}

	if bodyHTML != "" {
		emailContent.Body.Html = &types.Content{
			Data:    aws.String(bodyHTML),
			Charset: aws.String("UTF-8"),
		}
	}

	if bodyText != "" {
		emailContent.Body.Text = &types.Content{
			Data:    aws.String(bodyText),
			Charset: aws.String("UTF-8"),
		}
	}

	// Extract reply-to addresses
	var replyTo []string
	if replyToAddrs, ok := emailMsg["reply_to"].([]interface{}); ok {
		replyTo = make([]string, len(replyToAddrs))
		for i, addr := range replyToAddrs {
			if addrMap, ok := addr.(map[string]interface{}); ok {
				if email, ok := addrMap["email"].(string); ok {
					replyTo[i] = email
				}
			}
		}
	}

	input := &ses.SendEmailInput{
		Source:           aws.String(fromAddr),
		Destination:      destination,
		Message:          emailContent,
		ReplyToAddresses: replyTo,
	}

	result, err := a.client.SendEmail(ctx, input)
	if err != nil {
		return nil, normalizeSESError(err, "ses.send_email")
	}

	return &cloud.Response{
		StatusCode: 200,
		Headers: map[string]string{
			"ses.message_id": aws.ToString(result.MessageId),
		},
		Metadata: map[string]interface{}{
			"ses.message_id": aws.ToString(result.MessageId),
		},
	}, nil
}

func (a *sesAdapter) sendRawEmail(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	// Parse body as JSON with raw_message and destinations
	var rawEmailMsg map[string]interface{}
	if err := json.Unmarshal(req.Body, &rawEmailMsg); err != nil {
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, fmt.Sprintf("invalid JSON body: %v", err))
	}

	rawMessage, ok := rawEmailMsg["raw_message"].(string)
	if !ok {
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, "raw_message is required")
	}

	var destinations []string
	if dests, ok := rawEmailMsg["destinations"].([]interface{}); ok {
		destinations = make([]string, len(dests))
		for i, dest := range dests {
			destinations[i] = fmt.Sprintf("%v", dest)
		}
	}

	input := &ses.SendRawEmailInput{
		RawMessage: &types.RawMessage{
			Data: []byte(rawMessage),
		},
		Destinations: destinations,
	}

	result, err := a.client.SendRawEmail(ctx, input)
	if err != nil {
		return nil, normalizeSESError(err, "ses.send_raw_email")
	}

	return &cloud.Response{
		StatusCode: 200,
		Headers: map[string]string{
			"ses.message_id": aws.ToString(result.MessageId),
		},
		Metadata: map[string]interface{}{
			"ses.message_id": aws.ToString(result.MessageId),
		},
	}, nil
}

func (a *sesAdapter) getSendQuota(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	result, err := a.client.GetSendQuota(ctx, &ses.GetSendQuotaInput{})
	if err != nil {
		return nil, normalizeSESError(err, "ses.get_send_quota")
	}

	quota := map[string]interface{}{
		"max_24_hour_send":   result.Max24HourSend,
		"max_send_rate":      result.MaxSendRate,
		"sent_last_24_hours": result.SentLast24Hours,
	}

	body, _ := json.Marshal(quota)

	return &cloud.Response{
		StatusCode: 200,
		Body:       body,
	}, nil
}

func (a *sesAdapter) getSendStatistics(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	result, err := a.client.GetSendStatistics(ctx, &ses.GetSendStatisticsInput{})
	if err != nil {
		return nil, normalizeSESError(err, "ses.get_send_statistics")
	}

	dataPoints := make([]map[string]interface{}, len(result.SendDataPoints))
	for i, dp := range result.SendDataPoints {
		dataPoints[i] = map[string]interface{}{
			"timestamp":         dp.Timestamp,
			"delivery_attempts": dp.DeliveryAttempts,
			"bounces":           dp.Bounces,
			"complaints":        dp.Complaints,
			"rejects":           dp.Rejects,
		}
	}

	body, _ := json.Marshal(dataPoints)

	return &cloud.Response{
		StatusCode: 200,
		Body:       body,
	}, nil
}

func (a *sesAdapter) verifyEmailIdentity(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	email := req.Path
	if email == "" {
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, "email address is required")
	}

	_, err := a.client.VerifyEmailAddress(ctx, &ses.VerifyEmailAddressInput{
		EmailAddress: aws.String(email),
	})
	if err != nil {
		return nil, normalizeSESError(err, "ses.verify_email_identity")
	}

	return &cloud.Response{
		StatusCode: 200,
	}, nil
}

func (a *sesAdapter) deleteVerifiedEmailAddress(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	email := req.Path
	if email == "" {
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, "email address is required")
	}

	_, err := a.client.DeleteVerifiedEmailAddress(ctx, &ses.DeleteVerifiedEmailAddressInput{
		EmailAddress: aws.String(email),
	})
	if err != nil {
		return nil, normalizeSESError(err, "ses.delete_verified_email_address")
	}

	return &cloud.Response{
		StatusCode: 204, // No Content
	}, nil
}

func (a *sesAdapter) listVerifiedEmailAddresses(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	result, err := a.client.ListVerifiedEmailAddresses(ctx, &ses.ListVerifiedEmailAddressesInput{})
	if err != nil {
		return nil, normalizeSESError(err, "ses.list_verified_email_addresses")
	}

	body, _ := json.Marshal(result.VerifiedEmailAddresses)

	return &cloud.Response{
		StatusCode: 200,
		Body:       body,
	}, nil
}

func normalizeSESError(err error, operation string) *cloud.Error {
	if err == nil {
		return nil
	}

	return cloud.NewErrorWithCause(
		fmt.Sprintf("%s.error", operation),
		err.Error(),
		err,
	).WithMetadata("status_code", 500)
}



