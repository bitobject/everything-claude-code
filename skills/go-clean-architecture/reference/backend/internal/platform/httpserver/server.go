package httpserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"
)

// WriteTimeout bounds writing a response; the handler deadline (Timeout) must be shorter.
const WriteTimeout = 30 * time.Second

// New returns a server with timeouts set: the zero values mean "no timeout", which lets slow
// clients hold connections forever.
func New(addr string, h http.Handler, log *slog.Logger) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      WriteTimeout,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    1 << 20,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}
}

// Health serves the probes. /livez answers while the process runs; /readyz answers 503 once
// shutdown has started or when the database doesn't answer within a second, so the load
// balancer stops sending new requests before the server closes.
type Health struct {
	draining atomic.Bool
	ping     func(context.Context) error
}

// NewHealth returns probes that check readiness with ping (the database pool's Ping).
func NewHealth(ping func(context.Context) error) *Health { return &Health{ping: ping} }

// Live answers 204: the process is up.
func (h *Health) Live(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }

// Ready answers 204 when the server should get traffic, 503 otherwise.
func (h *Health) Ready(w http.ResponseWriter, r *http.Request) {
	if h.draining.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()
	if err := h.ping(ctx); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Shutdown is how Run stops: first fail readiness and keep serving for DrainDelay (the time
// the load balancer needs to notice), then stop accepting connections and wait up to Grace for
// requests in flight.
type Shutdown struct {
	Health     *Health
	DrainDelay time.Duration
	Grace      time.Duration
}

// Run serves until ctx is done, then shuts down as s says.
func Run(ctx context.Context, srv *http.Server, s Shutdown) error {
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()

	select {
	case err := <-errc:
		return fmt.Errorf("listen %s: %w", srv.Addr, err)
	case <-ctx.Done():
	}

	s.Health.draining.Store(true)
	select {
	case <-time.After(s.DrainDelay):
	case err := <-errc:
		return fmt.Errorf("listen %s: %w", srv.Addr, err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.Grace)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("listen %s: %w", srv.Addr, err)
	}
	return nil
}
