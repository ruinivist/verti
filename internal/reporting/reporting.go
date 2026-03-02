package reporting

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

type Class string

const (
	ClassConfig  Class = "config"
	ClassStore   Class = "store"
	ClassRestore Class = "restore"
)

type Error struct {
	Class   Class
	Message string
	Err     error
}

func (e *Error) Error() string {
	if strings.TrimSpace(e.Message) == "" {
		return fmt.Sprintf("%s error", e.Class)
	}
	return fmt.Sprintf("%s: %s", e.Class, e.Message)
}

func (e *Error) Unwrap() error {
	return e.Err
}

func Wrap(class Class, message string, err error) error {
	if err == nil {
		return nil
	}
	return &Error{
		Class:   class,
		Message: message,
		Err:     err,
	}
}

func Format(err error, debug bool) string {
	if err == nil {
		return ""
	}

	var wrapped *Error
	if errors.As(err, &wrapped) {
		if !debug || wrapped.Err == nil {
			return wrapped.Error()
		}
		return fmt.Sprintf("%s: %v", wrapped.Error(), wrapped.Err)
	}

	return err.Error()
}

func DebugEnabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("VERTI_DEBUG")))
	switch raw {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
