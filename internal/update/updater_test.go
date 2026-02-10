package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	selfupdate "github.com/creativeprojects/go-selfupdate"
)

// mockGitHubRelease creates a JSON response matching GitHub's release API format.
// It includes an asset matching the current platform so DetectLatest considers it valid.
func mockGitHubRelease(tag string) string {
	assetName := fmt.Sprintf("mush_%s_%s_%s.tar.gz", tag, runtime.GOOS, runtime.GOARCH)
	release := map[string]any{
		"tag_name":   "v" + tag,
		"name":       "Mush v" + tag,
		"prerelease": false,
		"draft":      false,
		"body":       "Release notes for " + tag,
		"assets": []any{
			map[string]any{
				"id":                   1,
				"name":                 assetName,
				"browser_download_url": fmt.Sprintf("https://example.com/download/%s", assetName),
			},
		},
	}
	data, _ := json.Marshal(release)
	return string(data)
}

// mockGitHubReleaseEmpty creates a release with no matching assets (no platform match).
func mockGitHubReleaseEmpty(tag string) string {
	release := map[string]any{
		"tag_name":   "v" + tag,
		"name":       "Mush v" + tag,
		"prerelease": false,
		"draft":      false,
		"assets":     []any{},
		"body":       "Release notes for " + tag,
	}
	data, _ := json.Marshal(release)
	return string(data)
}

func newTestUpdater(t *testing.T, handler http.Handler) *Updater {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	source, err := selfupdate.NewGitHubSource(selfupdate.GitHubConfig{
		APIToken:          "",
		EnterpriseBaseURL: server.URL + "/",
	})
	if err != nil {
		t.Fatalf("create test source: %v", err)
	}

	updater, err := selfupdate.NewUpdater(selfupdate.Config{
		Source: source,
	})
	if err != nil {
		t.Fatalf("create test updater: %v", err)
	}

	return &Updater{updater: updater}
}

func TestCheckLatest_NewerAvailable(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, "[%s]", mockGitHubRelease("2.0.0"))
	})

	u := newTestUpdater(t, handler)
	info, err := u.CheckLatest(context.Background(), "1.0.0")
	if err != nil {
		t.Fatalf("CheckLatest returned error: %v", err)
	}

	if !info.UpdateAvailable {
		t.Error("expected UpdateAvailable to be true")
	}
	if info.LatestVersion != "2.0.0" {
		t.Errorf("LatestVersion: got %q, want %q", info.LatestVersion, "2.0.0")
	}
	if info.CurrentVersion != "1.0.0" {
		t.Errorf("CurrentVersion: got %q, want %q", info.CurrentVersion, "1.0.0")
	}
}

func TestCheckLatest_UpToDate(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, "[%s]", mockGitHubRelease("1.0.0"))
	})

	u := newTestUpdater(t, handler)
	info, err := u.CheckLatest(context.Background(), "1.0.0")
	if err != nil {
		t.Fatalf("CheckLatest returned error: %v", err)
	}

	if info.UpdateAvailable {
		t.Error("expected UpdateAvailable to be false")
	}
}

func TestCheckLatest_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	u := newTestUpdater(t, handler)
	_, err := u.CheckLatest(context.Background(), "1.0.0")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestCheckLatest_RateLimited(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"API rate limit exceeded"}`)
	})

	u := newTestUpdater(t, handler)
	_, err := u.CheckLatest(context.Background(), "1.0.0")
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
}

func TestCheckLatest_DevBuild(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, "[%s]", mockGitHubRelease("1.0.0"))
	})

	u := newTestUpdater(t, handler)
	info, err := u.CheckLatest(context.Background(), "dev")
	if err != nil {
		t.Fatalf("CheckLatest returned error: %v", err)
	}

	// "dev" can't be parsed as semver, so update should be available
	if !info.UpdateAvailable {
		t.Error("expected UpdateAvailable to be true for dev build")
	}
}

func TestCheckLatest_NoReleases(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "[]")
	})

	u := newTestUpdater(t, handler)
	info, err := u.CheckLatest(context.Background(), "1.0.0")
	if err != nil {
		t.Fatalf("CheckLatest returned error: %v", err)
	}

	if info.UpdateAvailable {
		t.Error("expected UpdateAvailable to be false when no releases found")
	}
	if info.LatestVersion != "1.0.0" {
		t.Errorf("LatestVersion should fall back to current: got %q", info.LatestVersion)
	}
}

func TestCheckLatest_NoMatchingAssets(t *testing.T) {
	// Release exists but has no assets for current platform
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, "[%s]", mockGitHubReleaseEmpty("2.0.0"))
	})

	u := newTestUpdater(t, handler)
	info, err := u.CheckLatest(context.Background(), "1.0.0")
	if err != nil {
		t.Fatalf("CheckLatest returned error: %v", err)
	}

	// No matching assets means the release isn't found for this platform
	if info.UpdateAvailable {
		t.Error("expected UpdateAvailable to be false when no matching assets")
	}
}
