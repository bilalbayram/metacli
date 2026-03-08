package cmd

import (
	"github.com/bilalbayram/metacli/internal/msgr"
	"github.com/bilalbayram/metacli/internal/plugin"
	"github.com/spf13/cobra"
)

func newMSGRAutoReplyCommand(runtime Runtime, pluginRuntime plugin.Runtime) *cobra.Command {
	autoReplyCmd := &cobra.Command{
		Use:   "auto-reply",
		Short: "Messenger auto-reply greeting commands",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return requireSubcommand(cmd, "msgr auto-reply")
		},
	}
	autoReplyCmd.AddCommand(newMSGRAutoReplySetCommand(runtime, pluginRuntime))
	return autoReplyCmd
}

func newMSGRAutoReplySetCommand(runtime Runtime, pluginRuntime plugin.Runtime) *cobra.Command {
	var (
		profile string
		version string
		pageID  string
		message string
		locale  string
	)

	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set Messenger greeting text for a page",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := pluginRuntime.Trace(plugin.TraceEvent{
				PluginID:  msgrPluginID,
				Namespace: msgrNamespace,
				Command:   "auto-reply-set",
			}); err != nil {
				return writeCommandError(cmd, runtime, "meta msgr auto-reply set", err)
			}

			creds, resolvedVersion, err := resolveMSGRProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta msgr auto-reply set", err)
			}

			resolvedPageID, err := resolveMSGRPageID(pageID, creds.Profile)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta msgr auto-reply set", err)
			}

			options := msgr.SetGreetingOptions{
				PageID:  resolvedPageID,
				Message: message,
				Locale:  locale,
			}
			if _, _, err := msgr.BuildSetGreetingRequest(resolvedVersion, creds.Token, creds.AppSecret, options); err != nil {
				return writeCommandError(cmd, runtime, "meta msgr auto-reply set", err)
			}

			service := msgr.New(msgrNewGraphClient())
			result, err := service.SetGreeting(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, options)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta msgr auto-reply set", err)
			}

			return writeSuccess(cmd, runtime, "meta msgr auto-reply set", result, nil, nil)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version")
	cmd.Flags().StringVar(&pageID, "page-id", "", "Facebook Page ID (optional when profile has page_id)")
	cmd.Flags().StringVar(&message, "message", "", "Greeting message text")
	cmd.Flags().StringVar(&locale, "locale", "", "Locale for greeting (defaults to \"default\")")
	return cmd
}
