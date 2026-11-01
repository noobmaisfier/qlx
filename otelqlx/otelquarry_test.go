package otelqlx

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type pgErr struct{}

func (pgErr) Error() string {
	return `duplicate key value violates unique constraint: Key (email)=(a@b.c) already exists`
}
func (pgErr) SQLState() string { return "23505" }

func setup(t *testing.T, opts ...Option) (*Instrumentation, *tracetest.SpanRecorder, *metric.ManualReader) {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	rd := metric.NewManualReader()
	mp := metric.NewMeterProvider(metric.WithReader(rd))
	i, err := New(append([]Option{WithTracerProvider(tp), WithMeterProvider(mp)}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	return i, rec, rd
}

func TestSpanAndMetricForTaggedQuery(t *testing.T) {
	i, rec, rd := setup(t)
	i.Observe(context.Background(), `/* qlx: users.by_id */ SELECT "id" FROM "users" WHERE "id" = $1`, 25*time.Millisecond, nil)
	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("%d spans", len(spans))
	}
	s := spans[0]
	if s.Name() != "users.by_id" {
		t.Errorf("span name %q", s.Name())
	}
	if d := s.EndTime().Sub(s.StartTime()); d < 24*time.Millisecond || d > 26*time.Millisecond {
		t.Errorf("span duration %v, want ~25ms (back-dated start)", d)
	}
	got := map[string]string{}
	for _, a := range s.Attributes() {
		got[string(a.Key)] = a.Value.Emit()
	}
	for k, v := range map[string]string{"db.system.name": "postgresql", "db.operation.name": "SELECT", "db.query.summary": "users.by_id"} {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	if got["db.query.text"] == "" {
		t.Error("query text should be on the span by default")
	}
	var rm metricdata.ResourceMetrics
	if err := rd.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	if len(rm.ScopeMetrics) != 1 || rm.ScopeMetrics[0].Metrics[0].Name != "db.client.operation.duration" {
		t.Fatalf("metrics: %+v", rm)
	}
}

func TestErrorUsesSQLStateAndHidesMessage(t *testing.T) {
	i, rec, _ := setup(t)
	i.Observe(context.Background(), `INSERT INTO "users" ("email") VALUES ($1)`, time.Millisecond, pgErr{})
	s := rec.Ended()[0]
	if s.Status().Code.String() != "Error" || s.Status().Description != "23505" {
		t.Errorf("status %+v", s.Status())
	}
	if len(s.Events()) != 0 {
		t.Error("the error message (it can hold row values) must not be recorded by default")
	}
	got := map[string]string{}
	for _, a := range s.Attributes() {
		got[string(a.Key)] = a.Value.Emit()
	}
	if got["error.type"] != "23505" || got["db.response.status_code"] != "23505" {
		t.Errorf("attrs %v", got)
	}
}

func TestOptions(t *testing.T) {
	i, rec, _ := setup(t, WithQueryText(false), WithRecordErrors(true))
	i.Observe(context.Background(), `DELETE FROM "t"`, time.Millisecond, errors.New("boom"))
	s := rec.Ended()[0]
	for _, a := range s.Attributes() {
		if a.Key == "db.query.text" {
			t.Error("query text must be omitted")
		}
	}
	if len(s.Events()) != 1 {
		t.Errorf("want the error recorded, got %d events", len(s.Events()))
	}
	if s.Name() != "DELETE" {
		t.Errorf("untagged span name %q, want the verb", s.Name())
	}
}

func TestClassify(t *testing.T) {
	if et, _ := classify(context.Canceled); et != "context.Canceled" {
		t.Error(et)
	}
	if et, _ := classify(errors.New("x")); et != "_OTHER" {
		t.Error(et)
	}
}
