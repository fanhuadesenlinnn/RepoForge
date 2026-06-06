package cmd

import "github.com/spf13/cobra"

func newMakeCommand() *cobra.Command {
	var profileName string
	command := &cobra.Command{
		Use:   "make",
		Short: "制作离线软件源",
		RunE: func(_ *cobra.Command, _ []string) error {
			return notImplemented("make")
		},
	}
	command.Flags().StringVar(&profileName, "profile", "", "要制作的 profile 名称")
	_ = command.MarkFlagRequired("profile")
	return command
}
