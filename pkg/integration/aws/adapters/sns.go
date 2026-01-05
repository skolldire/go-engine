package adapters

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/skolldire/go-engine/pkg/integration/cloud"
)

type snsAdapter struct {
	client  *sns.Client
	timeout time.Duration
	retries RetryPolicy
}

// newSNSAdapter creates a cloud.Client that communicates with Amazon SNS using the provided
// AWS configuration, request timeout, and retry policy.
func newSNSAdapter(cfg aws.Config, timeout time.Duration, retries RetryPolicy) cloud.Client {
	return &snsAdapter{
		client:  sns.NewFromConfig(cfg),
		timeout: timeout,
		retries: retries,
	}
}

func (a *snsAdapter) Do(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	switch req.Operation {
	case "sns.publish":
		return a.publish(ctx, req)
	default:
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, fmt.Sprintf("unsupported SNS operation: %s", req.Operation))
	}
}

func (a *snsAdapter) publish(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	if req.Path == "" {
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, "topic ARN/path is required")
	}

	input := &sns.PublishInput{
		TopicArn: aws.String(req.Path),
		Message:   aws.String(string(req.Body)),
	}

	// Parse headers for SNS-specific attributes
	if req.Headers != nil {
		if subject, ok := req.Headers["sns.subject"]; ok {
			input.Subject = aws.String(subject)
		}

		// Parse message attributes
		attrs := make(map[string]types.MessageAttributeValue)
		for k, v := range req.Headers {
			if strings.HasPrefix(k, "sns.message_attribute.") {
				attrName := strings.TrimPrefix(k, "sns.message_attribute.")
				attrs[attrName] = types.MessageAttributeValue{
					DataType:    aws.String("String"),
					StringValue: aws.String(v),
				}
			}
		}
		if len(attrs) > 0 {
			input.MessageAttributes = attrs
		}
	}

	result, err := a.client.Publish(ctx, input)
	if err != nil {
		return nil, normalizeSNSError(err, "sns.publish")
	}

	return &cloud.Response{
		StatusCode: 200,
		Headers: map[string]string{
			"sns.message_id": aws.ToString(result.MessageId),
		},
		Metadata: map[string]interface{}{
			"sns.message_id": aws.ToString(result.MessageId),
		},
	}, nil
}
