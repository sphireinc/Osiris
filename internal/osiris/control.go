package osiris

import "fmt"

type flowKind uint8

const (
	flowReturn flowKind = iota + 1
	flowBreak
	flowContinue
)

type flowSignal struct {
	kind  flowKind
	value Value
}

// Error operates on flowSignal values and keeps the flowsignal-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (f *flowSignal) Error() string {
	switch f.kind {
	case flowReturn:
		return "return"
	case flowBreak:
		return "break"
	case flowContinue:
		return "continue"
	default:
		return "control flow"
	}
}

// newFlow handles the new flow step in the surrounding package and returns
// error.
//
// It returns an error when the operation cannot complete cleanly.
func newFlow(kind flowKind, value Value) error {
	return &flowSignal{kind: kind, value: value}
}

// asFlow handles the as flow step in the surrounding package and returns
// *flowSignal, bool.
//
// It returns *flowSignal, bool so callers can continue with the next stage of
// the pipeline without re-deriving the same information.
func asFlow(err error) (*flowSignal, bool) {
	if err == nil {
		return nil, false
	}
	sig, ok := err.(*flowSignal)
	return sig, ok
}

// flowError handles the flow error step in the surrounding package and returns
// error.
//
// It returns an error when the operation cannot complete cleanly.
func flowError(kind flowKind, where string) error {
	switch kind {
	case flowReturn:
		return fmt.Errorf("return used outside of a function")
	case flowBreak:
		return fmt.Errorf("break used outside of a loop")
	case flowContinue:
		return fmt.Errorf("continue used outside of a loop")
	default:
		return fmt.Errorf("%s used outside of control flow", where)
	}
}
