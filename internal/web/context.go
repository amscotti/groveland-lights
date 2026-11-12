package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"
)

type ctxKey int

const (
	ctxRequestID ctxKey = iota
	ctxServer
)

func newRequestID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func withRequestIDCtx(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxRequestID, id)
}

func requestIDOf(ctx context.Context) string {
	if v, ok := ctx.Value(ctxRequestID).(string); ok {
		return v
	}
	return "-"
}

// withServer embeds the *Server in the request context for handlers.
func (s *Server) withServer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxServer, s)))
	})
}

func serverFrom(ctx context.Context) *Server {
	if s, ok := ctx.Value(ctxServer).(*Server); ok {
		return s
	}
	panic("no server in context")
}

// nowFunc is swappable for tests that need an injected clock.
var nowFunc = time.Now
