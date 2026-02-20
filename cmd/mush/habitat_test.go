package main

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/musher-dev/mush/internal/client"
	"github.com/musher-dev/mush/internal/testutil"
)

func habitatMockClient(t *testing.T, habitats []client.HabitatSummary) *client.Client {
	t.Helper()

	payload, err := json.Marshal(habitats)
	if err != nil {
		t.Fatalf("marshal habitats: %v", err)
	}

	hc := &http.Client{Transport: linkRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/api/v1/runner/habitats" && r.Method == http.MethodGet {
			return linkJSONResponse(http.StatusOK, string(payload)), nil
		}

		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)

		return nil, nil //nolint:nilnil // unreachable after t.Fatalf
	})}

	return client.NewWithHTTPClient("https://api.test", "test-key", hc)
}

func TestHabitatList_Golden(t *testing.T) {
	out, buf := testWriter()

	withMockAPIClient(t, habitatMockClient(t, []client.HabitatSummary{
		{ID: "h1", Slug: "prod-habitat", Name: "Production", Status: "active", HabitatType: "cloud"},
		{ID: "h2", Slug: "staging", Name: "Staging Environment", Status: "active", HabitatType: "cloud"},
		{ID: "h3", Slug: "dev-local", Name: "Local Development", Status: "inactive", HabitatType: "local"},
	}))

	cmd := newHabitatListCmd()
	cmd.SetArgs([]string{})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetContext(out.WithContext(t.Context()))

	if err := cmd.Execute(); err != nil {
		t.Fatalf("habitat list should succeed: %v", err)
	}

	testutil.AssertGolden(t, buf.String(), "habitat_list.golden")
}

func TestHabitatList_Truncation_Golden(t *testing.T) {
	out, buf := testWriter()

	withMockAPIClient(t, habitatMockClient(t, []client.HabitatSummary{
		{ID: "h1", Slug: "my-habitat", Name: "This Is A Very Long Habitat Name That Should Be Truncated", Status: "active", HabitatType: "cloud"},
	}))

	cmd := newHabitatListCmd()
	cmd.SetArgs([]string{})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetContext(out.WithContext(t.Context()))

	if err := cmd.Execute(); err != nil {
		t.Fatalf("habitat list should succeed: %v", err)
	}

	testutil.AssertGolden(t, buf.String(), "habitat_list_truncated.golden")
}
