package cmd

import "github.com/spf13/cobra"

func newServerCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "server",
		Short: "管理只读 HTTP 软件源服务",
	}
	for _, name := range []string{"start", "enable", "stop", "disable", "status"} {
		name := name
		command.AddCommand(&cobra.Command{
			Use:   name,
			Short: serverShortDescription(name),
			RunE: func(_ *cobra.Command, _ []string) error {
				return notImplemented("server " + name)
			},
		})
	}
	return command
}

func serverShortDescription(name string) string {
	switch name {
	case "start":
		return "前台启动 HTTP 服务"
	case "enable":
		return "安装并启用 systemd 服务"
	case "stop":
		return "停止 systemd 服务"
	case "disable":
		return "禁用并删除 systemd 服务"
	default:
		return "查看 HTTP 服务状态"
	}
}
