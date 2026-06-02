package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestInitLoggerFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := InitWithWriter("test-service", "test", &buf)
	logger.Info("hello", "key", "value")

	output := buf.String()

	if !strings.Contains(output, `"service":"test-service"`) {
		t.Errorf("output missing service field: %s", output)
	}
	if !strings.Contains(output, `"env":"test"`) {
		t.Errorf("output missing env field: %s", output)
	}
	if !strings.Contains(output, `"msg":"hello"`) {
		t.Errorf("output missing msg: %s", output)
	}

	var m map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &m); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, output)
	}
}

func TestContextRoundTrip(t *testing.T) {
	logger := slog.Default()
	ctx := NewContext(context.Background(), logger)

	retrieved := FromContext(ctx)
	if retrieved != logger {
		t.Error("FromContext should return the same logger")
	}
}

func TestFromContextEmpty(t *testing.T) {
	logger := FromContext(context.Background())
	if logger == nil {
		t.Error("FromContext should return default logger when none in context")
	}
}
