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

// Infof reports a progress line when a reporter is attached.
func Infof(ctx context.Context, format string, args ...any) {
	fn, _ := ctx.Value(key{}).(Func)
	if fn != nil {
		fn(format, args...)
	}
}
