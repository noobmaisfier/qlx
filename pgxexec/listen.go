package pgxexec

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/noobmaisfier/qlx"
)

// Channel is a validated NOTIFY channel name. Build one with NewChannel so a
// bad name fails at startup, not on the first notification.
type Channel string

// NewChannel validates name with the same rules the renderer applies when it
// quotes the identifier.
func NewChannel(name string) (Channel, error) {
	if _, err := qlx.QuoteIdent(name); err != nil {
		return "", err
	}
	return Channel(name), nil
}

// ListenOptions tunes ListenChan. The zero value is usable.
type ListenOptions struct {
	// Buffer is the capacity of the returned channel (default 64). When the
	// consumer falls behind, the listener blocks; PostgreSQL queues the rest.
	Buffer int
	// BackoffBase and BackoffMax bound the wait between reconnect attempts
	// (default 100ms and 30s, doubling with jitter).
	BackoffBase, BackoffMax time.Duration
	// OnReconnect runs after each successful (re)subscription. Notifications
	// sent while the connection was down are lost, so use it to resynchronize
	// state; attempt is 0 for the first connection.
	OnReconnect func(attempt int)
	// OnError reports a lost connection before the wait.
	OnError func(err error)
}

// ListenChan subscribes to channels on a dedicated pooled connection and
// delivers notifications on the returned channel, reconnecting with backoff
// when the connection drops. The channel is closed when ctx ends; the
// connection is returned to the pool with UNLISTEN * so no subscription leaks
// into other users of the pool.
func ListenChan(ctx context.Context, pool *pgxpool.Pool, opt ListenOptions, channels ...Channel) <-chan Notification {
	if opt.Buffer <= 0 {
		opt.Buffer = 64
	}
	if opt.BackoffBase <= 0 {
		opt.BackoffBase = 100 * time.Millisecond
	}
	if opt.BackoffMax <= 0 {
		opt.BackoffMax = 30 * time.Second
	}
	out := make(chan Notification, opt.Buffer)
	go func() {
		defer close(out)
		wait := opt.BackoffBase
		for attempt := 0; ctx.Err() == nil; attempt++ {
			err := listenOnce(ctx, pool, opt, attempt, channels, out, func() { wait = opt.BackoffBase })
			if ctx.Err() != nil {
				return
			}
			if opt.OnError != nil && err != nil {
				opt.OnError(err)
			}
			sleep := wait + time.Duration(rand.Int64N(int64(wait)/2+1))
			select {
			case <-ctx.Done():
				return
			case <-time.After(sleep):
			}
			if wait *= 2; wait > opt.BackoffMax {
				wait = opt.BackoffMax
			}
		}
	}()
	return out
}

func listenOnce(ctx context.Context, pool *pgxpool.Pool, opt ListenOptions, attempt int, channels []Channel, out chan<- Notification, connected func()) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer func() {
		// Best effort: a broken connection is discarded by the pool anyway.
		cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		_, _ = conn.Exec(cctx, "UNLISTEN *")
		conn.Release()
	}()
	for _, ch := range channels {
		sql, _, err := qlx.SQL(qlx.Dialect{Version: qlx.PG18}, qlx.Listen(string(ch)))
		if err != nil {
			return err
		}
		if _, err := conn.Exec(ctx, sql); err != nil {
			return err
		}
	}
	connected()
	if opt.OnReconnect != nil {
		opt.OnReconnect(attempt)
	}
	for {
		n, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			return err
		}
		select {
		case out <- Notification{n.Channel, n.Payload, n.PID}:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
