package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/musher-dev/mush/internal/client"
	clierrors "github.com/musher-dev/mush/internal/errors"
	"github.com/musher-dev/mush/internal/output"
	"github.com/musher-dev/mush/internal/terminal"
)

func TestResolveQueue_WithFlag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/queues" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"q-1","slug":"default","name":"Default","status":"active","habitatId":"hab-1"}]}`))
	}))
	defer server.Close()

	c := client.New("test-key").WithBaseURL(server.URL)
	out := output.NewWriter(io.Discard, io.Discard, &terminal.Info{})

	queue, err := resolveQueue(context.Background(), c, "hab-1", "q-1", out)
	if err != nil {
		t.Fatalf("resolveQueue returned error: %v", err)
	}
	if queue.ID != "q-1" {
		t.Fatalf("queue.ID = %q, want q-1", queue.ID)
	}
}

func TestResolveQueue_WithInvalidFlag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"q-1","slug":"default","name":"Default","status":"active","habitatId":"hab-1"}]}`))
	}))
	defer server.Close()

	c := client.New("test-key").WithBaseURL(server.URL)
	out := output.NewWriter(io.Discard, io.Discard, &terminal.Info{})

	_, err := resolveQueue(context.Background(), c, "hab-1", "q-missing", out)
	if err == nil {
		t.Fatal("expected error for missing queue flag")
	}
}

func TestResolveQueue_NoQueuesForHabitat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	c := client.New("test-key").WithBaseURL(server.URL)
	out := output.NewWriter(io.Discard, io.Discard, &terminal.Info{})

	_, err := resolveQueue(context.Background(), c, "hab-1", "", out)
	if err == nil {
		t.Fatal("expected no queues error")
	}
}

func TestResolveQueue_AutoSelectSingle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"q-1","slug":"default","name":"Default","status":"active","habitatId":"hab-1"}]}`))
	}))
	defer server.Close()

	c := client.New("test-key").WithBaseURL(server.URL)
	out := output.NewWriter(io.Discard, io.Discard, &terminal.Info{})
	out.NoInput = true

	queue, err := resolveQueue(context.Background(), c, "hab-1", "", out)
	if err != nil {
		t.Fatalf("resolveQueue returned error: %v", err)
	}
	if queue.ID != "q-1" {
		t.Fatalf("queue.ID = %q, want q-1", queue.ID)
	}
}

func TestResolveQueue_NoInputMultipleQueues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[` +
			`{"id":"q-1","slug":"default","name":"Default","status":"active","habitatId":"hab-1"},` +
			`{"id":"q-2","slug":"other","name":"Other","status":"active","habitatId":"hab-1"}` +
			`]}`))
	}))
	defer server.Close()

	c := client.New("test-key").WithBaseURL(server.URL)
	out := output.NewWriter(io.Discard, io.Discard, &terminal.Info{})
	out.NoInput = true

	_, err := resolveQueue(context.Background(), c, "hab-1", "", out)
	if err == nil {
		t.Fatal("expected error for multiple queues in no-input mode")
	}
	var cliErr *clierrors.CLIError
	if !clierrors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T", err)
	}
	if cliErr.Message != "Queue required" {
		t.Fatalf("error message = %q, want %q", cliErr.Message, "Queue required")
	}
}

func TestResolveHabitatID_AutoSelectSingle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"id":"hab-1","slug":"local","name":"Local","status":"online","habitatType":"local"}]`))
	}))
	defer server.Close()

	c := client.New("test-key").WithBaseURL(server.URL)
	out := output.NewWriter(io.Discard, io.Discard, &terminal.Info{})
	out.NoInput = true

	id, err := resolveHabitatID(context.Background(), c, "", out)
	if err != nil {
		t.Fatalf("resolveHabitatID returned error: %v", err)
	}
	if id != "hab-1" {
		t.Fatalf("habitat ID = %q, want hab-1", id)
	}
}

func TestResolveHabitatID_NoInputMultipleHabitats(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[` +
			`{"id":"hab-1","slug":"local","name":"Local","status":"online","habitatType":"local"},` +
			`{"id":"hab-2","slug":"staging","name":"Staging","status":"offline","habitatType":"cloud"}` +
			`]`))
	}))
	defer server.Close()

	c := client.New("test-key").WithBaseURL(server.URL)
	out := output.NewWriter(io.Discard, io.Discard, &terminal.Info{})
	out.NoInput = true

	_, err := resolveHabitatID(context.Background(), c, "", out)
	if err == nil {
		t.Fatal("expected error for multiple habitats in no-input mode")
	}
	var cliErr *clierrors.CLIError
	if !clierrors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T", err)
	}
	if cliErr.Message != "Habitat required" {
		t.Fatalf("error message = %q, want %q", cliErr.Message, "Habitat required")
	}
}
