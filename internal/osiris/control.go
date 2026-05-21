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

func newFlow(kind flowKind, value Value) error {
	return &flowSignal{kind: kind, value: value}
}

func asFlow(err error) (*flowSignal, bool) {
	if err == nil {
		return nil, false
	}
	sig, ok := err.(*flowSignal)
	return sig, ok
}

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
