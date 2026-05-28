package aws

import (
	"context"
	"fmt"

	"github.com/skolldire/go-engine/pkg/integration/cloud"
)

// SQSSendMessage sends a message to SQS queue (convenience wrapper)
// Uses WithJSONBody() internally for ergonomic JSON serialization
// AWS SDK equivalent: SendMessage
func SQSSendMessage(ctx context.Context, client Client, queueURL string, v interface{}) (messageID string, err error) {
	req := &cloud.Request{
		Operation: "sqs.send_message",
		Path:      queueURL,
	}
	if err := req.WithJSONBody(v); err != nil {
		return "", fmt.Errorf("failed to marshal JSON body: %w", err)
	}
	resp, err := client.Do(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.Headers["sqs.message_id"], nil
}

// SQSSendMessageBytes sends raw bytes to SQS (for non-JSON payloads)
// AWS SDK equivalent: SendMessage
func SQSSendMessageBytes(ctx context.Context, client Client, queueURL string, body []byte) (messageID string, err error) {
	req := &cloud.Request{
		Operation: "sqs.send_message",
		Path:      queueURL,
	}
	req.WithBody(body)
	resp, err := client.Do(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.Headers["sqs.message_id"], nil
}

// SQSReceiveMessage receives messages from SQS queue
// AWS SDK equivalent: ReceiveMessage
func SQSReceiveMessage(ctx context.Context, client Client, queueURL string, maxMessages int32, waitTimeSeconds int32) (*cloud.Response, error) {
	req := &cloud.Request{
		Operation: "sqs.receive_message",
		Path:      queueURL,
		QueryParams: map[string]string{
			"MaxNumberOfMessages": fmt.Sprintf("%d", maxMessages),
			"WaitTimeSeconds":     fmt.Sprintf("%d", waitTimeSeconds),
		},
	}
	return client.Do(ctx, req)
}

// SQSDeleteMessage deletes a message from SQS queue
// AWS SDK equivalent: DeleteMessage
func SQSDeleteMessage(ctx context.Context, client Client, queueURL string, receiptHandle string) error {
	req := &cloud.Request{
		Operation: "sqs.delete_message",
		Path:      queueURL,
		Headers: map[string]string{
			"sqs.receipt_handle": receiptHandle,
		},
	}
	_, err := client.Do(ctx, req)
	return err
}

// SQSCreateQueue creates a new SQS queue
// AWS SDK equivalent: CreateQueue
func SQSCreateQueue(ctx context.Context, client Client, queueName string, attributes map[string]string) (queueURL string, err error) {
	req := &cloud.Request{
		Operation: "sqs.create_queue",
		Path:      queueName,
		Headers:   make(map[string]string),
	}
	for k, v := range attributes {
		req.Headers["sqs.queue_attribute."+k] = v
	}
	resp, err := client.Do(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.Headers["sqs.queue_url"], nil
}

// SQSDeleteQueue deletes an SQS queue
// AWS SDK equivalent: DeleteQueue
func SQSDeleteQueue(ctx context.Context, client Client, queueURL string) error {
	req := &cloud.Request{
		Operation: "sqs.delete_queue",
		Path:      queueURL,
	}
	_, err := client.Do(ctx, req)
	return err
}

// SQSListQueues lists SQS queues
// AWS SDK equivalent: ListQueues
func SQSListQueues(ctx context.Context, client Client, prefix string) (*cloud.Response, error) {
	req := &cloud.Request{
		Operation:   "sqs.list_queues",
		QueryParams: make(map[string]string),
	}
	if prefix != "" {
		req.QueryParams["QueueNamePrefix"] = prefix
	}
	return client.Do(ctx, req)
}

// SQSGetQueueURL gets the URL of an SQS queue by name
// AWS SDK equivalent: GetQueueUrl
func SQSGetQueueURL(ctx context.Context, client Client, queueName string) (queueURL string, err error) {
	req := &cloud.Request{
		Operation: "sqs.get_queue_url",
		Path:      queueName,
	}
	resp, err := client.Do(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.Headers["sqs.queue_url"], nil
}

// SNSPublish publishes a message to SNS topic (convenience wrapper)
// AWS SDK equivalent: Publish
func SNSPublish(ctx context.Context, client Client, topicARN string, v interface{}) (messageID string, err error) {
	req := &cloud.Request{
		Operation: "sns.publish",
		Path:      topicARN,
	}
	if err := req.WithJSONBody(v); err != nil {
		return "", fmt.Errorf("failed to marshal JSON body: %w", err)
	}
	resp, err := client.Do(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.Headers["sns.message_id"], nil
}

// LambdaInvoke invokes a Lambda function (convenience wrapper)
// AWS SDK equivalent: Invoke
func LambdaInvoke(ctx context.Context, client Client, functionName string, v interface{}) (*cloud.Response, error) {
	req := &cloud.Request{
		Operation: "lambda.invoke",
		Path:      functionName,
	}
	if err := req.WithJSONBody(v); err != nil {
		return nil, fmt.Errorf("failed to marshal JSON body: %w", err)
	}
	return client.Do(ctx, req)
}

// S3PutObject uploads an object to S3
// AWS SDK equivalent: PutObject
// Path format: "bucket/key"
func S3PutObject(ctx context.Context, client Client, bucket, key string, body []byte, contentType string, metadata map[string]string) (*cloud.Response, error) {
	req := &cloud.Request{
		Operation: "s3.put_object",
		Path:      fmt.Sprintf("%s/%s", bucket, key),
		Body:      body,
		Headers:   make(map[string]string),
	}
	if contentType != "" {
		req.Headers["s3.content_type"] = contentType
	}
	for k, v := range metadata {
		req.Headers["s3.metadata."+k] = v
	}
	return client.Do(ctx, req)
}

// S3GetObject retrieves an object from S3
// AWS SDK equivalent: GetObject
// Path format: "bucket/key"
func S3GetObject(ctx context.Context, client Client, bucket, key string) (*cloud.Response, error) {
	req := &cloud.Request{
		Operation: "s3.get_object",
		Path:      fmt.Sprintf("%s/%s", bucket, key),
	}
	return client.Do(ctx, req)
}

// S3DeleteObject deletes an object from S3
// AWS SDK equivalent: DeleteObject
// Path format: "bucket/key"
func S3DeleteObject(ctx context.Context, client Client, bucket, key string) error {
	req := &cloud.Request{
		Operation: "s3.delete_object",
		Path:      fmt.Sprintf("%s/%s", bucket, key),
	}
	_, err := client.Do(ctx, req)
	return err
}

// S3HeadObject retrieves object metadata from S3
// AWS SDK equivalent: HeadObject
// Path format: "bucket/key"
func S3HeadObject(ctx context.Context, client Client, bucket, key string) (*cloud.Response, error) {
	req := &cloud.Request{
		Operation: "s3.head_object",
		Path:      fmt.Sprintf("%s/%s", bucket, key),
	}
	return client.Do(ctx, req)
}

// S3ListObjects lists objects in S3 bucket
// AWS SDK equivalent: ListObjectsV2
// Path format: "bucket" or "bucket/prefix"
func S3ListObjects(ctx context.Context, client Client, bucket, prefix string, maxKeys int32) (*cloud.Response, error) {
	path := bucket
	if prefix != "" {
		path = fmt.Sprintf("%s/%s", bucket, prefix)
	}
	req := &cloud.Request{
		Operation:   "s3.list_objects",
		Path:        path,
		QueryParams: make(map[string]string),
	}
	if maxKeys > 0 {
		req.QueryParams["MaxKeys"] = fmt.Sprintf("%d", maxKeys)
	}
	return client.Do(ctx, req)
}

// S3CopyObject copies an object within S3
// AWS SDK equivalent: CopyObject
// destPath format: "destBucket/destKey"
func S3CopyObject(ctx context.Context, client Client, sourceBucket, sourceKey, destBucket, destKey string) (*cloud.Response, error) {
	req := &cloud.Request{
		Operation: "s3.copy_object",
		Path:      fmt.Sprintf("%s/%s", destBucket, destKey),
		Headers: map[string]string{
			"s3.source_bucket": sourceBucket,
			"s3.source_key":    sourceKey,
		},
	}
	return client.Do(ctx, req)
}

// SESSendEmail sends an email via SES
// AWS SDK equivalent: SendEmail
// emailMessage should be a map with: from, to, subject, body_html, body_text, cc, bcc, reply_to
func SESSendEmail(ctx context.Context, client Client, emailMessage map[string]interface{}) (messageID string, err error) {
	req := &cloud.Request{
		Operation: "ses.send_email",
	}
	if err := req.WithJSONBody(emailMessage); err != nil {
		return "", fmt.Errorf("failed to marshal JSON body: %w", err)
	}
	resp, err := client.Do(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.Headers["ses.message_id"], nil
}

// SESSendRawEmail sends a raw email via SES
// AWS SDK equivalent: SendRawEmail
func SESSendRawEmail(ctx context.Context, client Client, rawMessage []byte, destinations []string) (messageID string, err error) {
	req := &cloud.Request{
		Operation: "ses.send_raw_email",
	}
	body := map[string]interface{}{
		"raw_message":  string(rawMessage),
		"destinations": destinations,
	}
	if err := req.WithJSONBody(body); err != nil {
		return "", fmt.Errorf("failed to marshal JSON body: %w", err)
	}
	resp, err := client.Do(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.Headers["ses.message_id"], nil
}

// SESGetSendQuota gets SES send quota
// AWS SDK equivalent: GetSendQuota
func SESGetSendQuota(ctx context.Context, client Client) (*cloud.Response, error) {
	req := &cloud.Request{
		Operation: "ses.get_send_quota",
	}
	return client.Do(ctx, req)
}

// SESVerifyEmailIdentity verifies an email address
// AWS SDK equivalent: VerifyEmailIdentity
func SESVerifyEmailIdentity(ctx context.Context, client Client, email string) error {
	req := &cloud.Request{
		Operation: "ses.verify_email_identity",
		Path:      email,
	}
	_, err := client.Do(ctx, req)
	return err
}

// SESListVerifiedEmailAddresses lists verified email addresses
// AWS SDK equivalent: ListVerifiedEmailAddresses
func SESListVerifiedEmailAddresses(ctx context.Context, client Client) (*cloud.Response, error) {
	req := &cloud.Request{
		Operation: "ses.list_verified_email_addresses",
	}
	return client.Do(ctx, req)
}

// SSMGetParameter gets a parameter from SSM Parameter Store
// AWS SDK equivalent: GetParameter
func SSMGetParameter(ctx context.Context, client Client, name string, decrypt bool) (*cloud.Response, error) {
	req := &cloud.Request{
		Operation:   "ssm.get_parameter",
		Path:        name,
		QueryParams: make(map[string]string),
	}
	if decrypt {
		req.QueryParams["WithDecryption"] = "true"
	}
	return client.Do(ctx, req)
}

// SSMGetParameters gets multiple parameters from SSM Parameter Store
// AWS SDK equivalent: GetParameters
func SSMGetParameters(ctx context.Context, client Client, names []string, decrypt bool) (*cloud.Response, error) {
	req := &cloud.Request{
		Operation:   "ssm.get_parameters",
		QueryParams: make(map[string]string),
	}
	if decrypt {
		req.QueryParams["WithDecryption"] = "true"
	}
	if err := req.WithJSONBody(names); err != nil {
		return nil, fmt.Errorf("failed to marshal JSON body: %w", err)
	}
	return client.Do(ctx, req)
}

// SSMPutParameter puts a parameter into SSM Parameter Store
// AWS SDK equivalent: PutParameter
func SSMPutParameter(ctx context.Context, client Client, name, value, paramType, description string, overwrite bool, tags map[string]string) (*cloud.Response, error) {
	req := &cloud.Request{
		Operation: "ssm.put_parameter",
		Path:      name,
	}
	body := map[string]interface{}{
		"value":     value,
		"type":      paramType,
		"overwrite": overwrite,
	}
	if description != "" {
		body["description"] = description
	}
	if len(tags) > 0 {
		body["tags"] = tags
	}
	if err := req.WithJSONBody(body); err != nil {
		return nil, fmt.Errorf("failed to marshal JSON body: %w", err)
	}
	return client.Do(ctx, req)
}

// SSMDeleteParameter deletes a parameter from SSM Parameter Store
// AWS SDK equivalent: DeleteParameter
func SSMDeleteParameter(ctx context.Context, client Client, name string) error {
	req := &cloud.Request{
		Operation: "ssm.delete_parameter",
		Path:      name,
	}
	_, err := client.Do(ctx, req)
	return err
}

// SSMGetParametersByPath gets parameters by path from SSM Parameter Store
// AWS SDK equivalent: GetParametersByPath
func SSMGetParametersByPath(ctx context.Context, client Client, path string, recursive, decrypt bool) (*cloud.Response, error) {
	req := &cloud.Request{
		Operation:   "ssm.get_parameters_by_path",
		Path:        path,
		QueryParams: make(map[string]string),
	}
	if recursive {
		req.QueryParams["Recursive"] = "true"
	}
	if decrypt {
		req.QueryParams["WithDecryption"] = "true"
	}
	return client.Do(ctx, req)
}
