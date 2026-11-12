package kill

import (
	"errors"
	"fmt"
	"io"
	"strings"
)

// confirm writes question to out and reads one line from in. Only "y" and
// "yes" (case-insensitive) count as consent; anything else, including end of
// input, is a no.
func confirm(in io.Reader, out io.Writer, question string) (bool, error) {
	if in == nil || out == nil {
		return false, ErrNotInteractive
	}
	if _, err := fmt.Fprint(out, question); err != nil {
		return false, err
	}
	line, err := readLine(in)
	if err != nil {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

// readLine reads up to the next newline one byte at a time, so that no input
// beyond the line is consumed (a later prompt may read from the same stream).
func readLine(in io.Reader) (string, error) {
	var sb strings.Builder
	buf := make([]byte, 1)
	for {
		n, err := in.Read(buf)
		if n == 1 {
			if buf[0] == '\n' {
				return sb.String(), nil
			}
			sb.WriteByte(buf[0])
		}
		if errors.Is(err, io.EOF) {
			return sb.String(), nil
		}
		if err != nil {
			return "", err
		}
	}
}
