package cloud

import (
	"testing"
)

func TestResponse_UnmarshalBody(t *testing.T) {
	tests := []struct {
		name    string
		body    []byte
		wantErr bool
	}{
		{
			name:    "valid JSON",
			body:    []byte(`{"key":"value"}`),
			wantErr: false,
		},
		{
			name:    "empty body",
			body:    []byte{},
			wantErr: true,
		},
		{
			name:    "invalid JSON",
			body:    []byte(`{invalid}`),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &Response{Body: tt.body}
			var result map[string]interface{}
			err := resp.UnmarshalBody(&result)
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalBody() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestResponse_BodyString(t *testing.T) {
	resp := &Response{Body: []byte("test body")}
	if resp.BodyString() != "test body" {
		t.Errorf("BodyString() = %v, want test body", resp.BodyString())
	}
}

func TestResponse_Complete(t *testing.T) {
	resp := &Response{
		StatusCode: 200,
		Body:       []byte(`{"key":"value"}`),
		Headers:    map[string]string{"header": "value"},
		Metadata:   map[string]interface{}{"meta": "value"},
	}

	if resp.StatusCode != 200 {
		t.Errorf("StatusCode = %v, want 200", resp.StatusCode)
	}
	if len(resp.Body) == 0 {
		t.Errorf("Body should not be empty")
	}
}
