package observability_test

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/musher-dev/mush/internal/observability"
)

type testPropagator struct{}

func (testPropagator) Inject(context.Context, propagation.TextMapCarrier) {}

func (testPropagator) Extract(ctx context.Context, _ propagation.TextMapCarrier) context.Context {
	return ctx
}

func (testPropagator) Fields() []string { return nil }

type testErrorHandler struct{}

func (testErrorHandler) Handle(error) {}

func TestSetupTelemetry_Disabled(t *testing.T) {
	origTP := otel.GetTracerProvider()
	origPropagator := otel.GetTextMapPropagator()
	origErrorHandler := otel.GetErrorHandler()

	t.Cleanup(func() {
		otel.SetTracerProvider(origTP)
		otel.SetTextMapPropagator(origPropagator)
		otel.SetErrorHandler(origErrorHandler)
	})

	sentinelTP := sdktrace.NewTracerProvider()

	t.Cleanup(func() {
		_ = sentinelTP.Shutdown(t.Context())
	})

	sentinelPropagator := testPropagator{}
	sentinelErrorHandler := testErrorHandler{}

	otel.SetTracerProvider(sentinelTP)
	otel.SetTextMapPropagator(sentinelPropagator)
	otel.SetErrorHandler(sentinelErrorHandler)

	shutdown, err := observability.SetupTelemetry(t.Context(), &observability.TelemetryConfig{
		Enabled: false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := shutdown(t.Context()); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}

	if got := otel.GetTracerProvider(); got != sentinelTP {
		t.Fatalf("tracer provider changed when telemetry disabled")
	}

	if _, ok := otel.GetTextMapPropagator().(testPropagator); !ok {
		t.Fatalf("propagator changed when telemetry disabled")
	}

	if _, ok := otel.GetErrorHandler().(testErrorHandler); !ok {
		t.Fatalf("error handler changed when telemetry disabled")
	}
}

func TestSetupTelemetry_NilConfig(t *testing.T) {
	shutdown, err := observability.SetupTelemetry(t.Context(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := shutdown(t.Context()); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}
}

func TestSetupTelemetry_Enabled(t *testing.T) {
	origTP := otel.GetTracerProvider()
	origPropagator := otel.GetTextMapPropagator()
	origErrorHandler := otel.GetErrorHandler()

	t.Cleanup(func() {
		otel.SetTracerProvider(origTP)
		otel.SetTextMapPropagator(origPropagator)
		otel.SetErrorHandler(origErrorHandler)
	})

	sentinelTP := sdktrace.NewTracerProvider()

	t.Cleanup(func() {
		_ = sentinelTP.Shutdown(t.Context())
	})

	sentinelPropagator := testPropagator{}
	sentinelErrorHandler := testErrorHandler{}

	otel.SetTracerProvider(sentinelTP)
	otel.SetTextMapPropagator(sentinelPropagator)
	otel.SetErrorHandler(sentinelErrorHandler)

	shutdown, err := observability.SetupTelemetry(t.Context(), &observability.TelemetryConfig{
		Enabled:     true,
		Endpoint:    "localhost:4318",
		ServiceName: "mush-test",
		Version:     "0.0.1",
		Commit:      "abc123",
		Environment: "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// After setup, the global provider should NOT be a noop.
	tp := otel.GetTracerProvider()
	if _, isNoop := tp.(*noop.TracerProvider); isNoop {
		t.Fatal("expected real TracerProvider, got noop")
	}

	if tp == sentinelTP {
		t.Fatal("expected setup to replace tracer provider")
	}

	if err := shutdown(t.Context()); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}

	if got := otel.GetTracerProvider(); got != sentinelTP {
		t.Fatal("expected tracer provider to be restored after shutdown")
	}

	if _, ok := otel.GetTextMapPropagator().(testPropagator); !ok {
		t.Fatal("expected propagator to be restored after shutdown")
	}

	if _, ok := otel.GetErrorHandler().(testErrorHandler); !ok {
		t.Fatal("expected error handler to be restored after shutdown")
	}
}

func TestSetupTelemetry_ShutdownRestoresGlobalsOnCanceledContext(t *testing.T) {
	origTP := otel.GetTracerProvider()
	origPropagator := otel.GetTextMapPropagator()
	origErrorHandler := otel.GetErrorHandler()

	t.Cleanup(func() {
		otel.SetTracerProvider(origTP)
		otel.SetTextMapPropagator(origPropagator)
		otel.SetErrorHandler(origErrorHandler)
	})

	sentinelTP := sdktrace.NewTracerProvider()

	t.Cleanup(func() {
		_ = sentinelTP.Shutdown(t.Context())
	})

	sentinelPropagator := testPropagator{}
	sentinelErrorHandler := testErrorHandler{}

	otel.SetTracerProvider(sentinelTP)
	otel.SetTextMapPropagator(sentinelPropagator)
	otel.SetErrorHandler(sentinelErrorHandler)

	shutdown, err := observability.SetupTelemetry(t.Context(), &observability.TelemetryConfig{
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	canceledCtx, cancel := context.WithCancel(t.Context())
	cancel()

	_ = shutdown(canceledCtx)

	if got := otel.GetTracerProvider(); got != sentinelTP {
		t.Fatal("expected tracer provider to be restored even on shutdown error")
	}

	if _, ok := otel.GetTextMapPropagator().(testPropagator); !ok {
		t.Fatal("expected propagator to be restored even on shutdown error")
	}

	if _, ok := otel.GetErrorHandler().(testErrorHandler); !ok {
		t.Fatal("expected error handler to be restored even on shutdown error")
	}
}

func TestIsTelemetryEnabled(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		want     bool
	}{
		{"empty", "", false},
		{"true", "true", true},
		{"TRUE", "TRUE", true},
		{"1", "1", true},
		{"yes", "yes", true},
		{"false", "false", false},
		{"0", "0", false},
		{"no", "no", false},
		{"random", "random", false},
		{"whitespace true", "  true  ", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("OTEL_ENABLED", tt.envValue)

			got := observability.IsTelemetryEnabled()
			if got != tt.want {
				t.Errorf("IsTelemetryEnabled() = %v, want %v (env=%q)", got, tt.want, tt.envValue)
			}
		})
	}
}

func TestTracer_ReturnsNamedTracer(t *testing.T) {
	t.Parallel()

	tracer := observability.Tracer("mush.test")
	if tracer == nil {
		t.Fatal("expected non-nil tracer")
	}
}
