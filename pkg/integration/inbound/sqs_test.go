package inbound

import (
	"testing"

	"github.com/aws/aws-lambda-go/events"
)

func TestNormalizeSQSEvent(t *testing.T) {
	tests := []struct {
		name    string
		event   *events.SQSEvent
		wantLen int
		wantErr bool
	}{
		{
			name: "single record",
			event: &events.SQSEvent{
				Records: []events.SQSMessage{
					{
						MessageId:     "msg-123",
						Body:          `{"key":"value"}`,
						ReceiptHandle: "receipt-123",
						EventSource:   "aws:sqs",
						EventSourceARN: "arn:aws:sqs:us-east-1:123:my-queue",
					},
				},
			},
			wantLen: 1,
			wantErr: false,
		},
		{
			name: "multiple records",
			event: &events.SQSEvent{
				Records: []events.SQSMessage{
					{MessageId: "msg-1", Body: `{"key":"value1"}`},
					{MessageId: "msg-2", Body: `{"key":"value2"}`},
				},
			},
			wantLen: 2,
			wantErr: false,
		},
		{
			name:    "nil event",
			event:   nil,
			wantLen: 0,
			wantErr: false,
		},
		{
			name:    "empty event",
			event:   &events.SQSEvent{Records: []events.SQSMessage{}},
			wantLen: 0,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requests, err := NormalizeSQSEvent(tt.event)
			if (err != nil) != tt.wantErr {
				t.Errorf("NormalizeSQSEvent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(requests) != tt.wantLen {
				t.Errorf("NormalizeSQSEvent() len = %v, want %v", len(requests), tt.wantLen)
			}
			if tt.wantLen > 0 && requests[0].Operation != "sqs.receive" {
				t.Errorf("NormalizeSQSEvent() operation = %v, want sqs.receive", requests[0].Operation)
			}
		})
	}
}

func TestNormalizeSQSEvent_Headers(t *testing.T) {
	event := &events.SQSEvent{
		Records: []events.SQSMessage{
			{
				MessageId:     "msg-123",
				ReceiptHandle: "receipt-123",
				EventSource:   "aws:sqs",
				EventSourceARN: "arn:aws:sqs:us-east-1:123:my-queue",
			},
		},
	}

	requests, err := NormalizeSQSEvent(event)
	if err != nil {
		t.Fatalf("NormalizeSQSEvent() error = %v", err)
	}

	if len(requests) != 1 {
		t.Fatalf("NormalizeSQSEvent() len = %v, want 1", len(requests))
	}

	req := requests[0]
	if req.Headers["sqs.message_id"] != "msg-123" {
		t.Errorf("Headers[sqs.message_id] = %v, want msg-123", req.Headers["sqs.message_id"])
	}
	if req.Headers["sqs.receipt_handle"] != "receipt-123" {
		t.Errorf("Headers[sqs.receipt_handle] = %v, want receipt-123", req.Headers["sqs.receipt_handle"])
	}
}

func TestNormalizeSQSEvent_Body(t *testing.T) {
	event := &events.SQSEvent{
		Records: []events.SQSMessage{
			{Body: `{"key":"value"}`},
		},
	}

	requests, err := NormalizeSQSEvent(event)
	if err != nil {
		t.Fatalf("NormalizeSQSEvent() error = %v", err)
	}

	if string(requests[0].Body) != `{"key":"value"}` {
		t.Errorf("Body = %v, want {\"key\":\"value\"}", string(requests[0].Body))
	}
}

