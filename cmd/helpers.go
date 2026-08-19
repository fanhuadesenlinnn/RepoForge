package cmd

import (
	"context"
	"fmt"

	"github.com/fanhuadesenlinnn/RepoForge/internal/home"
	"github.com/fanhuadesenlinnn/RepoForge/internal/progress"
	"github.com/fanhuadesenlinnn/RepoForge/internal/repo"
	"github.com/spf13/cobra"
)

func loadRepo() (string, *repo.Config, error) {
	homeDir, err := home.Detect(false)
	if err != nil {
		return "", nil, err
	}
	cfg, err := repo.Load(homeDir)
	if err != nil {
		return "", nil, err
	}
	return homeDir, cfg, nil
}

func withProgress(command *cobra.Command) context.Context {
	return progress.With(command.Context(), func(format string, args ...any) {
		fmt.Fprintf(command.OutOrStdout(), format+"\n", args...)
	})
}
