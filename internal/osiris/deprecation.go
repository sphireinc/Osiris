package osiris

import (
	"strconv"
	"strings"
)

// DeprecationWarning is a stable, machine-recognizable warning contract for a
// feature that remains available temporarily.
type DeprecationWarning struct {
	Code         string
	Message      string
	Replacement  string
	RemovalMajor int
}

func (w DeprecationWarning) String() string {
	return w.Code + ": " + w.Message + "; use " + w.Replacement + " before major " + strconv.Itoa(w.RemovalMajor)
}

// IsSuppressed applies the documented comma-separated warning suppression
// policy. It deliberately accepts only an exact code or the explicit all.
func (w DeprecationWarning) IsSuppressed(value string) bool {
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item == "all" || item == w.Code {
			return true
		}
	}
	return false
}
