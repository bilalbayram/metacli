package cmd

import (
	"github.com/bilalbayram/metacli/internal/ig"
	"github.com/bilalbayram/metacli/internal/plugin"
	"github.com/spf13/cobra"
)

func newIGConversationsCommand(runtime Runtime, pluginRuntime plugin.Runtime) *cobra.Command {
	conversationsCmd := &cobra.Command{
		Use:   "conversations",
		Short: "Instagram conversation commands",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return requireSubcommand(cmd, "ig conversations")
		},
	}
	conversationsCmd.AddCommand(newIGConversationsListCommand(runtime, pluginRuntime))
	conversationsCmd.AddCommand(newIGConversationsReplyCommand(runtime, pluginRuntime))
	return conversationsCmd
}

func newIGConversationsListCommand(runtime Runtime, pluginRuntime plugin.Runtime) *cobra.Command {
	var (
		profile  string
		version  string
		igUserID string
		platform string
		limit    int
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List Instagram DM conversations",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := pluginRuntime.Trace(plugin.TraceEvent{
				PluginID:  igPluginID,
				Namespace: igNamespace,
				Command:   "conversations-list",
			}); err != nil {
				return writeCommandError(cmd, runtime, "meta ig conversations list", err)
			}

			creds, resolvedVersion, err := resolveIGProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ig conversations list", err)
			}
			resolvedIGUserID := resolveIGUserID(igUserID, creds.Profile)

			options := ig.ConversationListOptions{
				IGUserID: resolvedIGUserID,
				Platform: platform,
				Limit:    limit,
			}
			if _, _, err := ig.BuildConversationListRequest(resolvedVersion, creds.Token, creds.AppSecret, options); err != nil {
				return writeCommandError(cmd, runtime, "meta ig conversations list", err)
			}

			service := ig.New(igNewGraphClient())
			result, err := service.ListConversations(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, options)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ig conversations list", err)
			}

			return writeSuccess(cmd, runtime, "meta ig conversations list", result, result.Pagination, nil)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version")
	cmd.Flags().StringVar(&igUserID, "ig-user-id", "", "Instagram user id (optional when profile has ig_user_id)")
	cmd.Flags().StringVar(&platform, "platform", "instagram", "Messaging platform filter")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum number of conversations to return")
	return cmd
}

func newIGConversationsReplyCommand(runtime Runtime, pluginRuntime plugin.Runtime) *cobra.Command {
	var (
		profile     string
		version     string
		igUserID    string
		recipientID string
		message     string
	)

	cmd := &cobra.Command{
		Use:   "reply",
		Short: "Reply to an Instagram DM conversation",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := pluginRuntime.Trace(plugin.TraceEvent{
				PluginID:  igPluginID,
				Namespace: igNamespace,
				Command:   "conversations-reply",
			}); err != nil {
				return writeCommandError(cmd, runtime, "meta ig conversations reply", err)
			}

			creds, resolvedVersion, err := resolveIGProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ig conversations reply", err)
			}
			resolvedIGUserID := resolveIGUserID(igUserID, creds.Profile)

			options := ig.ConversationReplyOptions{
				IGUserID:    resolvedIGUserID,
				RecipientID: recipientID,
				Message:     message,
			}
			if _, err := ig.BuildConversationReplyRequest(resolvedVersion, creds.Token, creds.AppSecret, options); err != nil {
				return writeCommandError(cmd, runtime, "meta ig conversations reply", err)
			}

			service := ig.New(igNewGraphClient())
			result, err := service.ReplyToConversation(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, options)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ig conversations reply", err)
			}

			return writeSuccess(cmd, runtime, "meta ig conversations reply", result, nil, nil)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version")
	cmd.Flags().StringVar(&igUserID, "ig-user-id", "", "Instagram user id (optional when profile has ig_user_id)")
	cmd.Flags().StringVar(&recipientID, "recipient-id", "", "Instagram-scoped user ID")
	cmd.Flags().StringVar(&message, "message", "", "Message text to send")
	return cmd
}
