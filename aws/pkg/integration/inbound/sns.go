package inbound

import (
	"github.com/aws/aws-lambda-go/events"
	"github.com/skolldire/go-engine/pkg/integration/cloud"
)

// NormalizeSNSEvent converts SNS Lambda event to normalized Request(s)
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
