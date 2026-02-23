package cmd

import (
	"errors"
	"strings"

	"github.com/bilalbayram/metacli/internal/config"
	"github.com/bilalbayram/metacli/internal/graph"
	"github.com/bilalbayram/metacli/internal/ig"
	"github.com/bilalbayram/metacli/internal/plugin"
	"github.com/spf13/cobra"
)

const (
	igPluginID  = "instagram"
	igNamespace = "ig"
)

var (
	igLoadProfileCredentials = loadProfileCredentials
	igNewGraphClient         = func() *graph.Client {
		return graph.NewClient(nil, "")
	}
)

func NewIGCommand(runtime Runtime) *cobra.Command {
	tracer, err := plugin.NewNamespaceTracer(igNamespace)
	if err != nil {
		return newPluginErrorCommand(igNamespace, err)
	}

	registry, err := newPluginRegistry(tracer, newIGPluginManifest(runtime))
	if err != nil {
		return newPluginErrorCommand(igNamespace, err)
	}
	return buildCommandFromRegistry(registry, igNamespace)
}

func newIGPluginManifest(runtime Runtime) plugin.Manifest {
	return plugin.Manifest{
		ID:      igPluginID,
		Command: igNamespace,
		Short:   "Instagram Graph commands",
		Build: func(pluginRuntime plugin.Runtime) (*cobra.Command, error) {
			igCmd := &cobra.Command{
				Use:   igNamespace,
				Short: "Instagram Graph commands",
				RunE: func(_ *cobra.Command, _ []string) error {
					return errors.New("ig requires a subcommand")
				},
			}
			igCmd.AddCommand(newIGHealthCommand(runtime, pluginRuntime))
			igCmd.AddCommand(newIGMediaCommand(runtime, pluginRuntime))
			igCmd.AddCommand(newIGCaptionCommand(runtime, pluginRuntime))
			igCmd.AddCommand(newIGPublishCommand(runtime, pluginRuntime))
			return igCmd, nil
		},
	}
}

func newIGHealthCommand(runtime Runtime, pluginRuntime plugin.Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "Verify IG plugin runtime scaffold",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := pluginRuntime.Trace(plugin.TraceEvent{
				PluginID:  igPluginID,
				Namespace: igNamespace,
				Command:   "health",
			}); err != nil {
				return err
			}
			return writeSuccess(cmd, runtime, "meta ig health", map[string]string{
				"namespace": igNamespace,
				"plugin":    igPluginID,
				"status":    "ok",
			}, nil, nil)
		},
	}
}

func newIGMediaCommand(runtime Runtime, pluginRuntime plugin.Runtime) *cobra.Command {
	mediaCmd := &cobra.Command{
		Use:   "media",
		Short: "Instagram media upload and status commands",
		RunE: func(_ *cobra.Command, _ []string) error {
			return errors.New("ig media requires a subcommand")
		},
	}
	mediaCmd.AddCommand(newIGMediaUploadCommand(runtime, pluginRuntime))
	mediaCmd.AddCommand(newIGMediaStatusCommand(runtime, pluginRuntime))
	return mediaCmd
}

func newIGCaptionCommand(runtime Runtime, pluginRuntime plugin.Runtime) *cobra.Command {
	captionCmd := &cobra.Command{
		Use:   "caption",
		Short: "Instagram caption validation commands",
		RunE: func(_ *cobra.Command, _ []string) error {
			return errors.New("ig caption requires a subcommand")
		},
	}
	captionCmd.AddCommand(newIGCaptionValidateCommand(runtime, pluginRuntime))
	return captionCmd
}

func newIGPublishCommand(runtime Runtime, pluginRuntime plugin.Runtime) *cobra.Command {
	publishCmd := &cobra.Command{
		Use:   "publish",
		Short: "Instagram publishing commands",
		RunE: func(_ *cobra.Command, _ []string) error {
			return errors.New("ig publish requires a subcommand")
		},
	}
	publishCmd.AddCommand(newIGPublishFeedCommand(runtime, pluginRuntime))
	return publishCmd
}

func newIGCaptionValidateCommand(runtime Runtime, pluginRuntime plugin.Runtime) *cobra.Command {
	var (
		caption string
		strict  bool
	)

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate an Instagram caption",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := pluginRuntime.Trace(plugin.TraceEvent{
				PluginID:  igPluginID,
				Namespace: igNamespace,
				Command:   "caption-validate",
			}); err != nil {
				return writeCommandError(cmd, runtime, "meta ig caption validate", err)
			}

			result := ig.ValidateCaption(caption, strict)
			if len(result.Errors) > 0 {
				return writeCommandError(cmd, runtime, "meta ig caption validate", errors.New(strings.Join(result.Errors, "; ")))
			}

			return writeSuccess(cmd, runtime, "meta ig caption validate", result, nil, nil)
		},
	}

	cmd.Flags().StringVar(&caption, "caption", "", "Caption text to validate")
	cmd.Flags().BoolVar(&strict, "strict", false, "Treat warnings as errors")
	return cmd
}

func newIGMediaUploadCommand(runtime Runtime, pluginRuntime plugin.Runtime) *cobra.Command {
	var (
		profile        string
		version        string
		igUserID       string
		mediaURL       string
		caption        string
		mediaType      string
		isCarouselItem bool
	)

	cmd := &cobra.Command{
		Use:   "upload",
		Short: "Upload an Instagram media container",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := pluginRuntime.Trace(plugin.TraceEvent{
				PluginID:  igPluginID,
				Namespace: igNamespace,
				Command:   "media-upload",
			}); err != nil {
				return writeCommandError(cmd, runtime, "meta ig media upload", err)
			}

			creds, resolvedVersion, err := resolveIGProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ig media upload", err)
			}

			options := ig.MediaUploadOptions{
				IGUserID:       igUserID,
				MediaURL:       mediaURL,
				Caption:        caption,
				MediaType:      mediaType,
				IsCarouselItem: isCarouselItem,
			}
			if _, _, err := ig.BuildUploadRequest(resolvedVersion, creds.Token, creds.AppSecret, options); err != nil {
				return writeCommandError(cmd, runtime, "meta ig media upload", err)
			}

			service := ig.New(igNewGraphClient())
			result, err := service.Upload(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, options)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ig media upload", err)
			}

			return writeSuccess(cmd, runtime, "meta ig media upload", result, nil, nil)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version")
	cmd.Flags().StringVar(&igUserID, "ig-user-id", "", "Instagram user id")
	cmd.Flags().StringVar(&mediaURL, "media-url", "", "Public media URL")
	cmd.Flags().StringVar(&caption, "caption", "", "Instagram caption")
	cmd.Flags().StringVar(&mediaType, "media-type", ig.MediaTypeImage, "Media type: IMAGE|VIDEO|REELS")
	cmd.Flags().BoolVar(&isCarouselItem, "is-carousel-item", false, "Mark media container as a carousel child")
	return cmd
}

func newIGMediaStatusCommand(runtime Runtime, pluginRuntime plugin.Runtime) *cobra.Command {
	var (
		profile    string
		version    string
		creationID string
	)

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Get Instagram media container processing status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := pluginRuntime.Trace(plugin.TraceEvent{
				PluginID:  igPluginID,
				Namespace: igNamespace,
				Command:   "media-status",
			}); err != nil {
				return writeCommandError(cmd, runtime, "meta ig media status", err)
			}

			creds, resolvedVersion, err := resolveIGProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ig media status", err)
			}

			options := ig.MediaStatusOptions{
				CreationID: creationID,
			}
			if _, err := ig.BuildStatusRequest(resolvedVersion, creds.Token, creds.AppSecret, options); err != nil {
				return writeCommandError(cmd, runtime, "meta ig media status", err)
			}

			service := ig.New(igNewGraphClient())
			result, err := service.Status(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, options)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ig media status", err)
			}

			return writeSuccess(cmd, runtime, "meta ig media status", result, nil, nil)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version")
	cmd.Flags().StringVar(&creationID, "creation-id", "", "Instagram media container creation id")
	return cmd
}

func newIGPublishFeedCommand(runtime Runtime, pluginRuntime plugin.Runtime) *cobra.Command {
	var (
		profile   string
		version   string
		igUserID  string
		mediaURL  string
		caption   string
		mediaType string
		strict    bool
	)

	cmd := &cobra.Command{
		Use:   "feed",
		Short: "Publish Instagram feed media in immediate mode",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := pluginRuntime.Trace(plugin.TraceEvent{
				PluginID:  igPluginID,
				Namespace: igNamespace,
				Command:   "publish-feed",
			}); err != nil {
				return writeCommandError(cmd, runtime, "meta ig publish feed", err)
			}

			creds, resolvedVersion, err := resolveIGProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ig publish feed", err)
			}

			options := ig.FeedPublishOptions{
				IGUserID:   igUserID,
				MediaURL:   mediaURL,
				Caption:    caption,
				MediaType:  mediaType,
				StrictMode: strict,
			}

			captionValidation := ig.ValidateCaption(options.Caption, options.StrictMode)
			if !captionValidation.Valid {
				return writeCommandError(cmd, runtime, "meta ig publish feed", errors.New(strings.Join(captionValidation.Errors, "; ")))
			}

			if _, _, err := ig.BuildUploadRequest(resolvedVersion, creds.Token, creds.AppSecret, ig.MediaUploadOptions{
				IGUserID:  options.IGUserID,
				MediaURL:  options.MediaURL,
				Caption:   options.Caption,
				MediaType: options.MediaType,
			}); err != nil {
				return writeCommandError(cmd, runtime, "meta ig publish feed", err)
			}

			service := ig.New(igNewGraphClient())
			result, err := service.PublishFeedImmediate(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, options)
			if err != nil {
				return writeCommandError(cmd, runtime, "meta ig publish feed", err)
			}

			return writeSuccess(cmd, runtime, "meta ig publish feed", result, nil, nil)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version")
	cmd.Flags().StringVar(&igUserID, "ig-user-id", "", "Instagram user id (required)")
	cmd.Flags().StringVar(&mediaURL, "media-url", "", "Public media URL (required)")
	cmd.Flags().StringVar(&caption, "caption", "", "Instagram caption (required)")
	cmd.Flags().StringVar(&mediaType, "media-type", ig.MediaTypeImage, "Media type: IMAGE|VIDEO|REELS")
	cmd.Flags().BoolVar(&strict, "strict", true, "Treat caption warnings as errors")
	return cmd
}

func resolveIGProfileAndVersion(runtime Runtime, profile string, version string) (*ProfileCredentials, string, error) {
	resolvedProfile := strings.TrimSpace(profile)
	if resolvedProfile == "" {
		resolvedProfile = runtime.ProfileName()
	}
	if resolvedProfile == "" {
		return nil, "", errors.New("profile is required (--profile or global --profile)")
	}

	creds, err := igLoadProfileCredentials(resolvedProfile)
	if err != nil {
		return nil, "", err
	}

	resolvedVersion := strings.TrimSpace(version)
	if resolvedVersion == "" {
		resolvedVersion = creds.Profile.GraphVersion
	}
	if resolvedVersion == "" {
		resolvedVersion = config.DefaultGraphVersion
	}

	return creds, resolvedVersion, nil
}
