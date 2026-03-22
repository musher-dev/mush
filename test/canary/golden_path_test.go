package canary

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/musher-dev/mush/internal/client"
)

func TestGoldenPathCanary(t *testing.T) {
	baseURL := os.Getenv("MUSH_CANARY_API_URL")
	apiKey := os.Getenv("MUSH_CANARY_API_KEY")
	habitatID := os.Getenv("MUSH_CANARY_HABITAT_ID")
	queueID := os.Getenv("MUSH_CANARY_QUEUE_ID")

	if baseURL == "" || apiKey == "" || habitatID == "" || queueID == "" {
		t.Skip("canary disabled; set MUSH_CANARY_API_URL, MUSH_CANARY_API_KEY, MUSH_CANARY_HABITAT_ID, and MUSH_CANARY_QUEUE_ID")
	}

	httpClient, err := client.NewInstrumentedHTTPClient(os.Getenv("MUSHER_NETWORK_CA_CERT_FILE"))
	if err != nil {
		t.Fatalf("build HTTP client: %v", err)
	}

	c := client.NewWithHTTPClient(baseURL, apiKey, httpClient)

	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	if _, validateErr := c.ValidateKey(ctx); validateErr != nil {
		t.Fatalf("validate key: %v", validateErr)
	}

	habitats, err := c.ListHabitats(ctx)
	if err != nil {
		t.Fatalf("list habitats: %v", err)
	}

	foundHabitat := false

	for _, h := range habitats {
		if h.ID == habitatID {
			foundHabitat = true
			break
		}
	}

	if !foundHabitat {
		t.Fatalf("configured habitat %q not found", habitatID)
	}

	availability, err := c.GetQueueInstructionAvailability(ctx, queueID)
	if err != nil {
		t.Fatalf("queue instruction availability: %v", err)
	}

	if availability == nil || !availability.HasActiveInstruction {
		t.Fatalf("queue %q has no active instruction", queueID)
	}

	job, claimed, err := c.ClaimJob(ctx, habitatID, queueID, 1)
	if err != nil {
		t.Fatalf("claim job: %v", err)
	}

	if claimed && job != nil {
		t.Logf("canary claimed live job id=%s; releasing", job.ID)

		if releaseErr := c.ReleaseJob(ctx, job.ID); releaseErr != nil {
			t.Fatalf("release claimed job: %v", releaseErr)
		}
	}
}

func TestCanaryHealthProbeBudget(t *testing.T) {
	baseURL := os.Getenv("MUSH_CANARY_API_URL")
	if baseURL == "" {
		t.Skip("canary disabled; set MUSH_CANARY_API_URL")
	}

	start := time.Now()

	result := client.ProbeHealth(t.Context(), baseURL)
	if !result.Reachable {
		t.Fatalf("health probe unreachable: %s", result.Error)
	}

	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("health probe exceeded budget: %s", elapsed)
	}

	if result.StatusCode == http.StatusTooManyRequests {
		t.Fatalf("health probe is rate-limited (429)")
	}
}
