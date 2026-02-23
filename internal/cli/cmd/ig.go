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
	publishCmd.AddCommand(newIGPublishReelCommand(runtime, pluginRuntime))
	publishCmd.AddCommand(newIGPublishStoryCommand(runtime, pluginRuntime))
	publishCmd.AddCommand(newIGPublishScheduleCommand(runtime, pluginRuntime))
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
	cmd.Flags().StringVar(&mediaType, "media-type", ig.MediaTypeImage, "Media type: IMAGE|VIDEO|REELS|STORIES")
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

type igPublishImmediateSpec struct {
	use              string
	short            string
	traceCommand     string
	commandName      string
	surface          string
	defaultMediaType string
	mediaTypeHelp    string
}

func newIGPublishFeedCommand(runtime Runtime, pluginRuntime plugin.Runtime) *cobra.Command {
	return newIGPublishImmediateCommand(runtime, pluginRuntime, igPublishImmediateSpec{
		use:              "feed",
		short:            "Publish Instagram feed media in immediate mode",
		traceCommand:     "publish-feed",
		commandName:      "meta ig publish feed",
		surface:          ig.PublishSurfaceFeed,
		defaultMediaType: ig.MediaTypeImage,
		mediaTypeHelp:    "Media type: IMAGE|VIDEO",
	})
}

func newIGPublishReelCommand(runtime Runtime, pluginRuntime plugin.Runtime) *cobra.Command {
	return newIGPublishImmediateCommand(runtime, pluginRuntime, igPublishImmediateSpec{
		use:              "reel",
		short:            "Publish Instagram reels media in immediate mode",
		traceCommand:     "publish-reel",
		commandName:      "meta ig publish reel",
		surface:          ig.PublishSurfaceReel,
		defaultMediaType: ig.MediaTypeReels,
		mediaTypeHelp:    "Media type: REELS",
	})
}

func newIGPublishStoryCommand(runtime Runtime, pluginRuntime plugin.Runtime) *cobra.Command {
	return newIGPublishImmediateCommand(runtime, pluginRuntime, igPublishImmediateSpec{
		use:              "story",
		short:            "Publish Instagram stories media in immediate mode",
		traceCommand:     "publish-story",
		commandName:      "meta ig publish story",
		surface:          ig.PublishSurfaceStory,
		defaultMediaType: ig.MediaTypeStories,
		mediaTypeHelp:    "Media type: STORIES",
	})
}

func newIGPublishImmediateCommand(runtime Runtime, pluginRuntime plugin.Runtime, spec igPublishImmediateSpec) *cobra.Command {
	var (
		profile           string
		version           string
		igUserID          string
		mediaURL          string
		caption           string
		mediaType         string
		idempotencyKey    string
		publishAt         string
		scheduleStatePath string
		strict            bool
	)

	cmd := &cobra.Command{
		Use:   spec.use,
		Short: spec.short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := pluginRuntime.Trace(plugin.TraceEvent{
				PluginID:  igPluginID,
				Namespace: igNamespace,
				Command:   spec.traceCommand,
			}); err != nil {
				return writeIGPublishScheduleCommandError(cmd, runtime, spec.commandName, err)
			}

			creds, resolvedVersion, err := resolveIGProfileAndVersion(runtime, profile, version)
			if err != nil {
				return writeIGPublishScheduleCommandError(cmd, runtime, spec.commandName, err)
			}

			options := ig.FeedPublishOptions{
				IGUserID:       igUserID,
				MediaURL:       mediaURL,
				Caption:        caption,
				MediaType:      mediaType,
				StrictMode:     strict,
				IdempotencyKey: idempotencyKey,
			}

			normalizedMediaType, err := ig.ValidatePublishMediaTypeForSurface(spec.surface, options.MediaType)
			if err != nil {
				return writeIGPublishScheduleCommandError(cmd, runtime, spec.commandName, err)
			}
			options.MediaType = normalizedMediaType

			captionValidation := ig.ValidateCaption(options.Caption, options.StrictMode)
			if !captionValidation.Valid {
				return writeIGPublishScheduleCommandError(cmd, runtime, spec.commandName, errors.New(strings.Join(captionValidation.Errors, "; ")))
			}

			if _, _, err := ig.BuildUploadRequest(resolvedVersion, creds.Token, creds.AppSecret, ig.MediaUploadOptions{
				IGUserID:       options.IGUserID,
				MediaURL:       options.MediaURL,
				Caption:        options.Caption,
				MediaType:      options.MediaType,
				IdempotencyKey: options.IdempotencyKey,
			}); err != nil {
				return writeIGPublishScheduleCommandError(cmd, runtime, spec.commandName, err)
			}

			if strings.TrimSpace(publishAt) != "" {
				resolvedSchedulePath, err := resolveIGScheduleStatePath(scheduleStatePath)
				if err != nil {
					return writeIGPublishScheduleCommandError(cmd, runtime, spec.commandName, err)
				}

				scheduleService := ig.NewScheduleService(resolvedSchedulePath)
				result, err := scheduleService.Schedule(ig.SchedulePublishOptions{
					Profile:        creds.Name,
					Version:        resolvedVersion,
					Surface:        spec.surface,
					IdempotencyKey: options.IdempotencyKey,
					IGUserID:       options.IGUserID,
					MediaURL:       options.MediaURL,
					Caption:        options.Caption,
					MediaType:      options.MediaType,
					StrictMode:     options.StrictMode,
					PublishAt:      publishAt,
				})
				if err != nil {
					return writeIGPublishScheduleCommandError(cmd, runtime, spec.commandName, err)
				}
				return writeSuccess(cmd, runtime, spec.commandName, result, nil, nil)
			}

			service := ig.New(igNewGraphClient())
			var result *ig.FeedPublishResult
			switch spec.surface {
			case ig.PublishSurfaceFeed:
				result, err = service.PublishFeedImmediate(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, options)
			case ig.PublishSurfaceReel:
				result, err = service.PublishReelImmediate(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, options)
			case ig.PublishSurfaceStory:
				result, err = service.PublishStoryImmediate(cmd.Context(), resolvedVersion, creds.Token, creds.AppSecret, options)
			default:
				err = errors.New("invalid publish command surface")
			}
			if err != nil {
				return writeIGPublishScheduleCommandError(cmd, runtime, spec.commandName, err)
			}

			return writeSuccess(cmd, runtime, spec.commandName, result, nil, nil)
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile name")
	cmd.Flags().StringVar(&version, "version", "", "Graph API version")
	cmd.Flags().StringVar(&igUserID, "ig-user-id", "", "Instagram user id (required)")
	cmd.Flags().StringVar(&mediaURL, "media-url", "", "Public media URL (required)")
	cmd.Flags().StringVar(&caption, "caption", "", "Instagram caption (required)")
	cmd.Flags().StringVar(&mediaType, "media-type", spec.defaultMediaType, spec.mediaTypeHelp)
	cmd.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "Idempotency key used to suppress duplicate publish/schedule requests")
	cmd.Flags().StringVar(&publishAt, "publish-at", "", "Schedule publish time (RFC3339); when set, publish is scheduled instead of immediate execution")
	cmd.Flags().StringVar(&scheduleStatePath, "schedule-state-path", "", "Schedule state file path (defaults to ~/.meta/ig/schedules.json)")
	cmd.Flags().BoolVar(&strict, "strict", true, "Treat caption warnings as errors")
	return cmd
}

func newIGPublishScheduleCommand(runtime Runtime, pluginRuntime plugin.Runtime) *cobra.Command {
	scheduleCmd := &cobra.Command{
		Use:   "schedule",
		Short: "Instagram publish schedule lifecycle commands",
		RunE: func(_ *cobra.Command, _ []string) error {
			return errors.New("ig publish schedule requires a subcommand")
		},
	}
	scheduleCmd.AddCommand(newIGPublishScheduleListCommand(runtime, pluginRuntime))
	scheduleCmd.AddCommand(newIGPublishScheduleCancelCommand(runtime, pluginRuntime))
	scheduleCmd.AddCommand(newIGPublishScheduleRetryCommand(runtime, pluginRuntime))
	return scheduleCmd
}

func newIGPublishScheduleListCommand(runtime Runtime, pluginRuntime plugin.Runtime) *cobra.Command {
	var (
		status            string
		scheduleStatePath string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List scheduled Instagram publish jobs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := pluginRuntime.Trace(plugin.TraceEvent{
				PluginID:  igPluginID,
				Namespace: igNamespace,
				Command:   "publish-schedule-list",
			}); err != nil {
				return writeIGPublishScheduleCommandError(cmd, runtime, "meta ig publish schedule list", err)
			}

			resolvedSchedulePath, err := resolveIGScheduleStatePath(scheduleStatePath)
			if err != nil {
				return writeIGPublishScheduleCommandError(cmd, runtime, "meta ig publish schedule list", err)
			}

			scheduleService := ig.NewScheduleService(resolvedSchedulePath)
			result, err := scheduleService.List(ig.ScheduleListOptions{
				Status: status,
			})
			if err != nil {
				return writeIGPublishScheduleCommandError(cmd, runtime, "meta ig publish schedule list", err)
			}
			return writeSuccess(cmd, runtime, "meta ig publish schedule list", result, nil, nil)
		},
	}

	cmd.Flags().StringVar(&status, "status", "", "Filter by status: scheduled|canceled|failed")
	cmd.Flags().StringVar(&scheduleStatePath, "schedule-state-path", "", "Schedule state file path (defaults to ~/.meta/ig/schedules.json)")
	return cmd
}

func newIGPublishScheduleCancelCommand(runtime Runtime, pluginRuntime plugin.Runtime) *cobra.Command {
	var (
		scheduleID        string
		scheduleStatePath string
	)

	cmd := &cobra.Command{
		Use:   "cancel",
		Short: "Cancel a scheduled Instagram publish job",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := pluginRuntime.Trace(plugin.TraceEvent{
				PluginID:  igPluginID,
				Namespace: igNamespace,
				Command:   "publish-schedule-cancel",
			}); err != nil {
				return writeIGPublishScheduleCommandError(cmd, runtime, "meta ig publish schedule cancel", err)
			}

			resolvedSchedulePath, err := resolveIGScheduleStatePath(scheduleStatePath)
			if err != nil {
				return writeIGPublishScheduleCommandError(cmd, runtime, "meta ig publish schedule cancel", err)
			}

			scheduleService := ig.NewScheduleService(resolvedSchedulePath)
			result, err := scheduleService.Cancel(ig.ScheduleCancelOptions{
				ScheduleID: scheduleID,
			})
			if err != nil {
				return writeIGPublishScheduleCommandError(cmd, runtime, "meta ig publish schedule cancel", err)
			}
			return writeSuccess(cmd, runtime, "meta ig publish schedule cancel", result, nil, nil)
		},
	}

	cmd.Flags().StringVar(&scheduleID, "schedule-id", "", "Schedule identifier")
	cmd.Flags().StringVar(&scheduleStatePath, "schedule-state-path", "", "Schedule state file path (defaults to ~/.meta/ig/schedules.json)")
	return cmd
}

func newIGPublishScheduleRetryCommand(runtime Runtime, pluginRuntime plugin.Runtime) *cobra.Command {
	var (
		scheduleID        string
		publishAt         string
		scheduleStatePath string
	)

	cmd := &cobra.Command{
		Use:   "retry",
		Short: "Retry a canceled or failed Instagram publish schedule",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := pluginRuntime.Trace(plugin.TraceEvent{
				PluginID:  igPluginID,
				Namespace: igNamespace,
				Command:   "publish-schedule-retry",
			}); err != nil {
				return writeIGPublishScheduleCommandError(cmd, runtime, "meta ig publish schedule retry", err)
			}

			resolvedSchedulePath, err := resolveIGScheduleStatePath(scheduleStatePath)
			if err != nil {
				return writeIGPublishScheduleCommandError(cmd, runtime, "meta ig publish schedule retry", err)
			}

			scheduleService := ig.NewScheduleService(resolvedSchedulePath)
			result, err := scheduleService.Retry(ig.ScheduleRetryOptions{
				ScheduleID: scheduleID,
				PublishAt:  publishAt,
			})
			if err != nil {
				return writeIGPublishScheduleCommandError(cmd, runtime, "meta ig publish schedule retry", err)
			}
			return writeSuccess(cmd, runtime, "meta ig publish schedule retry", result, nil, nil)
		},
	}

	cmd.Flags().StringVar(&scheduleID, "schedule-id", "", "Schedule identifier")
	cmd.Flags().StringVar(&publishAt, "publish-at", "", "Retry publish time (RFC3339); defaults to existing schedule time")
	cmd.Flags().StringVar(&scheduleStatePath, "schedule-state-path", "", "Schedule state file path (defaults to ~/.meta/ig/schedules.json)")
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

func resolveIGScheduleStatePath(path string) (string, error) {
	resolvedPath := strings.TrimSpace(path)
	if resolvedPath != "" {
		return resolvedPath, nil
	}
	return ig.DefaultScheduleStatePath()
}

func writeIGPublishScheduleCommandError(cmd *cobra.Command, runtime Runtime, commandName string, err error) error {
	return writeCommandError(cmd, runtime, commandName, ig.ClassifyPublishScheduleError(err))
}
