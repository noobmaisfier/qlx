// Package otelqlx reports every statement run through pgxexec.Observed as
// an OpenTelemetry client span and a duration histogram, following the stable
// database semantic conventions (db.system.name, db.operation.name,
// db.query.summary, db.query.text, db.response.status_code, error.type).
//
//	obs, err := otelqlx.New()
//	if err != nil { ... }
//	db := pgxexec.Observed(pool, obs.Observe)
//
// Statements are reported with $n placeholders and no arguments, so values
// never reach telemetry. The span and metric label is the name given with
// qlx.Tagged (or the SQL verb when untagged), never the SQL text, which
// keeps cardinality bounded. Error messages are not recorded by default
// because PostgreSQL puts row values in them ("Key (email)=(a@b.c) already
// exists"); the SQLSTATE is.
//
// This module is a separate go.mod so the core library and pgxexec carry no
// OpenTelemetry dependency. It was written without access to the OpenTelemetry
// modules and has not been compiled yet.
package otelqlx

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/noobmaisfier/qlx"
)

const instrumentationName = "github.com/noobmaisfier/qlx/otelqlx"

type config struct {
	tp           trace.TracerProvider
	mp           metric.MeterProvider
	queryText    bool
	recordErrors bool
}

// Option configures New.
type Option func(*config)

// WithTracerProvider uses tp instead of the global provider.
func WithTracerProvider(tp trace.TracerProvider) Option { return func(c *config) { c.tp = tp } }

// WithMeterProvider uses mp instead of the global provider.
func WithMeterProvider(mp metric.MeterProvider) Option { return func(c *config) { c.mp = mp } }

// WithQueryText controls whether db.query.text (the parameterized SQL) is put
// on spans. Default true. Turn it off when even query shapes are sensitive.
func WithQueryText(on bool) Option { return func(c *config) { c.queryText = on } }

// WithRecordErrors also records the error message as a span event. Off by
// default: PostgreSQL error messages can contain row values.
func WithRecordErrors(on bool) Option { return func(c *config) { c.recordErrors = on } }

// Instrumentation holds the tracer and the histogram.
type Instrumentation struct {
	cfg      config
	tracer   trace.Tracer
	duration metric.Float64Histogram
}

// New builds the instrumentation. It only fails if the metric instrument
// cannot be created.
func New(opts ...Option) (*Instrumentation, error) {
	cfg := config{queryText: true}
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.tp == nil {
		cfg.tp = otel.GetTracerProvider()
	}
	if cfg.mp == nil {
		cfg.mp = otel.GetMeterProvider()
	}
	h, err := cfg.mp.Meter(instrumentationName).Float64Histogram(
		"db.client.operation.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Duration of database client operations."),
		metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 10),
	)
	if err != nil {
		return nil, fmt.Errorf("otelqlx: %w", err)
	}
	return &Instrumentation{cfg: cfg, tracer: cfg.tp.Tracer(instrumentationName), duration: h}, nil
}

// Observe has the signature of pgxexec.Observer.
func (i *Instrumentation) Observe(ctx context.Context, sql string, took time.Duration, err error) {
	end := time.Now()
	info := qlx.DescribeSQL(sql)

	attrs := []attribute.KeyValue{attribute.String("db.system.name", "postgresql")}
	if info.Operation != "" {
		attrs = append(attrs, attribute.String("db.operation.name", info.Operation))
	}
	if info.Name != "" {
		attrs = append(attrs, attribute.String("db.query.summary", info.Name))
	}
	var errType, state string
	if err != nil {
		errType, state = classify(err)
		attrs = append(attrs, attribute.String("error.type", errType))
		if state != "" {
			attrs = append(attrs, attribute.String("db.response.status_code", state))
		}
	}

	spanAttrs := attrs
	if i.cfg.queryText {
		spanAttrs = append(append([]attribute.KeyValue(nil), attrs...), attribute.String("db.query.text", sql))
	}
	name := info.Label()
	if name == "" {
		name = "postgresql"
	}
	// The observer runs after the statement, so the span is back-dated.
	_, span := i.tracer.Start(ctx, name,
		trace.WithTimestamp(end.Add(-took)),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(spanAttrs...))
	if err != nil {
		if i.cfg.recordErrors {
			span.RecordError(err)
		}
		desc := state
		if desc == "" {
			desc = errType
		}
		span.SetStatus(codes.Error, desc)
	}
	span.End(trace.WithTimestamp(end))

	i.duration.Record(ctx, took.Seconds(), metric.WithAttributes(attrs...))
}

// sqlStater is implemented by *pgconn.PgError.
type sqlStater interface{ SQLState() string }

// classify returns error.type and the SQLSTATE when there is one. Context
// cancellation and deadlines get their own stable values; everything else is
// the opaque "_OTHER" the conventions prescribe for unknown errors.
func classify(err error) (errType, sqlstate string) {
	var ss sqlStater
	switch {
	case errors.As(err, &ss) && ss.SQLState() != "":
		return ss.SQLState(), ss.SQLState()
	case errors.Is(err, context.Canceled):
		return "context.Canceled", ""
	case errors.Is(err, context.DeadlineExceeded):
		return "context.DeadlineExceeded", ""
	}
	return "_OTHER", ""
}
