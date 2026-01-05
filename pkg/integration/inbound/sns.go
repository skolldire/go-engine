package inbound

import (
	"github.com/aws/aws-lambda-go/events"
	"github.com/skolldire/go-engine/pkg/integration/cloud"
)

// NormalizeSNSEvent converts an AWS SNS Lambda event into a slice of normalized cloud.Request objects.
// If the provided event is nil, it returns (nil, nil).
// For each SNS record it produces a Request with Operation "sns.receive", Path set to the record's TopicArn,
// Method "POST", headers populated from the SNS metadata (message_id, topic_arn, subject, type, timestamp),
// and Body set to the raw bytes of the SNS Message when non-empty.
func NormalizeSNSEvent(event *events.SNSEvent) ([]*cloud.Request, error) {
	if event == nil {
		return nil, nil
	}

	requests := make([]*cloud.Request, 0, len(event.Records))

	for _, record := range event.Records {
		req := &cloud.Request{
			Operation: "sns.receive",
			Path:      record.SNS.TopicArn,
			Method:    "POST", // Optional
			Headers: map[string]string{
				"sns.message_id": record.SNS.MessageID,
				"sns.topic_arn":  record.SNS.TopicArn,
				"sns.subject":    record.SNS.Subject,
				"sns.type":       record.SNS.Type,
				"sns.timestamp":  record.SNS.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
			},
		}

		// Body as raw bytes
		if record.SNS.Message != "" {
			req.Body = []byte(record.SNS.Message)
		}

		requests = append(requests, req)
	}

	return requests, nil
}
