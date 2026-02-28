package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/bilalbayram/metacli/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		var exitErr *cli.ExitError
		if errors.As(err, &exitErr) {
			if !errorAlreadyPrinted(err) {
				fmt.Fprintln(os.Stderr, exitErr.Error())
			}
			os.Exit(exitErr.Code)
		}

		if !errorAlreadyPrinted(err) {
			fmt.Fprintln(os.Stderr, err.Error())
		}
		os.Exit(cli.ExitCodeUnknown)
	}
}

type alreadyPrintedError interface {
	AlreadyPrinted() bool
}

func errorAlreadyPrinted(err error) bool {
	var marker alreadyPrintedError
	return errors.As(err, &marker) && marker.AlreadyPrinted()
}
