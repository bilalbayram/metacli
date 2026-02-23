package ops

import "errors"

func Initialize(statePath string) (InitResult, error) {
	state, err := InitBaseline(statePath)
	if err != nil {
		switch {
		case errors.Is(err, ErrStatePathRequired):
			return InitResult{}, WrapExit(ExitCodeInput, err)
		case errors.Is(err, ErrBaselineAlreadyExist):
			return InitResult{}, WrapExit(ExitCodeInput, err)
		default:
			return InitResult{}, WrapExit(ExitCodeState, err)
		}
	}
	return InitResult{
		StatePath: statePath,
		State:     state,
	}, nil
}

func Run(statePath string) (RunResult, error) {
	state, err := LoadBaseline(statePath)
	if err != nil {
		switch {
		case errors.Is(err, ErrStatePathRequired):
			return RunResult{}, WrapExit(ExitCodeInput, err)
		default:
			return RunResult{}, WrapExit(ExitCodeState, err)
		}
	}
	return RunResult{
		StatePath: statePath,
		Report:    NewReportSkeleton(state),
	}, nil
}
