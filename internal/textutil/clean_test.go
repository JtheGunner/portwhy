package textutil

import "testing"

func TestClean(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"plain", "postgres", "postgres"},
		{"keeps printable non-ASCII", "→ LM Studio · Zürich", "→ LM Studio · Zürich"},
		{"escape sequence", "evil\x1b[2Jname", "evil?[2Jname"},
		{"carriage return", "safe\rkill -TERM 1", "safe?kill -TERM 1"},
		{"newline and tab", "a\nb\tc", "a?b?c"},
		{"bell", "a\x07b", "a?b"},
		{"C1 control CSI", "a\u009bb", "a?b"},
		{"invalid UTF-8", "a\x9bb", "a�b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Clean(tt.in); got != tt.want {
				t.Errorf("Clean(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
