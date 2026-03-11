package client

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func loadContractFixture(t *testing.T, name string) []byte {
	t.Helper()

	path := filepath.Join("..", "..", "test", "contracts", name)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}

	return data
}

func TestContractRunnerMe(t *testing.T) {
	var identity Identity
	if err := json.Unmarshal(loadContractFixture(t, "runner_me.json"), &identity); err != nil {
		t.Fatalf("decode identity fixture: %v", err)
	}

	if identity.CredentialName == "" {
		t.Fatal("contract violation: credentialName is required")
	}

	if identity.OrganizationID == "" {
		t.Fatal("contract violation: organizationId is required")
	}
}

func TestContractRunnerConfig(t *testing.T) {
	var cfg RunnerConfigResponse
	if err := json.Unmarshal(loadContractFixture(t, "runner_config.json"), &cfg); err != nil {
		t.Fatalf("decode runner config fixture: %v", err)
	}

	if cfg.ConfigVersion == "" {
		t.Fatal("contract violation: configVersion is required")
	}

	if cfg.OrganizationID == "" {
		t.Fatal("contract violation: organizationId is required")
	}

	if cfg.RefreshAfterSeconds <= 0 {
		t.Fatal("contract violation: refreshAfterSeconds must be > 0")
	}
}

func TestContractJobsClaim(t *testing.T) {
	var claim JobClaimResponse
	if err := json.Unmarshal(loadContractFixture(t, "jobs_claim.json"), &claim); err != nil {
		t.Fatalf("decode jobs claim fixture: %v", err)
	}

	if claim.Job.ID == "" {
		t.Fatal("contract violation: job.id is required")
	}

	if claim.Job.OrganizationID == "" {
		t.Fatal("contract violation: job.organizationId is required")
	}

	if claim.Job.CeType == "" {
		t.Fatal("contract violation: job.ceType is required")
	}
}

func TestContractWorkersRegister(t *testing.T) {
	var resp RegisterWorkerResponse
	if err := json.Unmarshal(loadContractFixture(t, "workers_register.json"), &resp); err != nil {
		t.Fatalf("decode workers register fixture: %v", err)
	}

	if resp.WorkerID == "" {
		t.Fatal("contract violation: workerId is required")
	}

	if resp.RunnerID == "" {
		t.Fatal("contract violation: runnerId is required")
	}

	if resp.HeartbeatIntervalMs <= 0 {
		t.Fatal("contract violation: heartbeatIntervalMs must be > 0")
	}
}
