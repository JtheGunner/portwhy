package textutil

import (
	"errors"
	"strings"
	"testing"
)

func TestCleanError_CleansMessageAndKeepsCause(t *testing.T) {
	cause := errors.New("boom\x1b[2J\rspoof")

	err := CleanError(cause)

	if strings.ContainsAny(err.Error(), "\x1b\r") {
		t.Errorf("Error() = %q, want it free of control characters", err.Error())
	}
	if !errors.Is(err, cause) {
		t.Error("errors.Is does not find the original error")
	}
}
