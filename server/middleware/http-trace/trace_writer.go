package trace

import (
	"bytes"
	"context"
	"net/http"
	"time"

	"aihot-server/common/handlers/log"

	legoTrace "git.xiaojukeji.com/lego/context-go"
)

type basicWriter interface {
	http.ResponseWriter
}

// traceWriter wraps a http.ResponseWriter that implements the minimal
// http.ResponseWriter interface.
type traceWriter struct {
	basicWriter
	rec     *bytes.Buffer
	ctx     context.Context
	log     log.CommonLog
	code    int
	lastTM  time.Time
	onFlush func(w *traceWriter)
}

// Write writes the data to the connection as part of an HTTP reply.
func (b *traceWriter) Write(buf []byte) (int, error) {
	n, err := b.rec.Write(buf)
	if err != nil {
		if b.log != nil {
			b.log.Errorf(b.ctx, legoTrace.DLTagUndefined, "write buffer error || errormsg=%s", err.Error())
		}
		return n, err
	}
	n, err = b.basicWriter.Write(buf)
	return n, err
}

// WriteHeader writes the response header.
func (b *traceWriter) WriteHeader(code int) {
	b.basicWriter.WriteHeader(code)
	b.code = code
}

// Flush implements http.Flusher.
func (b *traceWriter) Flush() {
	if h, ok := b.basicWriter.(http.Flusher); ok {
		h.Flush()
		if b.onFlush != nil {
			b.onFlush(b)
			b.rec.Reset()
			b.lastTM = time.Now()
		}
	}
}
