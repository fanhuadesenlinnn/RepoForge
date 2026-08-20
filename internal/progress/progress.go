// Package progress carries optional human-readable progress over context.
// A nil reporter is silent, so tests stay quiet.
package progress

import "context"

// Func prints one progress line. format is a fmt-style string without a
// trailing newline; the caller that installs the reporter usually adds one.
type Func func(format string, args ...any)

type key struct{}

// With attaches a reporter to ctx.
func With(ctx context.Context, fn Func) context.Context {
	if fn == nil {
		return ctx
	}
	return context.WithValue(ctx, key{}, fn)
}

// Warnf reports a warning progress line when a reporter is attached.
func Warnf(ctx context.Context, format string, args ...any) {
	fn, _ := ctx.Value(key{}).(Func)
	if fn != nil {
		fn("[警告] "+format, args...)
	}
}

// Infof reports a progress line when a reporter is attached.
func Infof(ctx context.Context, format string, args ...any) {
	fn, _ := ctx.Value(key{}).(Func)
	if fn != nil {
		fn(format, args...)
	}
}

// Download reports the completion (or failure) of one file in a batch.
// The receiver decides how to render it: a live progress bar on a terminal,
// or one plain line per file when output is redirected.
type Download struct {
	Done   int    // files finished so far (including this one)
	Total  int    // total files in the batch
	Status string // 完成 / 失败
	Name   string // file base name
}

type downloadKey struct{}

// DownloadFunc renders one Download event.
type DownloadFunc func(Download)

// WithDownload attaches a download-progress receiver to ctx.
func WithDownload(ctx context.Context, fn DownloadFunc) context.Context {
	if fn == nil {
		return ctx
	}
	return context.WithValue(ctx, downloadKey{}, fn)
}

// ReportDownload forwards a Download event when a receiver is attached.
func ReportDownload(ctx context.Context, d Download) {
	fn, _ := ctx.Value(downloadKey{}).(DownloadFunc)
	if fn != nil {
		fn(d)
	}
}
