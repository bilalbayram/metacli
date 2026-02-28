package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

type subcommandRequiredError struct {
	err error
}

func (e *subcommandRequiredError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *subcommandRequiredError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func (e *subcommandRequiredError) AlreadyPrinted() bool {
	return true
}

func requireSubcommand(cmd *cobra.Command, commandName string) error {
	message := fmt.Sprintf("%s requires a subcommand", commandName)
	if cmd == nil {
		return &subcommandRequiredError{err: errors.New(message)}
	}

	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	_, _ = fmt.Fprintln(cmd.ErrOrStderr(), message)

	stdout := cmd.OutOrStdout()
	stderr := cmd.ErrOrStderr()
	if stdout != stderr {
		cmd.SetOut(stderr)
		defer cmd.SetOut(stdout)
	}
	if err := cmd.Help(); err != nil {
		return &subcommandRequiredError{err: fmt.Errorf("%s: print help: %w", message, err)}
	}
	return &subcommandRequiredError{err: errors.New(message)}
}
