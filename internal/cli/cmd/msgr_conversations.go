package cmd

import (
	"github.com/bilalbayram/metacli/internal/msgr"
	"github.com/bilalbayram/metacli/internal/plugin"
	"github.com/spf13/cobra"
)

func newMSGRConversationsCommand(runtime Runtime, pluginRuntime plugin.Runtime) *cobra.Command {
	conversationsCmd := &cobra.Command{
		Use:   "conversations",
		Short: "Messenger conversation commands",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return requireSubcommand(cmd, "msgr conversations")
		},
	}
	conversationsCmd.AddCommand(newMSGRConversationsListCommand(runtime, pluginRuntime))
	conversationsCmd.AddCommand(newMSGRConversationsReplyCommand(runtime, pluginRuntime))
	return conversationsCmd
}

func newMSGRConversationsListCommand(runtime Runtime, pluginRuntime plugin.Runtime) *cobra.Command {
	var (
		profile string
		version string
		pageID  string
		limit   int
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List Messenger conversations for a page",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := pluginRuntime.Trace(plugin.TraceEvent{
				PluginID:  msgrPluginID,
				Namespace: msgrNamespace,
				Command:   "conversations-list",
			}); err != nil {
				return writeCommandError(cmd, runtime, "meta msgr conversations list", err)
			}

			creds, resolvedVersion, err := resolveMSGRProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta msgr conversations list", err)
			}

			resolvedPageID, err := resolveMSGRPageID(pageID, creds.Profile)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta msgr conversations list", err)
			}

			options := msgr.ListConversationsOptions{
				PageID: resolvedPageID,
				Limit:  limit,
			}
			if _, _, err := msgr.BuildListConversationsRequest(resolvedVersion, creds.Token, creds.AppSecret, options); err != nil {
				return writeCommandError(cmd, runtime, "meta msgr conversations list", err)
			}

			service := msgr.New(msgrNewGraphClient())
			result, err := service.ListConversations(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, options)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta msgr conversations list", err)
			}

			return writeSuccess(cmd, runtime, "meta msgr conversations list", result, result.Pagination, nil)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version")
	cmd.Flags().StringVar(&pageID, "page-id", "", "Facebook Page ID (optional when profile has page_id)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum number of conversations to return")
	return cmd
}

func newMSGRConversationsReplyCommand(runtime Runtime, pluginRuntime plugin.Runtime) *cobra.Command {
	var (
		profile     string
		version     string
		recipientID string
		message     string
	)

	cmd := &cobra.Command{
		Use:   "reply",
		Short: "Reply to a Messenger conversation",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := pluginRuntime.Trace(plugin.TraceEvent{
				PluginID:  msgrPluginID,
				Namespace: msgrNamespace,
				Command:   "conversations-reply",
			}); err != nil {
				return writeCommandError(cmd, runtime, "meta msgr conversations reply", err)
			}

			creds, resolvedVersion, err := resolveMSGRProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta msgr conversations reply", err)
			}

			options := msgr.ReplyOptions{
				RecipientID: recipientID,
				Message:     message,
			}
			if _, err := msgr.BuildReplyRequest(resolvedVersion, creds.Token, creds.AppSecret, options); err != nil {
				return writeCommandError(cmd, runtime, "meta msgr conversations reply", err)
			}

			service := msgr.New(msgrNewGraphClient())
			result, err := service.Reply(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, options)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta msgr conversations reply", err)
			}

			return writeSuccess(cmd, runtime, "meta msgr conversations reply", result, nil, nil)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version")
	cmd.Flags().StringVar(&recipientID, "recipient-id", "", "Page-scoped user ID (PSID)")
	cmd.Flags().StringVar(&message, "message", "", "Message text to send")
	return cmd
}

