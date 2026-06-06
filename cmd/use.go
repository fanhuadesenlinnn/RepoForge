package cmd

import "github.com/spf13/cobra"

func newUseCommand() *cobra.Command {
	var profileName string
	var disable bool
	var remove bool
	command := &cobra.Command{
		Use:   "use",
		Short: "启用或禁用本机 file:// 软件源",
		RunE: func(_ *cobra.Command, _ []string) error {
			return notImplemented("use")
		},
	}
	command.Flags().StringVar(&profileName, "profile", "", "要使用的 profile 名称")
	command.Flags().BoolVar(&disable, "disable", false, "禁用本机软件源")
	command.Flags().BoolVar(&remove, "remove", false, "禁用时删除软件源配置文件")
	_ = command.MarkFlagRequired("profile")
	return command
}
