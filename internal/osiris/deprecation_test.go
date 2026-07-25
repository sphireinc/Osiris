package osiris

import (
	"strings"
	"testing"
)

func TestDeprecationWarningContract(t *testing.T) {
	warning := DeprecationWarning{Code: "OSIRIS-D001", Message: "old is deprecated", Replacement: "new", RemovalMajor: 2}
	if !strings.Contains(warning.String(), "OSIRIS-D001") || !strings.Contains(warning.String(), "major 2") {
		t.Fatalf("warning string = %q", warning.String())
	}
	for _, value := range []string{"OSIRIS-D001", "other, OSIRIS-D001", "all"} {
		if !warning.IsSuppressed(value) {
			t.Fatalf("warning not suppressed by %q", value)
		}
	}
	if warning.IsSuppressed("OSIRIS-D002") {
		t.Fatal("different warning code suppressed active warning")
	}
}
