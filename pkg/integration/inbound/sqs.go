package inbound

import (
	"encoding/json"

	"github.com/aws/aws-lambda-go/events"
	"github.com/skolldire/go-engine/pkg/integration/cloud"
)

// NormalizeSQSEvent converts SQS Lambda event to normalized Request(s)
func NormalizeSQSEvent(event *events.SQSEvent) ([]*cloud.Request, error) {
	if event == nil {
		return nil, nil
	}

	requests := make([]*cloud.Request, 0, len(event.Records))

	for _, record := range event.Records {
		req := &cloud.Request{
			Operation: "sqs.receive",
			Path:      record.EventSourceARN, // or extract queue name
			Method:    "POST",                 // Optional, mainly for documentation
			Headers: map[string]string{
				"sqs.message_id":       record.MessageId,
				"sqs.receipt_handle":   record.ReceiptHandle,
				"sqs.event_source":     record.EventSource,
				"sqs.event_source_arn": record.EventSourceARN,
			},
		}

		// Body as raw bytes (preserve original encoding)
		if record.Body != "" {
			req.Body = []byte(record.Body)
		}

		// Handle message attributes
		if len(record.MessageAttributes) > 0 {
			attrs := make(map[string]string)
			for k, v := range record.MessageAttributes {
				if v.StringValue != nil {
					attrs[k] = *v.StringValue
				}
			}
			// Store as JSON string in headers
			if len(attrs) > 0 {
				req.Headers["sqs.message_attributes"] = serializeAttrs(attrs)
			}
		}

		requests = append(requests, req)
	}

	return requests, nil
}

// serializeAttrs converts map to JSON string
func serializeAttrs(attrs map[string]string) string {
	jsonBytes, err := json.Marshal(attrs)
	if err != nil {
		// Fallback to empty JSON object if marshaling fails
		return "{}"
	}
	return string(jsonBytes)
}



