package mysqlrenamedb

import "fmt"

type UsageError struct {
	Message string
}

func (e UsageError) Error() string {
	return e.Message
}

func newUsageError(format string, args ...interface{}) error {
	return UsageError{Message: fmt.Sprintf(format, args...)}
}
