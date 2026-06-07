package review

import "fmt"

const (
	scanOpLoadState = "load_state"
	scanOpRefresh   = "refresh"
)

// ScanError wraps fatal discovery failures with operation context.
type ScanError struct {
	Operation string
	Root      string
	Err       error
}

func (e *ScanError) Error() string {
	return fmt.Sprintf("%s scan failed for %q: %v", e.Operation, e.Root, e.Err)
}

func (e *ScanError) Unwrap() error {
	return e.Err
}

func newScanError(operation, root string, err error) error {
	if err == nil {
		return nil
	}
	return &ScanError{
		Operation: operation,
		Root:      root,
		Err:       err,
	}
}
