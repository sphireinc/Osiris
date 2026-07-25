package osiris

import (
	"strings"
	"testing"
)

func TestDecodeValueBytesRejectsOversizedPayload(t *testing.T) {
	data := make([]byte, maxCodecValueBytes+1)
	copy(data, codecHeader)
	if _, err := decodeValueBytes(data); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized codec payload error = %v", err)
	}
}
