package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestReplUsesLanguageParserForDeclarationsAndExpressions(t *testing.T) {
	declaration, err, complete := replSourceForInput("const answer = 2 + 3")
	if err != nil || !complete {
		t.Fatalf("const source = %q, err=%v, complete=%v", declaration, err, complete)
	}
	if declaration != "const answer = 2 + 3" {
		t.Fatalf("declaration source = %q, want unchanged source", declaration)
	}

	expression, err, complete := replSourceForInput("2 + 3")
	if err != nil || !complete {
		t.Fatalf("expression source = %q, err=%v, complete=%v", expression, err, complete)
	}
	if expression != "emit 2 + 3" {
		t.Fatalf("expression source = %q, want implicit emit", expression)
	}
}

func TestReplExecutesDeclarationsAndMultilineBlocks(t *testing.T) {
	input := strings.NewReader("const answer = 2 + 3\nvar total = answer\ntotal = total + 4\nemit total\nfunc plusOne(value) {\n return value + 1\n}\nemit plusOne(2)\n:q\n")
	var output bytes.Buffer
	if err := runReplWithInput(context.Background(), input, &output); err != nil {
		t.Fatalf("runReplWithInput() error = %v\noutput:\n%s", err, output.String())
	}
	got := output.String()
	if !strings.Contains(got, "9u64\n") {
		t.Fatalf("REPL output = %q, want mutable declaration result 9u64", got)
	}
	if !strings.Contains(got, "3u64\n") {
		t.Fatalf("REPL output = %q, want multiline function result 3u64", got)
	}
	if !strings.Contains(got, "....> ") {
		t.Fatalf("REPL output = %q, want deterministic continuation prompt", got)
	}
}

func TestReplReportsIncompleteInputAtEOF(t *testing.T) {
	var output bytes.Buffer
	err := runReplWithInput(context.Background(), strings.NewReader("func broken(value) {\n"), &output)
	if err == nil || !strings.Contains(err.Error(), "incomplete input") {
		t.Fatalf("runReplWithInput() error = %v, want incomplete-input error", err)
	}
	if strings.Contains(output.String(), "broken") {
		t.Fatalf("incomplete source was executed: %q", output.String())
	}
}

func TestReplParserHistoryKeepsDeclaredTypesVisible(t *testing.T) {
	history := "struct Point { value u64 }\n" +
		"type Count = u64\n" +
		"newtype PointID u64"
	source, err, complete := replSourceForInputWithHistory(history, "let point = Point { value: Count(3u64) }")
	if err != nil || !complete {
		t.Fatalf("history parse source = %q, err=%v, complete=%v", source, err, complete)
	}
}

func TestReplParserHistoryClassifiesTypeSequence(t *testing.T) {
	history := ""
	for _, line := range []string{
		"struct Point { value u64 }",
		"type Count = u64",
		"newtype PointID u64",
		"let point = Point { value: Count(3u64) }",
	} {
		source, err, complete := replSourceForInputWithHistory(history, line)
		if err != nil || !complete {
			t.Fatalf("line %q classified as source=%q, err=%v, complete=%v, history=%q", line, source, err, complete, history)
		}
		if history == "" {
			history = source
		} else {
			history += "\n" + source
		}
	}
}
