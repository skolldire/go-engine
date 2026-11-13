package logging_test

import (
	"bytes"
	"encoding/json"
	"testing"
	"testing/slogtest"

	logging "github.com/skolldire/go-engine/logging"
)

func TestStandardHandlerContract(t *testing.T) {
	var b bytes.Buffer
	cfg := logging.DefaultConfig()
	cfg.RedactKeys = nil
	l := newLogger(t, cfg, logging.Dependencies{Output: &b})
	err := slogtest.TestHandler(l.Handler(), func() []map[string]any {
		var result []map[string]any
		for _, line := range bytes.Split(bytes.TrimSpace(b.Bytes()), []byte("\n")) {
			var entry map[string]any
			if err := json.Unmarshal(line, &entry); err != nil {
				t.Fatal(err)
			}
			result = append(result, entry)
		}
		return result
	})
	if err != nil {
		t.Fatal(err)
	}
}
