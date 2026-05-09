package store

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)

func NotFoundError(resource, id string) error {
	if id == "" {
		return fmt.Errorf("%w: %s", ErrNotFound, resource)
	}
	return fmt.Errorf("%w: %s %q", ErrNotFound, resource, id)
}

func ConflictError(msg string) error {
	return fmt.Errorf("%w: %s", ErrConflict, msg)
}

func IsConstraintError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint") ||
		strings.Contains(msg, "constraint failed") ||
		strings.Contains(msg, "constraint violation") ||
		strings.Contains(msg, "duplicate key value violates unique constraint")
}
