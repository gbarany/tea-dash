package gitea

import (
	"errors"
	"fmt"
)

// HTTPError is returned by raw REST helpers for non-2xx responses.
type HTTPError struct {
	Method     string
	Path       string
	StatusCode int
	Status     string
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("gitea %s %s: %s: %s", e.Method, e.Path, e.Status, e.Body)
}

func isHTTPStatus(err error, code int) bool {
	var httpErr *HTTPError
	return errors.As(err, &httpErr) && httpErr.StatusCode == code
}
