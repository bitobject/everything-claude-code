// Package postgres opens connection pools and runs migrations. It is the only package that knows
// DSNs and session settings; repositories receive the pool.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Config holds the pool and session settings of the application role.
type Config struct {
	AppName  string // application_name, shown in pg_stat_activity
	MaxConns int32  // per process; processes × MaxConns must stay below the server's max_connections
	// Server-side limits, sent at connect. They protect the database when a caller forgets a
	// deadline; the request deadline (httpserver.Timeout) is the client-side one.
	StatementTimeout       time.Duration // one statement
	TransactionTimeout     time.Duration // a whole transaction (PostgreSQL 17+)
	IdleInTransactionLimit time.Duration // a transaction left open without work
	// AfterConnect runs on every new connection (tests use it to SET ROLE).
	AfterConnect func(context.Context, *pgx.Conn) error
}

// DefaultConfig is a starting point for the API and the worker.
func DefaultConfig(appName string) Config {
	return Config{
		AppName:                appName,
		MaxConns:               10,
		StatementTimeout:       5 * time.Second,
		TransactionTimeout:     10 * time.Second,
		IdleInTransactionLimit: 30 * time.Second,
	}
}

// Open creates a pool whose sessions use UTC and the limits in c, and checks that the server
// answers.
func Open(ctx context.Context, dsn string, c Config) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		// The parse error may echo the DSN, password included: don't wrap it.
		return nil, errors.New("parse database config: invalid DSN")
	}
	cfg.MaxConns = c.MaxConns
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnLifetimeJitter = 5 * time.Minute // avoid reconnect storms
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.HealthCheckPeriod = 30 * time.Second
	cfg.ConnConfig.ConnectTimeout = 5 * time.Second
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		// pgx returns timestamptz in time.Local whatever the session's time zone is; scanning
		// it as UTC here makes every time that leaves an adapter UTC, row mappings and views
		// alike, without a .UTC() at each call site.
		conn.TypeMap().RegisterType(&pgtype.Type{
			Name: "timestamptz", OID: pgtype.TimestamptzOID,
			Codec: &pgtype.TimestamptzCodec{ScanLocation: time.UTC},
		})
		if c.AfterConnect != nil {
			return c.AfterConnect(ctx, conn)
		}
		return nil
	}
	rp := cfg.ConnConfig.RuntimeParams
	rp["application_name"] = c.AppName
	rp["timezone"] = "UTC"
	rp["statement_timeout"] = ms(c.StatementTimeout)
	rp["transaction_timeout"] = ms(c.TransactionTimeout)
	rp["idle_in_transaction_session_timeout"] = ms(c.IdleInTransactionLimit)

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

// ms formats d as PostgreSQL milliseconds; zero means no limit.
func ms(d time.Duration) string { return strconv.FormatInt(d.Milliseconds(), 10) }
