package inbound

import (
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
)

func TestNormalizeSNSEvent(t *testing.T) {
	tests := []struct {
		name    string
		event   *events.SNSEvent
		wantLen int
		wantErr bool
	}{
		{
			name: "single record",
			event: &events.SNSEvent{
				Records: []events.SNSEventRecord{
					{
						SNS: events.SNSEntity{
							MessageID: "msg-123",
							Message:   `{"key":"value"}`,
							TopicArn:  "arn:aws:sns:us-east-1:123:my-topic",
							Subject:   "Test Subject",
							Type:      "Notification",
							Timestamp: time.Now(),
						},
					},
				},
			},
			wantLen: 1,
			wantErr: false,
		},
		{
			name:    "nil event",
			event:   nil,
			wantLen: 0,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requests, err := NormalizeSNSEvent(tt.event)
			if (err != nil) != tt.wantErr {
				t.Errorf("NormalizeSNSEvent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(requests) != tt.wantLen {
				t.Errorf("NormalizeSNSEvent() len = %v, want %v", len(requests), tt.wantLen)
			}
			if tt.wantLen > 0 && requests[0].Operation != "sns.receive" {
				t.Errorf("NormalizeSNSEvent() operation = %v, want sns.receive", requests[0].Operation)
			}
		})
	}
}
