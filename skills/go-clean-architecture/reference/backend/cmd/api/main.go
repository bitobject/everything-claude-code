// Command api is the composition root of the HTTP API: it reads configuration, builds
// infrastructure, wires repositories into use cases and use cases into HTTP adapters, and runs
// the server. `api migrate` applies the database migrations as the schema owner. It is the only
// place that knows every concrete type; nothing here contains business rules.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"example.com/app/internal/notifications"
	"example.com/app/internal/platform/config"
	"example.com/app/internal/platform/events"
	"example.com/app/internal/platform/httpserver"
	"example.com/app/internal/platform/logging"
	"example.com/app/internal/platform/postgres"
	"example.com/app/migrations"
)

// The DSNs are unset from the environment once read: child processes don't inherit them (the
// process's own /proc/<pid>/environ still shows what it started with).
type cfg struct {
	Addr           string        `env:"HTTP_ADDR" envDefault:":8080"`
	DatabaseURL    config.Secret `env:"DATABASE_URL,unset"`         // the application role (serve)
	MigrateURL     config.Secret `env:"MIGRATE_DATABASE_URL,unset"` // the schema owner (migrate)
	HandlerTimeout time.Duration `env:"HTTP_HANDLER_TIMEOUT" envDefault:"10s"`
	DrainDelay     time.Duration `env:"SHUTDOWN_DRAIN_DELAY" envDefault:"5s"`
	Grace          time.Duration `env:"SHUTDOWN_GRACE" envDefault:"15s"`
	LogLevel       slog.Level    `env:"LOG_LEVEL" envDefault:"INFO"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("exit", "err", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var c cfg
	if err := env.Parse(&c); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	log := logging.New(os.Stdout, c.LogLevel)

	switch cmd := subcommand(args); cmd {
	case "migrate":
		if c.MigrateURL == "" {
			return errors.New("config: MIGRATE_DATABASE_URL is required for migrate")
		}
		return postgres.Migrate(ctx, c.MigrateURL.Reveal(), migrations.FS, log)
	case "serve":
		if c.DatabaseURL == "" {
			return errors.New("config: DATABASE_URL is required")
		}
		return serve(ctx, log, c)
	default:
		return fmt.Errorf("unknown command %q (want serve or migrate)", cmd)
	}
}

func subcommand(args []string) string {
	if len(args) == 0 {
		return "serve"
	}
	return args[0]
}

func serve(ctx context.Context, log *slog.Logger, c cfg) error {
	pool, err := postgres.Open(ctx, c.DatabaseURL.Reveal(), postgres.DefaultConfig("api"))
	if err != nil {
		return err
	}
	defer pool.Close()

	// Insert-only River client: no queues, never started. The worker process works the jobs.
	jobsClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Schema: "river", Logger: log})
	if err != nil {
		return fmt.Errorf("river client: %w", err)
	}
	bus := events.NewBus(jobsClient)
	notifications.Subscribe(bus) // every subscriber module, in every process that publishes

	v1, err := newAPI(log, pool, bus)
	if err != nil {
		return err
	}

	health := httpserver.NewHealth(pool.Ping)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", health.Live) // probes skip the middleware: no access log lines
	mux.HandleFunc("GET /readyz", health.Ready)
	mux.Handle("/v1/", httpserver.Chain(v1,
		httpserver.RequestID,      // first, so every log line of the request has the ID
		httpserver.AccessLog(log), // one line per request, with the final status
		httpserver.Recover(log),   // a panic becomes a logged 500
		httpserver.Timeout(c.HandlerTimeout),
		httpserver.LimitBody,
		http.NewCrossOriginProtection().Handler, // cookie-authenticated mutations from other sites
	))

	log.InfoContext(ctx, "listening", "addr", c.Addr)
	err = httpserver.Run(ctx, httpserver.New(c.Addr, mux, log), httpserver.Shutdown{
		Health: health, DrainDelay: c.DrainDelay, Grace: c.Grace,
	})
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
