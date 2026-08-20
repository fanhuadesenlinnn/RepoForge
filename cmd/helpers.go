package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

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

// isTerminal reports whether w is an interactive character device (a real
// terminal), as opposed to a pipe, file, or CI log. Only terminals get the
// live progress bar; redirected output keeps one plain line per file.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

const (
	barWidth     = 20
	nameMaxWidth = 38
	lineMinWidth = 80
)

// renderDownloadBar draws one self-refreshing progress line with \r, covering
// the previous line. A final newline is emitted once the batch is complete so
// following output starts on a fresh line.
func renderDownloadBar(out io.Writer, d progress.Download) {
	pct := 0
	if d.Total > 0 {
		pct = d.Done * 100 / d.Total
	}
	filled := d.Done * barWidth / d.Total
	if d.Total > 0 && filled > barWidth {
		filled = barWidth
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

	name := d.Name
	if len(name) > nameMaxWidth {
		name = name[:nameMaxWidth-1] + "…"
	}
	status := "下载中"
	if d.Status != "" {
		status = d.Status
	}

	line := fmt.Sprintf("\r[下载] %s %3d%%  %d/%d  %s: %s",
		bar, pct, d.Done, d.Total, status, name)
	if len(line) < lineMinWidth {
		line += strings.Repeat(" ", lineMinWidth-len(line))
	}
	fmt.Fprint(out, line)
	if d.Done >= d.Total {
		fmt.Fprintln(out)
	}
}

// withProgress wires progress reporting onto the command context. Plain
// progress lines always print; download events render as a live progress bar
// on terminals and as one line per file otherwise.
func withProgress(command *cobra.Command) context.Context {
	out := command.OutOrStdout()
	tty := isTerminal(out)

	ctx := progress.With(command.Context(), func(format string, args ...any) {
		fmt.Fprintf(out, format+"\n", args...)
	})
	return progress.WithDownload(ctx, func(d progress.Download) {
		if tty {
			renderDownloadBar(out, d)
		} else {
			fmt.Fprintf(out, "[下载] %d/%d  %s  %s\n", d.Done, d.Total, d.Status, d.Name)
		}
	})
}
