package svc

//noinspection ALL
func NewBadResult(e error) ExitResult {
	return NewExitResult(1, e)
}

func NewSuccessResult() ExitResult {
	return NewExitResult(0, nil)
}

func NewExitResult(code int, e error) ExitResult {
	return ExitResult{
		Code:  code,
		Error: e,
	}
}
