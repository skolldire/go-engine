package aws

import (
	"context"
	"fmt"

	"github.com/skolldire/go-engine/pkg/integration/cloud"
)

// SQSSendMessage sends a message to SQS queue (convenience wrapper)
// Uses WithJSONBody() internally for ergonomic JSON serialization
// SQSSendMessage sends the provided value as a JSON message to the SQS queue identified by queueURL and returns the SQS-assigned message ID.
// It returns an error if marshalling the value or executing the request fails.
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
// SQSSendMessageBytes sends raw bytes as a message to the specified SQS queue.
// It returns the SQS message ID from the response headers, or an error if the request fails.
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

// QSReceiveMessage receives messages from SQS queue
// QSReceiveMessage requests messages from the SQS queue at queueURL using maxMessages and waitTimeSeconds to control batching and long polling.
// It returns the cloud.Response from the service or an error.
func QSReceiveMessage(ctx context.Context, client Client, queueURL string, maxMessages int32, waitTimeSeconds int32) (*cloud.Response, error) {
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
// SQSDeleteMessage deletes a message from the specified SQS queue using the provided receipt handle.
// It sends a delete request to the given queue URL and returns any error encountered while performing the operation.
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
// SQSCreateQueue creates a new SQS queue with the given name and attributes.
// It returns the created queue's URL from the response headers, or an error if the request fails.
// The attributes map is sent as queue attributes; each key is included using the "sqs.queue_attribute." prefix.
func SQSCreateQueue(ctx context.Context, client Client, queueName string, attributes map[string]string) (queueURL string, err error) {
	req := &cloud.Request{
		Operation: "sqs.create_queue",
		Path:      queueName,
		Headers:  make(map[string]string),
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
// SQSDeleteQueue deletes the SQS queue identified by queueURL.
// It returns any error encountered while performing the delete operation.
func SQSDeleteQueue(ctx context.Context, client Client, queueURL string) error {
	req := &cloud.Request{
		Operation: "sqs.delete_queue",
		Path:      queueURL,
	}
	_, err := client.Do(ctx, req)
	return err
}

// SQSListQueues lists SQS queues
// SQSListQueues lists SQS queue URLs, optionally filtered by the provided queue name prefix.
// It returns the cloud response containing the list of queue URLs or an error.
func SQSListQueues(ctx context.Context, client Client, prefix string) (*cloud.Response, error) {
	req := &cloud.Request{
		Operation: "sqs.list_queues",
		QueryParams: make(map[string]string),
	}
	if prefix != "" {
		req.QueryParams["QueueNamePrefix"] = prefix
	}
	return client.Do(ctx, req)
}

// SQSGetQueueURL gets the URL of an SQS queue by name
// SQSGetQueueURL retrieves the URL for the SQS queue identified by queueName.
// It returns the queue URL extracted from the response headers, or an error if the request fails.
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
// SNSPublish publishes v to the SNS topic identified by topicARN.
// It returns the SNS message ID from the response headers, or an error if JSON marshaling or the request fails.
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
// LambdaInvoke invokes the Lambda function identified by functionName using v as the JSON payload and returns the cloud service response.
// If v cannot be marshaled to JSON or the request fails, an error is returned.
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
// S3PutObject uploads body to S3 at the path formed by bucket/key.
// It sets the optional content type and prefixes metadata keys with `s3.metadata.` in request headers, and returns the cloud response or an error.
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
// S3GetObject retrieves the object stored at the specified bucket and key.
// It returns the operation response containing object data or an error.
func S3GetObject(ctx context.Context, client Client, bucket, key string) (*cloud.Response, error) {
	req := &cloud.Request{
		Operation: "s3.get_object",
		Path:      fmt.Sprintf("%s/%s", bucket, key),
	}
	return client.Do(ctx, req)
}

// S3DeleteObject deletes an object from S3
// AWS SDK equivalent: DeleteObject
// S3DeleteObject deletes the object identified by bucket and key from S3.
// The bucket and key are combined as "bucket/key" to form the request path.
// It returns an error if the delete request fails.
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
// S3HeadObject retrieves metadata for the object identified by bucket and key in S3.
// The returned *cloud.Response contains the object's metadata when successful, or an error otherwise.
func S3HeadObject(ctx context.Context, client Client, bucket, key string) (*cloud.Response, error) {
	req := &cloud.Request{
		Operation: "s3.head_object",
		Path:      fmt.Sprintf("%s/%s", bucket, key),
	}
	return client.Do(ctx, req)
}

// S3ListObjects lists objects in S3 bucket
// AWS SDK equivalent: ListObjectsV2
// S3ListObjects lists objects in an S3 bucket, optionally scoped to a prefix.
// If prefix is non-empty the request path is "bucket/prefix"; otherwise the path is "bucket".
// When maxKeys is greater than zero the value is sent as the MaxKeys query parameter to limit results.
// It returns the service response or an error.
func S3ListObjects(ctx context.Context, client Client, bucket, prefix string, maxKeys int32) (*cloud.Response, error) {
	path := bucket
	if prefix != "" {
		path = fmt.Sprintf("%s/%s", bucket, prefix)
	}
	req := &cloud.Request{
		Operation: "s3.list_objects",
		Path:      path,
		QueryParams: make(map[string]string),
	}
	if maxKeys > 0 {
		req.QueryParams["MaxKeys"] = fmt.Sprintf("%d", maxKeys)
	}
	return client.Do(ctx, req)
}

// S3CopyObject copies an object within S3
// AWS SDK equivalent: CopyObject
// S3CopyObject copies an object from the source bucket/key to the destination bucket/key.
// The request path is formatted as "destBucket/destKey"; the source bucket and key are provided in the `s3.source_bucket` and `s3.source_key` headers.
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
// SESSendEmail sends an email through SES using the provided message fields.
// 
// The emailMessage map must contain the message properties such as "from", "to",
// "subject", "body_html", "body_text", "cc", "bcc", and "reply_to" as applicable.
// 
// It returns the SES message ID extracted from the response header "ses.message_id",
// or an error if the message body cannot be marshaled to JSON or the request fails.
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
// SESSendRawEmail sends a raw MIME email via SES to the specified destinations.
// It returns the SES message ID from response headers on success, or an error if marshaling or the request fails.
func SESSendRawEmail(ctx context.Context, client Client, rawMessage []byte, destinations []string) (messageID string, err error) {
	req := &cloud.Request{
		Operation: "ses.send_raw_email",
	}
	body := map[string]interface{}{
		"raw_message": string(rawMessage),
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
// SESGetSendQuota calls the SES GetSendQuota operation and returns the service response.
// It returns a cloud.Response containing send quota information or an error.
func SESGetSendQuota(ctx context.Context, client Client) (*cloud.Response, error) {
	req := &cloud.Request{
		Operation: "ses.get_send_quota",
	}
	return client.Do(ctx, req)
}

// SESVerifyEmailIdentity verifies an email address
// SESVerifyEmailIdentity sends a request to Amazon SES to verify the specified email identity.
// It returns an error if the verification request fails.
func SESVerifyEmailIdentity(ctx context.Context, client Client, email string) error {
	req := &cloud.Request{
		Operation: "ses.verify_email_identity",
		Path:      email,
	}
	_, err := client.Do(ctx, req)
	return err
}

// SESListVerifiedEmailAddresses lists verified email addresses
// SESListVerifiedEmailAddresses retrieves the list of email addresses verified in Amazon SES.
// The returned Response contains the verified email addresses as returned by the service; an error is returned if the request fails.
func SESListVerifiedEmailAddresses(ctx context.Context, client Client) (*cloud.Response, error) {
	req := &cloud.Request{
		Operation: "ses.list_verified_email_addresses",
	}
	return client.Do(ctx, req)
}

// SSMGetParameter gets a parameter from SSM Parameter Store
// SSMGetParameter retrieves a Systems Manager Parameter Store parameter by name.
// If decrypt is true and the parameter is a SecureString, the returned parameter value is decrypted.
func SSMGetParameter(ctx context.Context, client Client, name string, decrypt bool) (*cloud.Response, error) {
	req := &cloud.Request{
		Operation: "ssm.get_parameter",
		Path:      name,
		QueryParams: make(map[string]string),
	}
	if decrypt {
		req.QueryParams["WithDecryption"] = "true"
	}
	return client.Do(ctx, req)
}

// SSMGetParameters gets multiple parameters from SSM Parameter Store
// SSMGetParameters retrieves one or more Systems Manager Parameter Store parameters by name.
// If decrypt is true the request asks for decrypted parameter values.
// It sends the parameter names as a JSON body and returns the service response, or an error if JSON marshaling or the client request fails.
func SSMGetParameters(ctx context.Context, client Client, names []string, decrypt bool) (*cloud.Response, error) {
	req := &cloud.Request{
		Operation: "ssm.get_parameters",
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
// SSMPutParameter stores or updates a Systems Manager (SSM) Parameter Store parameter with the given name, value, and type.
// If description is non-empty it is included; tags are attached when provided; overwrite controls whether an existing
// parameter is replaced. It returns the cloud response from the service or an error.
func SSMPutParameter(ctx context.Context, client Client, name, value, paramType, description string, overwrite bool, tags map[string]string) (*cloud.Response, error) {
	req := &cloud.Request{
		Operation: "ssm.put_parameter",
		Path:      name,
	}
	body := map[string]interface{}{
		"value":    value,
		"type":     paramType,
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
// SSMDeleteParameter deletes the Systems Manager Parameter Store parameter identified by name.
// It returns an error if the deletion request fails.
func SSMDeleteParameter(ctx context.Context, client Client, name string) error {
	req := &cloud.Request{
		Operation: "ssm.delete_parameter",
		Path:      name,
	}
	_, err := client.Do(ctx, req)
	return err
}

// SSMGetParametersByPath gets parameters by path from SSM Parameter Store
// SSMGetParametersByPath retrieves Systems Manager Parameter Store parameters under the given path.
// When recursive is true, the request includes the Recursive query parameter; when decrypt is true, it includes WithDecryption.
// It executes the request using the provided Client and returns the resulting cloud.Response or an error.
func SSMGetParametersByPath(ctx context.Context, client Client, path string, recursive, decrypt bool) (*cloud.Response, error) {
	req := &cloud.Request{
		Operation: "ssm.get_parameters_by_path",
		Path:      path,
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
