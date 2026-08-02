package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestTask123CLIJSONAndInventory(t *testing.T) {
	fixture := filepath.Join("..", "..", "internal", "osiris", "testdata", "task-123", "basic_test.osr")
	var output, stderr bytes.Buffer
	if code := execute(context.Background(), []string{"test", "-json", "-run", "addition", fixture}, &output, &stderr); code != cliExitSuccess {
		t.Fatalf("test exit=%d stdout=%s stderr=%s", code, output.String(), stderr.String())
	}
	var event testEvent
	if err := json.Unmarshal(output.Bytes(), &event); err != nil {
		t.Fatal(err)
	}
	if event.SchemaVersion != 1 || event.Event != "test" || event.Status != "ok" || event.Location.Start.Line == 0 || len(event.Assertions) != 2 {
		t.Fatalf("event = %#v", event)
	}
	output.Reset()
	if err := run(context.Background(), []string{"test", "-list", "-json", fixture}, &output, &stderr); err != nil {
		t.Fatal(err)
	}
	var inventory struct {
		SchemaVersion int `json:"schema_version"`
		Tests         []struct {
			Name string `json:"name"`
		} `json:"tests"`
	}
	if err := json.Unmarshal(output.Bytes(), &inventory); err != nil || inventory.SchemaVersion != 1 || len(inventory.Tests) != 5 {
		t.Fatalf("inventory = %s, err=%v", output.String(), err)
	}
}
