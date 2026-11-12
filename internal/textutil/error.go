package textutil

type cleanedError struct{ err error }

// CleanError wraps an error whose text comes from an external command, so that
// its message is safe to print on a terminal while errors.Is and errors.As
// still see the original error.
func CleanError(err error) error {
	return cleanedError{err: err}
}

func (e cleanedError) Error() string { return Clean(e.err.Error()) }
func (e cleanedError) Unwrap() error { return e.err }
