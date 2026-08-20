package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fanhuadesenlinnn/RepoForge/internal/home"
	"github.com/fanhuadesenlinnn/RepoForge/internal/sign"
	"github.com/spf13/cobra"
)

func newGPGCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "gpg",
		Short: "管理 OpenPGP 签名密钥",
		Long: `管理用于签名离线源元数据的 OpenPGP 密钥（纯 Go 实现，无需本机 gpg）。

  repoforge gpg init    生成签名密钥对（Ed25519）到 home/config/signing/
  repoforge gpg export  导出公钥（供客户端配置 signed-by / gpgkey）

repo.yaml 中 signing.enabled: true 后，sync / make 生成的
repomd.xml（RPM）与 Release（DEB）会被自动签名。`,
		Args: noArgs,
	}
	command.AddCommand(
		newGPGInitCommand(),
		newGPGExportCommand(),
	)
	return command
}

func signingDir(homeDir string) string {
	return filepath.Join(homeDir, "config", "signing")
}

func newGPGInitCommand() *cobra.Command {
	var name, email string
	command := &cobra.Command{
		Use:   "init",
		Short: "生成 OpenPGP 签名密钥对",
		Args:  noArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			homeDir, err := home.Detect(false)
			if err != nil {
				return err
			}
			dir := signingDir(homeDir)
			privPath := filepath.Join(dir, "private.key")
			if _, err := os.Stat(privPath); err == nil {
				return fmt.Errorf("私钥已存在：%s（如需重新生成请先删除）", privPath)
			}
			priv, pub, fingerprint, err := sign.Generate(name, email)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(dir, 0o700); err != nil {
				return err
			}
			if err := os.WriteFile(privPath, priv, 0o600); err != nil {
				return err
			}
			pubPath := filepath.Join(dir, "public.key")
			if err := os.WriteFile(pubPath, pub, 0o644); err != nil {
				return err
			}
			fmt.Fprintf(command.OutOrStdout(), "已生成签名密钥对：\n")
			fmt.Fprintf(command.OutOrStdout(), "  私钥: %s\n", privPath)
			fmt.Fprintf(command.OutOrStdout(), "  公钥: %s\n", pubPath)
			fmt.Fprintf(command.OutOrStdout(), "  指纹: %s\n", fingerprint)
			fmt.Fprintf(command.OutOrStdout(), "\n在 repo.yaml 中设置 signing.enabled: true 以启用自动签名。\n")
			return nil
		},
	}
	command.Flags().StringVar(&name, "name", sign.DefaultKeyName, "密钥身份名称")
	command.Flags().StringVar(&email, "email", sign.DefaultKeyEmail, "密钥身份邮箱")
	return command
}

func newGPGExportCommand() *cobra.Command {
	var output string
	command := &cobra.Command{
		Use:   "export",
		Short: "导出公钥",
		Args:  noArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			homeDir, err := home.Detect(false)
			if err != nil {
				return err
			}
			pubPath := filepath.Join(signingDir(homeDir), "public.key")
			pub, err := os.ReadFile(pubPath)
			if err != nil {
				return fmt.Errorf("未找到公钥 %s（请先运行 repoforge gpg init）: %w", pubPath, err)
			}
			if output == "" {
				_, err = command.OutOrStdout().Write(pub)
				return err
			}
			if err := os.WriteFile(output, pub, 0o644); err != nil {
				return err
			}
			fmt.Fprintf(command.OutOrStdout(), "已导出公钥到 %s\n", output)
			return nil
		},
	}
	command.Flags().StringVarP(&output, "output", "o", "", "输出文件（缺省输出到 stdout）")
	return command
}
