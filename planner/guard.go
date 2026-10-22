package planner

func StopValidation(err error) error {
	if err == nil {
		return nil
	}
	return &validationStop{cause: err}
}

type validationStop struct{ cause error }

func (e *validationStop) Error() string { return "contextir: validation stopped" }
func (e *validationStop) Unwrap() error { return e.cause }
