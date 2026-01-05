package cloud

import (
	"testing"
	"time"
)

func TestRequest_WithJSONBody(t *testing.T) {
	tests := []struct {
		name    string
		input   interface{}
		wantErr bool
	}{
		{
			name:    "valid map",
			input:   map[string]string{"key": "value"},
			wantErr: false,
		},
		{
			name:    "valid struct",
			input:   struct{ Name string }{"test"},
			wantErr: false,
		},
		{
			name:    "nil input",
			input:   nil,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &Request{}
			err := req.WithJSONBody(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("WithJSONBody() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(req.Body) == 0 && tt.input != nil {
				t.Errorf("WithJSONBody() body is empty but should have content")
			}
		})
	}
}

func TestRequest_WithBody(t *testing.T) {
	req := &Request{}
	body := []byte("test body")
	result := req.WithBody(body)

	if result != req {
		t.Errorf("WithBody() should return the same request")
	}
	if string(req.Body) != string(body) {
		t.Errorf("WithBody() body = %v, want %v", req.Body, body)
	}
}

func TestRequest_Complete(t *testing.T) {
	req := &Request{
		Operation:  "sqs.send",
		Path:       "my-queue",
		Timeout:    5 * time.Second,
		Method:     "POST",
		Headers:    map[string]string{"key": "value"},
		QueryParams: map[string]string{"param": "value"},
	}

	if req.Operation != "sqs.send" {
		t.Errorf("Operation = %v, want sqs.send", req.Operation)
	}
	if req.Path != "my-queue" {
		t.Errorf("Path = %v, want my-queue", req.Path)
	}
}

