package engine

import (
	"io"
	"time"
)

// readIdleLimit is how long (seconds) we allow with no bytes received before
// treating the download as hung. Slow-but-moving downloads keep flowing and are
// unaffected; a dead connection will be aborted so the run doesn't hang forever.
const readIdleLimit = 60 * time.Second

// idleReader fails if no read yields data for readIdleLimit.
type idleReader struct {
	r io.Reader
}

func (ir idleReader) Read(p []byte) (int, error) {
	type readResult struct {
		n   int
		err error
	}
	ch := make(chan readResult, 1)
	go func() {
		n, err := ir.r.Read(p)
		ch <- readResult{n, err}
	}()
	t := time.NewTimer(readIdleLimit)
	defer t.Stop()
	select {
	case res := <-ch:
		return res.n, res.err
	case <-t.C:
		return 0, io.ErrNoProgress
	}
}
