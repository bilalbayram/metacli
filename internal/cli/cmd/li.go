package cmd

import "github.com/spf13/cobra"

func NewLICommand(runtime Runtime) *cobra.Command {
	liCmd := &cobra.Command{
		Use:   "li",
		Short: "LinkedIn Marketing commands",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return requireSubcommand(cmd, "li")
		},
	}

	liCmd.AddCommand(newLIAuthCommand(runtime))
	liCmd.AddCommand(newLIApiCommand(runtime))
	liCmd.AddCommand(newLIAccountCommand(runtime))
	liCmd.AddCommand(newLIOrganizationCommand(runtime))
	liCmd.AddCommand(newLICampaignGroupCommand(runtime))
	liCmd.AddCommand(newLICampaignCommand(runtime))
	liCmd.AddCommand(newLICreativeCommand(runtime))
	liCmd.AddCommand(newLIInsightsCommand(runtime))
	liCmd.AddCommand(newLITargetingCommand(runtime))
	liCmd.AddCommand(newLILeadFormCommand(runtime))
	liCmd.AddCommand(newLILeadCommand(runtime))
	return liCmd
}

func newLIAuthCommand(runtime Runtime) *cobra.Command {
	cmd := newLISubcommandGroup("auth", "LinkedIn auth commands")
	cmd.AddCommand(newLIAuthSetupCommand(runtime))
	cmd.AddCommand(newLIAuthValidateCommand(runtime))
	cmd.AddCommand(newLIAuthScopesCommand(runtime))
	cmd.AddCommand(newLIAuthWhoAmICommand(runtime))
	return cmd
}

func newLIApiCommand(runtime Runtime) *cobra.Command {
	cmd := newLISubcommandGroup("api", "Raw LinkedIn API commands")
	cmd.AddCommand(newLIApiGetCommand(runtime))
	cmd.AddCommand(newLIApiPostCommand(runtime))
	cmd.AddCommand(newLIApiDeleteCommand(runtime))
	return cmd
}

func newLIAccountCommand(runtime Runtime) *cobra.Command {
	cmd := newLISubcommandGroup("account", "LinkedIn ad account commands")
	cmd.AddCommand(newLIAccountListCommand(runtime))
	cmd.AddCommand(newLIAccountRolesCommand(runtime))
	return cmd
}

func newLIOrganizationCommand(runtime Runtime) *cobra.Command {
	cmd := newLISubcommandGroup("organization", "LinkedIn organization commands")
	cmd.AddCommand(newLIOrganizationListCommand(runtime))
	cmd.AddCommand(newLIOrganizationGetCommand(runtime))
	cmd.AddCommand(newLIOrganizationRolesCommand(runtime))
	return cmd
}

func newLICampaignGroupCommand(runtime Runtime) *cobra.Command {
	cmd := newLISubcommandGroup("campaign-group", "LinkedIn campaign group commands")
	cmd.AddCommand(newLICampaignGroupListCommand(runtime))
	cmd.AddCommand(newLICampaignGroupGetCommand(runtime))
	cmd.AddCommand(newLICampaignGroupUpdateCommand(runtime))
	cmd.AddCommand(newLICampaignGroupPauseCommand(runtime))
	cmd.AddCommand(newLICampaignGroupResumeCommand(runtime))
	return cmd
}

func newLICampaignCommand(runtime Runtime) *cobra.Command {
	cmd := newLISubcommandGroup("campaign", "LinkedIn campaign commands")
	cmd.AddCommand(newLICampaignListCommand(runtime))
	cmd.AddCommand(newLICampaignGetCommand(runtime))
	cmd.AddCommand(newLICampaignCreateCommand(runtime))
	cmd.AddCommand(newLICampaignUpdateCommand(runtime))
	cmd.AddCommand(newLICampaignPauseCommand(runtime))
	cmd.AddCommand(newLICampaignResumeCommand(runtime))
	return cmd
}

func newLICreativeCommand(runtime Runtime) *cobra.Command {
	cmd := newLISubcommandGroup("creative", "LinkedIn creative commands")
	cmd.AddCommand(newLICreativeListCommand(runtime))
	cmd.AddCommand(newLICreativeGetCommand(runtime))
	cmd.AddCommand(newLICreativeCreateCommand(runtime))
	cmd.AddCommand(newLICreativeUpdateCommand(runtime))
	cmd.AddCommand(newLICreativeArchiveCommand(runtime))
	return cmd
}

func newLIInsightsCommand(runtime Runtime) *cobra.Command {
	cmd := newLISubcommandGroup("insights", "LinkedIn reporting commands")
	cmd.AddCommand(newLIInsightsRunCommand(runtime))
	cmd.AddCommand(newLIInsightsMetricsListCommand(runtime))
	cmd.AddCommand(newLIInsightsPivotsListCommand(runtime))
	cmd.AddCommand(newLIInsightsDemographicRunCommand(runtime))
	return cmd
}

func newLITargetingCommand(runtime Runtime) *cobra.Command {
	cmd := newLISubcommandGroup("targeting", "LinkedIn targeting commands")
	cmd.Long = "LinkedIn targeting commands. Respect LinkedIn anti-discrimination requirements and avoid building audience criteria around protected characteristics."
	cmd.AddCommand(newLITargetingFacetsCommand(runtime))
	cmd.AddCommand(newLITargetingEntitiesCommand(runtime))
	cmd.AddCommand(newLITargetingSearchCommand(runtime))
	cmd.AddCommand(newLITargetingSimilarCommand(runtime))
	cmd.AddCommand(newLITargetingValidateCommand(runtime))
	return cmd
}

func newLILeadFormCommand(runtime Runtime) *cobra.Command {
	cmd := newLISubcommandGroup("lead-form", "LinkedIn lead form commands")
	cmd.AddCommand(newLILeadFormListCommand(runtime))
	cmd.AddCommand(newLILeadFormGetCommand(runtime))
	cmd.AddCommand(newLILeadFormCreateCommand(runtime))
	return cmd
}

func newLILeadCommand(runtime Runtime) *cobra.Command {
	cmd := newLISubcommandGroup("lead", "LinkedIn lead commands")
	cmd.AddCommand(newLILeadListCommand(runtime))
	cmd.AddCommand(newLILeadGetCommand(runtime))
	cmd.AddCommand(newLILeadSyncCommand(runtime))
	cmd.AddCommand(newLILeadWebhookCommand(runtime))
	return cmd
}

func newLISubcommandGroup(use string, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return requireSubcommand(cmd, "li "+use)
		},
	}
}
