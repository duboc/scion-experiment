package store

import "fmt"

// Code classifies store errors so callers can map them to API error codes.
type Code int

// Error codes returned by the store.
const (
	CodeInvalidArgument Code = iota + 1
	CodeNotFound
	CodeFailedPrecondition
)

// Error is returned by Store methods. Message is safe to show to API clients.
type Error struct {
	Code    Code
	Message string
}

func (e *Error) Error() string { return e.Message }

// Is reports whether target is an *Error with the same Code, so that
// errors.Is(err, ErrNotFound) works for any not-found error.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && t.Code == e.Code
}

// Sentinels for use with errors.Is.
var (
	ErrInvalidArgument    = &Error{Code: CodeInvalidArgument, Message: "invalid argument"}
	ErrNotFound           = &Error{Code: CodeNotFound, Message: "not found"}
	ErrFailedPrecondition = &Error{Code: CodeFailedPrecondition, Message: "failed precondition"}
)

func invalidf(format string, args ...any) error {
	return &Error{Code: CodeInvalidArgument, Message: fmt.Sprintf(format, args...)}
}

func notFound(id string) error {
	return &Error{Code: CodeNotFound, Message: fmt.Sprintf("incident %q not found", id)}
}
