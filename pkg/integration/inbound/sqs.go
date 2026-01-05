package inbound

import (
	"github.com/aws/aws-lambda-go/events"
	"github.com/skolldire/go-engine/pkg/integration/cloud"
)

// NormalizeSQSEvent converts an AWS Lambda SQS event into a slice of normalized cloud.Request pointers.
// For a nil input it returns (nil, nil). Each returned Request has Operation "sqs.receive", Path set to
// the record's EventSourceARN, Method "POST", and Headers containing "sqs.message_id", "sqs.receipt_handle",
// "sqs.event_source", and "sqs.event_source_arn". If a record Body is non-empty it is preserved as raw bytes
// in Request.Body. Message attributes with string values are collected, serialized, and stored under the
// "sqs.message_attributes" header. The implementation currently always returns a nil error.
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

// serializeAttrs converts a map of string attributes into a JSON-like string of key/value pairs in the form {"key":"value",...}.
// The iteration order is unspecified (map order) and keys/values are not escaped, so this is a simplistic serializer for simple attribute sets.
func serializeAttrs(attrs map[string]string) string {
	// Simple serialization - could use json.Marshal for more complex cases
	result := "{"
	first := true
	for k, v := range attrs {
		if !first {
			result += ","
		}
		result += `"` + k + `":"` + v + `"`
		first = false
	}
	result += "}"
	return result
}


