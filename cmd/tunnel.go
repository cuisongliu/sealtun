package cmd

import "github.com/spf13/cobra"

// tunnelCmd groups every operation that acts on an existing tunnel:
// lifecycle, traffic inspection, access policy, shares, and domains.
var tunnelCmd = &cobra.Command{
	Use:   "tunnel",
	Short: "Inspect, operate, and manage existing tunnels",
	Args:  cobra.NoArgs,
}

// tunnelAccessCmd groups access-policy operations under tunnel.
var tunnelAccessCmd = &cobra.Command{
	Use:   "access",
	Short: "Manage HTTPS access policy and audit data for a tunnel",
	Args:  cobra.NoArgs,
}

// tunnelShareCmd groups temporary access links under tunnel.
var tunnelShareCmd = &cobra.Command{
	Use:   "share",
	Short: "Manage temporary access links for HTTPS tunnels",
	Args:  cobra.NoArgs,
}

// tunnelDomainCmd groups custom-domain operations under tunnel.
var tunnelDomainCmd = &cobra.Command{
	Use:   "domain",
	Short: "Manage custom domains for a tunnel",
	Args:  cobra.NoArgs,
}

func init() {
	rootCmd.AddCommand(tunnelCmd)
	tunnelCmd.AddCommand(tunnelAccessCmd, tunnelShareCmd, tunnelDomainCmd)
}

// aliasCommand builds a hidden command that forwards to target while sharing
// its flag set, so every legacy flat command keeps working unchanged while
// the canonical path moves under `sealtun tunnel`.
func aliasCommand(target *cobra.Command) *cobra.Command {
	alias := *target
	alias.Hidden = true
	alias.Short = target.Short + " (alias for `" + target.CommandPath() + "`)"
	alias.Long = "Deprecated alias for `" + target.CommandPath() + "`; use the new path instead."
	alias.Aliases = nil
	return &alias
}

// aliasGroup rebuilds a hidden alias for a parent command group, inheriting
// the original parent's persistent flags; the children it receives are the
// already-aliased leaf commands.
func aliasGroup(parent *cobra.Command, children ...*cobra.Command) *cobra.Command {
	group := &cobra.Command{
		Use:    parent.Use,
		Short:  "Deprecated alias; commands moved under `sealtun tunnel`",
		Hidden: true,
		Args:   cobra.NoArgs,
	}
	group.PersistentFlags().AddFlagSet(parent.PersistentFlags())
	group.AddCommand(children...)
	return group
}
