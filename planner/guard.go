package planner

// StopValidation marks a guard failure which another prose model call cannot
// repair, such as a validation adapter's transport failure or invalid usage.
// The original typed error is returned intact and no writer retry is consumed.
// Ordinary content findings should remain unwrapped to use the repair budget.
func StopValidation(err error) error {
	if err == nil {
		return nil
	}
	return &validationStop{cause: err}
}

type validationStop struct{ cause error }

func (e *validationStop) Error() string { return "contextir: validation stopped" }
func (e *validationStop) Unwrap() error { return e.cause }
