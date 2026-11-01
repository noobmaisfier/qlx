package pgxexec

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/noobmaisfier/qlx"
)

// ConnectOptions configure Connect. The zero value is usable.
type ConnectOptions struct {
	// Mode chooses how statements reach the server (default CacheStatements).
	Mode PoolMode
	// Tracer, when set, observes every statement run through Client.DB.
	Tracer Tracer
	// AfterConnect runs on every new connection after the range codecs are
	// registered; generated code passes its batched type registration here.
	AfterConnect func(ctx context.Context, c *pgx.Conn) error
	// MinServer is the lowest server major version the application supports;
	// Connect fails if the server is older.
	MinServer int
	// Spec is the generated SchemaSpec; when set, Connect checks the server's
	// major version against its Targets and calls OnAhead if the server is
	// newer than every target (the code still works, but runs ahead of its
	// pinned ledger).
	Spec *qlx.SchemaSpec
	// Shapes overrides the process-wide shape cache for this client.
	Shapes *qlx.ShapeCache
	// OnAhead is called with the server major when it is ahead of Spec.Targets.
	OnAhead func(serverMajor int, targets []int)
}

// Client bundles a pool with the server's dialect, so every statement is
// rendered for the version actually running and a construct newer than the
// server fails with ErrUnsupportedByServer before any network call.
type Client struct {
	Pool    *pgxpool.Pool
	Dialect qlx.Dialect
	// Shapes caches the rendering of every statement shape this client runs
	// (default qlx.DefaultShapes). Its Stats show hits, misses and
	// evictions; a growing eviction count means a per-call shape.
	Shapes *qlx.ShapeCache
	db     Querier
}

// Connect opens a pool, registers the range and multirange codecs, applies
// the pool mode, and reads server_version_num once.
func Connect(ctx context.Context, dsn string, opt ConnectOptions) (*Client, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	opt.Mode.Configure(cfg.ConnConfig)
	user := opt.AfterConnect
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		if err := AfterConnect(ctx, c); err != nil {
			return err
		}
		if user != nil {
			return user(ctx, c)
		}
		return nil
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	d, err := DetectDialect(ctx, pool)
	if err != nil {
		pool.Close()
		return nil, err
	}
	if opt.MinServer > 0 && d.Version < opt.MinServer {
		pool.Close()
		return nil, fmt.Errorf("pgxexec: server is PostgreSQL %d, the application needs %d or newer", d.Version, opt.MinServer)
	}
	if opt.Spec != nil {
		ahead, err := opt.Spec.CheckServer(d.Version)
		if err != nil {
			pool.Close()
			return nil, err
		}
		if ahead && opt.OnAhead != nil {
			opt.OnAhead(d.Version, opt.Spec.Targets)
		}
	}
	shapes := opt.Shapes
	if shapes == nil {
		shapes = qlx.DefaultShapes
	}
	c := &Client{Pool: pool, Dialect: d, Shapes: shapes, db: pool}
	if opt.Tracer != nil {
		c.db = Traced(pool, opt.Tracer)
	}
	return c, nil
}

// DB is the Querier to pass to the executors; it carries the tracer.
func (c *Client) DB() Querier { return c.db }

// Render renders s for the server's version.
func (c *Client) Render(s qlx.Statement) (qlx.Stmt, error) { return qlx.Render(c.Dialect, s) }

// Close closes the pool.
func (c *Client) Close() { c.Pool.Close() }

// resolver turns a builder (or a rendered Stmt) into SQL and arguments through
// the shape cache.
func (c *Client) resolver(st qlx.Statement) resolver {
	return func(binds []qlx.Binding) (string, []any, error) {
		return c.Shapes.Resolve(c.Dialect, st, binds...)
	}
}
