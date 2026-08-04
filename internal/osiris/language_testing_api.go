package osiris

import (
	"fmt"
	"strings"
	"sync"
)

// languageTestState is private runtime state installed for one language test
// case. It gives std/testing the same test-scoped ownership model as the
// command-line runner without exposing a Go callback or host object to source.
type languageTestState struct {
	mu         sync.Mutex
	name       string
	failed     bool
	skipped    bool
	control    string
	message    string
	logs       []string
	assertions []LanguageTestAssertionResult
}

type languageTestControlError struct {
	kind    string
	message string
}

func (e *languageTestControlError) Error() string {
	if e == nil || e.message == "" {
		return "language test control flow"
	}
	return e.message
}

func newLanguageTestState(name string) *languageTestState {
	return &languageTestState{name: name}
}

func (s *languageTestState) failure(message string, expression string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.failed = true
	if message == "" {
		message = "test assertion failed"
	}
	s.assertions = append(s.assertions, LanguageTestAssertionResult{Expression: expression, Passed: false, Message: message})
	s.mu.Unlock()
}

func (s *languageTestState) log(message string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.logs = append(s.logs, message)
	s.mu.Unlock()
}

func (s *languageTestState) setControl(kind, message string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.control == "" {
		s.control = kind
		s.message = message
	}
	if kind == "skip" {
		s.skipped = true
	} else {
		s.failed = true
	}
	s.mu.Unlock()
}

func (s *languageTestState) snapshot() (failed bool, skipped bool, control, message string, logs []string, assertions []LanguageTestAssertionResult) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.failed, s.skipped, s.control, s.message, append([]string(nil), s.logs...), append([]LanguageTestAssertionResult(nil), s.assertions...)
}

func testingState(rt *Runtime) (*languageTestState, error) {
	if rt == nil || rt.languageTest == nil {
		return nil, fmt.Errorf("std/testing is available only while running an Osiris language test")
	}
	return rt.languageTest, nil
}

func testingMessage(args []Value, format bool) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("testing message expects at least one argument")
	}
	if !format {
		parts := make([]string, len(args))
		for i, arg := range args {
			parts[i] = arg.String()
		}
		return strings.Join(parts, " "), nil
	}
	pattern, err := args[0].AsString()
	if err != nil {
		return "", fmt.Errorf("testing format must be a string: %w", err)
	}
	values := make([]any, len(args)-1)
	for i, arg := range args[1:] {
		// Value.String is the stable language representation and avoids
		// leaking private host representations into test output.
		values[i] = arg.String()
	}
	return fmt.Sprintf(pattern, values...), nil
}

func testingAssertValue(rt *Runtime, args []Value) (Value, error) {
	state, err := testingState(rt)
	if err != nil {
		return nullValue(), err
	}
	if len(args) < 1 || args[0].kind != kindBool {
		return nullValue(), fmt.Errorf("testing.assert expects a bool and an optional message")
	}
	passed := args[0].boo
	message := "test assertion failed"
	if len(args) > 1 {
		message, err = testingMessage(args[1:], false)
		if err != nil {
			return nullValue(), err
		}
	}
	if !passed {
		state.failure(message, "assert")
	}
	return boolValue(passed), nil
}

func testingAssertEqualValue(rt *Runtime, args []Value) (Value, error) {
	state, err := testingState(rt)
	if err != nil {
		return nullValue(), err
	}
	if len(args) < 2 {
		return nullValue(), fmt.Errorf("testing.assert_equal expects got, want, and an optional message")
	}
	passed := args[0].Equal(args[1])
	message := fmt.Sprintf("got %s, want %s", args[0].String(), args[1].String())
	if len(args) > 2 {
		message, err = testingMessage(args[2:], false)
		if err != nil {
			return nullValue(), err
		}
	}
	if !passed {
		state.failure(message, "assert_equal")
	}
	return boolValue(passed), nil
}

func testingErrorValue(rt *Runtime, args []Value) (Value, error) {
	state, err := testingState(rt)
	if err != nil {
		return nullValue(), err
	}
	message, err := testingMessage(args, false)
	if err != nil {
		return nullValue(), err
	}
	state.failure(message, "error")
	return nullValue(), nil
}

func testingErrorfValue(rt *Runtime, args []Value) (Value, error) {
	state, err := testingState(rt)
	if err != nil {
		return nullValue(), err
	}
	message, err := testingMessage(args, true)
	if err != nil {
		return nullValue(), err
	}
	state.failure(message, "errorf")
	return nullValue(), nil
}

func testingFatalValue(rt *Runtime, args []Value, format bool) (Value, error) {
	state, err := testingState(rt)
	if err != nil {
		return nullValue(), err
	}
	message, err := testingMessage(args, format)
	if err != nil {
		return nullValue(), err
	}
	state.setControl("fatal", message)
	return nullValue(), &languageTestControlError{kind: "fatal", message: message}
}

func testingSkipValue(rt *Runtime, args []Value, format bool) (Value, error) {
	state, err := testingState(rt)
	if err != nil {
		return nullValue(), err
	}
	message, err := testingMessage(args, format)
	if err != nil {
		return nullValue(), err
	}
	state.setControl("skip", message)
	return nullValue(), &languageTestControlError{kind: "skip", message: message}
}

func testingLogValue(rt *Runtime, args []Value, format bool) (Value, error) {
	state, err := testingState(rt)
	if err != nil {
		return nullValue(), err
	}
	message, err := testingMessage(args, format)
	if err != nil {
		return nullValue(), err
	}
	state.log(message)
	return nullValue(), nil
}

func testingNameValue(rt *Runtime, args []Value) (Value, error) {
	state, err := testingState(rt)
	if err != nil {
		return nullValue(), err
	}
	if len(args) != 0 {
		return nullValue(), fmt.Errorf("testing.name expects no arguments")
	}
	state.mu.Lock()
	name := state.name
	state.mu.Unlock()
	return stringValue(name), nil
}

func testingFailedValue(rt *Runtime, args []Value) (Value, error) {
	state, err := testingState(rt)
	if err != nil {
		return nullValue(), err
	}
	if len(args) != 0 {
		return nullValue(), fmt.Errorf("testing.failed expects no arguments")
	}
	state.mu.Lock()
	failed := state.failed
	state.mu.Unlock()
	return boolValue(failed), nil
}

func languageTestingModule(rt *Runtime) map[string]Value {
	return map[string]Value{
		"assert":       functionValueOf("std/testing.assert", func(args []Value) (Value, error) { return testingAssertValue(rt, args) }),
		"assert_equal": functionValueOf("std/testing.assert_equal", func(args []Value) (Value, error) { return testingAssertEqualValue(rt, args) }),
		"error":        functionValueOf("std/testing.error", func(args []Value) (Value, error) { return testingErrorValue(rt, args) }),
		"errorf":       functionValueOf("std/testing.errorf", func(args []Value) (Value, error) { return testingErrorfValue(rt, args) }),
		"fatal":        functionValueOf("std/testing.fatal", func(args []Value) (Value, error) { return testingFatalValue(rt, args, false) }),
		"fatalf":       functionValueOf("std/testing.fatalf", func(args []Value) (Value, error) { return testingFatalValue(rt, args, true) }),
		"skip":         functionValueOf("std/testing.skip", func(args []Value) (Value, error) { return testingSkipValue(rt, args, false) }),
		"skipf":        functionValueOf("std/testing.skipf", func(args []Value) (Value, error) { return testingSkipValue(rt, args, true) }),
		"log":          functionValueOf("std/testing.log", func(args []Value) (Value, error) { return testingLogValue(rt, args, false) }),
		"logf":         functionValueOf("std/testing.logf", func(args []Value) (Value, error) { return testingLogValue(rt, args, true) }),
		"name":         functionValueOf("std/testing.name", func(args []Value) (Value, error) { return testingNameValue(rt, args) }),
		"failed":       functionValueOf("std/testing.failed", func(args []Value) (Value, error) { return testingFailedValue(rt, args) }),
	}
}
