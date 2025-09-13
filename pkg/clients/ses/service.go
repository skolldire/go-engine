package ses

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awses "github.com/aws/aws-sdk-go-v2/service/ses"
	sestypes "github.com/aws/aws-sdk-go-v2/service/ses/types"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"gopkg.in/gomail.v2"
)

const (
	_msgExecDone    = "ses.%s executed successfully"
	_msgExecErr     = "ses.%s execution failed"
	_attrSender     = "sender"
	_attrRecipients = "recipients"
	_attrSubject    = "subject"
)

type client struct {
	svc    *awses.Client
	arn    string
	logger logger.Service
	sender string
}

var _ Service = (*client)(nil)

func NewService(d Dependencies) Service {
	return &client{
		svc:    d.SESClient,
		arn:    d.Config.ARN,
		logger: d.Log,
		sender: d.Config.Sender,
	}
}

func (c *client) Send(ctx context.Context, e Email) error {
	run := func(ctx context.Context) error {
		raw, err := c.prepare(e)
		if err != nil {
			return errors.Join(err, ErrExecution)
		}
		return c.sendRaw(ctx, e.To, raw)
	}

	data := map[string]interface{}{_attrSender: c.sender, _attrRecipients: strings.Join(e.To, ","), _attrSubject: e.Subject}
	err := run(ctx)

	if err != nil {
		c.logRequestError(ctx, "SendNotification", err, data)
		return errors.Join(err, ErrExecution)
	}
	c.logRequestSuccess(ctx, "SendNotification", data)
	return nil
}

func (c *client) sendRaw(ctx context.Context, recipients []string, raw []byte) error {

	data := map[string]interface{}{_attrSender: c.sender, _attrRecipients: strings.Join(recipients, ",")}

	input := &awses.SendRawEmailInput{
		RawMessage:    &sestypes.RawMessage{Data: raw},
		Destinations:  recipients,
		SourceArn:     aws.String(c.arn),
		FromArn:       aws.String(c.arn),
		ReturnPathArn: aws.String(c.arn),
		Source:        aws.String(c.sender),
	}

	_, err := c.svc.SendRawEmail(ctx, input)
	if err != nil {
		c.logRequestError(ctx, "SendRawEmail", err, data)
		return errors.Join(err, ErrSendEmail)
	}
	return nil
}

func (c *client) logRequestError(ctx context.Context, method string, err error, data map[string]interface{}) {
	data["error"] = err
	c.logger.Debug(ctx, fmt.Sprintf(_msgExecErr, method), data)
}

func (c *client) logRequestSuccess(ctx context.Context, method string, data map[string]interface{}) {
	c.logger.Debug(ctx, fmt.Sprintf(_msgExecDone, method), data)
}

func (c *client) prepare(e Email) ([]byte, error) {
	msg := gomail.NewMessage()

	msg.SetHeader("From", c.sender)
	msg.SetHeader("To", e.To...)
	msg.SetHeader("Reply-To", e.ReplyTo...)
	msg.SetHeader("Subject", e.Subject)

	charset := "UTF-8"
	msg.SetBody("text/html; charset="+charset, *e.HTML)

	var raw bytes.Buffer
	if _, err := msg.WriteTo(&raw); err != nil {
		return nil, errors.Join(err, ErrPrepareEmail)
	}
	return raw.Bytes(), nil
}
