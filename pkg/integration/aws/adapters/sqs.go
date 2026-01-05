package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/skolldire/go-engine/pkg/integration/cloud"
)

type sqsAdapter struct {
	client  *sqs.Client
	timeout time.Duration
	retries RetryPolicy
}

// newSQSAdapter creates a cloud.Client that communicates with Amazon SQS using the provided AWS configuration,
// applying the given request timeout and retry policy.
func newSQSAdapter(cfg aws.Config, timeout time.Duration, retries RetryPolicy) cloud.Client {
	return &sqsAdapter{
		client:  sqs.NewFromConfig(cfg),
		timeout: timeout,
		retries: retries,
	}
}

func (a *sqsAdapter) Do(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	switch req.Operation {
	case "sqs.send_message":
		return a.sendMessage(ctx, req)
	case "sqs.receive_message":
		return a.receiveMessages(ctx, req)
	case "sqs.delete_message":
		return a.deleteMessage(ctx, req)
	case "sqs.create_queue":
		return a.createQueue(ctx, req)
	case "sqs.delete_queue":
		return a.deleteQueue(ctx, req)
	case "sqs.list_queues":
		return a.listQueues(ctx, req)
	case "sqs.get_queue_url":
		return a.getQueueURL(ctx, req)
	default:
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, fmt.Sprintf("unsupported SQS operation: %s", req.Operation))
	}
}

func (a *sqsAdapter) sendMessage(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	if req.Path == "" {
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, "queue URL/path is required")
	}

	input := &sqs.SendMessageInput{
		QueueUrl:    aws.String(req.Path),
		MessageBody: aws.String(string(req.Body)),
	}

	// Parse headers for SQS-specific attributes
	if req.Headers != nil {
		if delaySeconds, ok := req.Headers["sqs.delay_seconds"]; ok {
			if delay, err := strconv.ParseInt(delaySeconds, 10, 32); err == nil {
				input.DelaySeconds = int32(delay)
			}
		}

		if groupID, ok := req.Headers["sqs.message_group_id"]; ok {
			input.MessageGroupId = aws.String(groupID)
		}

		if dedupeID, ok := req.Headers["sqs.message_dedupe_id"]; ok {
			input.MessageDeduplicationId = aws.String(dedupeID)
		}

		// Parse message attributes
		attrs := make(map[string]types.MessageAttributeValue)
		for k, v := range req.Headers {
			if strings.HasPrefix(k, "sqs.message_attribute.") {
				attrName := strings.TrimPrefix(k, "sqs.message_attribute.")
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

	result, err := a.client.SendMessage(ctx, input)
	if err != nil {
		return nil, normalizeSQSError(err, "sqs.send_message")
	}

	return &cloud.Response{
		StatusCode: 200,
		Headers: map[string]string{
			"sqs.message_id": aws.ToString(result.MessageId),
		},
		Metadata: map[string]interface{}{
			"sqs.message_id":           aws.ToString(result.MessageId),
			"sqs.sequence_number":      aws.ToString(result.SequenceNumber),
			"sqs.md5_of_message_body":   aws.ToString(result.MD5OfMessageBody),
			"sqs.md5_of_message_attrs": aws.ToString(result.MD5OfMessageAttributes),
		},
	}, nil
}

func (a *sqsAdapter) receiveMessages(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	if req.Path == "" {
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, "queue URL/path is required")
	}

	input := &sqs.ReceiveMessageInput{
		QueueUrl: aws.String(req.Path),
	}

	// Parse query params
	if req.QueryParams != nil {
		if maxMessages, ok := req.QueryParams["MaxNumberOfMessages"]; ok {
			if max, err := strconv.ParseInt(maxMessages, 10, 32); err == nil {
				input.MaxNumberOfMessages = int32(max)
			}
		}

		if waitTime, ok := req.QueryParams["WaitTimeSeconds"]; ok {
			if wait, err := strconv.ParseInt(waitTime, 10, 32); err == nil {
				input.WaitTimeSeconds = int32(wait)
			}
		}
	}

	// Default values
	if input.MaxNumberOfMessages == 0 {
		input.MaxNumberOfMessages = 1
	}

	result, err := a.client.ReceiveMessage(ctx, input)
	if err != nil {
		return nil, normalizeSQSError(err, "sqs.receive_message")
	}

	// Convert messages to JSON array
	messages := make([]map[string]interface{}, len(result.Messages))
	for i, msg := range result.Messages {
		messages[i] = map[string]interface{}{
			"message_id":     aws.ToString(msg.MessageId),
			"receipt_handle": aws.ToString(msg.ReceiptHandle),
			"body":           aws.ToString(msg.Body),
			"attributes":     msg.Attributes,
		}
	}

	bodyBytes, _ := json.Marshal(messages)

	return &cloud.Response{
		StatusCode: 200,
		Body:      bodyBytes,
		Headers: map[string]string{
			"sqs.message_count": strconv.Itoa(len(result.Messages)),
		},
	}, nil
}

func (a *sqsAdapter) deleteMessage(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	if req.Path == "" {
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, "queue URL/path is required")
	}

	receiptHandle := ""
	if req.Headers != nil {
		receiptHandle = req.Headers["sqs.receipt_handle"]
	}

	if receiptHandle == "" {
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, "receipt handle is required")
	}

	input := &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(req.Path),
		ReceiptHandle: aws.String(receiptHandle),
	}

	_, err := a.client.DeleteMessage(ctx, input)
	if err != nil {
		return nil, normalizeSQSError(err, "sqs.delete_message")
	}

	return &cloud.Response{
		StatusCode: 204, // No Content
	}, nil
}

func (a *sqsAdapter) createQueue(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	if req.Path == "" {
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, "queue name is required")
	}

	input := &sqs.CreateQueueInput{
		QueueName: aws.String(req.Path),
	}

	// Parse attributes from headers
	if req.Headers != nil {
		attrs := make(map[string]string)
		for k, v := range req.Headers {
			if strings.HasPrefix(k, "sqs.queue_attribute.") {
				attrName := strings.TrimPrefix(k, "sqs.queue_attribute.")
				attrs[attrName] = v
			}
		}
		if len(attrs) > 0 {
			input.Attributes = attrs
		}
	}

	result, err := a.client.CreateQueue(ctx, input)
	if err != nil {
		return nil, normalizeSQSError(err, "sqs.create_queue")
	}

	return &cloud.Response{
		StatusCode: 201, // Created
		Headers: map[string]string{
			"sqs.queue_url": aws.ToString(result.QueueUrl),
		},
		Metadata: map[string]interface{}{
			"sqs.queue_url": aws.ToString(result.QueueUrl),
		},
	}, nil
}

func (a *sqsAdapter) deleteQueue(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	if req.Path == "" {
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, "queue URL is required")
	}

	input := &sqs.DeleteQueueInput{
		QueueUrl: aws.String(req.Path),
	}

	_, err := a.client.DeleteQueue(ctx, input)
	if err != nil {
		return nil, normalizeSQSError(err, "sqs.delete_queue")
	}

	return &cloud.Response{
		StatusCode: 204, // No Content
	}, nil
}

func (a *sqsAdapter) listQueues(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	input := &sqs.ListQueuesInput{}

	// Parse query params
	if req.QueryParams != nil {
		if prefix, ok := req.QueryParams["QueueNamePrefix"]; ok {
			input.QueueNamePrefix = aws.String(prefix)
		}
	}

	result, err := a.client.ListQueues(ctx, input)
	if err != nil {
		return nil, normalizeSQSError(err, "sqs.list_queues")
	}

	// Convert queue URLs to JSON array
	queueURLs := make([]string, len(result.QueueUrls))
	for i, url := range result.QueueUrls {
		queueURLs[i] = url
	}

	bodyBytes, _ := json.Marshal(queueURLs)

	return &cloud.Response{
		StatusCode: 200,
		Body:       bodyBytes,
		Headers: map[string]string{
			"sqs.queue_count": strconv.Itoa(len(result.QueueUrls)),
		},
	}, nil
}

func (a *sqsAdapter) getQueueURL(ctx context.Context, req *cloud.Request) (*cloud.Response, error) {
	if req.Path == "" {
		return nil, cloud.NewError(cloud.ErrCodeInvalidRequest, "queue name is required")
	}

	input := &sqs.GetQueueUrlInput{
		QueueName: aws.String(req.Path),
	}

	result, err := a.client.GetQueueUrl(ctx, input)
	if err != nil {
		return nil, normalizeSQSError(err, "sqs.get_queue_url")
	}

	return &cloud.Response{
		StatusCode: 200,
		Headers: map[string]string{
			"sqs.queue_url": aws.ToString(result.QueueUrl),
		},
		Metadata: map[string]interface{}{
			"sqs.queue_url": aws.ToString(result.QueueUrl),
		},
	}, nil
}
