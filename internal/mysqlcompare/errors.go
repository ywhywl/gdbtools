package mysqlcompare

type usageError struct {
	message string
}

func (e usageError) Error() string {
	return e.message
}

func newUsageError(message string) error {
	return usageError{message: message}
}
