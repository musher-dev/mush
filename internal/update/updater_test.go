package update

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"testing"
	"time"

	selfupdate "github.com/creativeprojects/go-selfupdate"
)

type fakeSource struct {
	releases []selfupdate.SourceRelease
	err      error
}

func (f *fakeSource) ListReleases(context.Context, selfupdate.Repository) ([]selfupdate.SourceRelease, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.releases, nil
}

func (f *fakeSource) DownloadReleaseAsset(context.Context, *selfupdate.Release, int64) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

type fakeRelease struct {
	id         int64
	tag        string
	name       string
	url        string
	assets     []selfupdate.SourceAsset
	published  time.Time
	draft      bool
	prerelease bool
}

func (r *fakeRelease) GetID() int64                        { return r.id }
func (r *fakeRelease) GetTagName() string                  { return r.tag }
func (r *fakeRelease) GetDraft() bool                      { return r.draft }
func (r *fakeRelease) GetPrerelease() bool                 { return r.prerelease }
func (r *fakeRelease) GetPublishedAt() time.Time           { return r.published }
func (r *fakeRelease) GetReleaseNotes() string             { return "" }
func (r *fakeRelease) GetName() string                     { return r.name }
func (r *fakeRelease) GetURL() string                      { return r.url }
func (r *fakeRelease) GetAssets() []selfupdate.SourceAsset { return r.assets }

type fakeAsset struct {
	id   int64
	name string
	url  string
	size int
}

func (a *fakeAsset) GetID() int64                  { return a.id }
func (a *fakeAsset) GetName() string               { return a.name }
func (a *fakeAsset) GetSize() int                  { return a.size }
func (a *fakeAsset) GetBrowserDownloadURL() string { return a.url }

func newTestUpdater(t *testing.T, source selfupdate.Source) *Updater {
	t.Helper()
	updater, err := selfupdate.NewUpdater(selfupdate.Config{
		Source: source,
		OS:     runtime.GOOS,
		Arch:   runtime.GOARCH,
	})
	if err != nil {
		t.Fatalf("create test updater: %v", err)
	}
	return &Updater{updater: updater}
}

func testRelease(version string, withAsset bool) selfupdate.SourceRelease {
	rel := &fakeRelease{
		id:        1,
		tag:       "v" + version,
		name:      "Mush v" + version,
		url:       "https://example.com/releases/v" + version,
		published: time.Now().UTC(),
	}
	if withAsset {
		assetName := fmt.Sprintf("mush_%s_%s_%s.tar.gz", version, runtime.GOOS, runtime.GOARCH)
		rel.assets = []selfupdate.SourceAsset{
			&fakeAsset{id: 1, name: assetName, url: "https://example.com/download/" + assetName, size: 1},
		}
	}
	return rel
}

func TestCheckLatestNewerAvailable(t *testing.T) {
	u := newTestUpdater(t, &fakeSource{releases: []selfupdate.SourceRelease{testRelease("2.0.0", true)}})
	info, err := u.CheckLatest(t.Context(), "1.0.0")
	if err != nil {
		t.Fatalf("CheckLatest returned error: %v", err)
	}
	if !info.UpdateAvailable {
		t.Error("expected UpdateAvailable to be true")
	}
	if info.LatestVersion != "2.0.0" {
		t.Errorf("LatestVersion: got %q, want %q", info.LatestVersion, "2.0.0")
	}
}

func TestCheckLatestUpToDate(t *testing.T) {
	u := newTestUpdater(t, &fakeSource{releases: []selfupdate.SourceRelease{testRelease("1.0.0", true)}})
	info, err := u.CheckLatest(t.Context(), "1.0.0")
	if err != nil {
		t.Fatalf("CheckLatest returned error: %v", err)
	}
	if info.UpdateAvailable {
		t.Error("expected UpdateAvailable to be false")
	}
}

func TestCheckLatestAPIError(t *testing.T) {
	u := newTestUpdater(t, &fakeSource{err: errors.New("boom")})
	_, err := u.CheckLatest(t.Context(), "1.0.0")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCheckLatestDevBuild(t *testing.T) {
	u := newTestUpdater(t, &fakeSource{releases: []selfupdate.SourceRelease{testRelease("1.0.0", true)}})
	info, err := u.CheckLatest(t.Context(), "dev")
	if err != nil {
		t.Fatalf("CheckLatest returned error: %v", err)
	}
	if !info.UpdateAvailable {
		t.Error("expected UpdateAvailable to be true for dev build")
	}
}

func TestCheckLatestNoReleases(t *testing.T) {
	u := newTestUpdater(t, &fakeSource{releases: []selfupdate.SourceRelease{}})
	info, err := u.CheckLatest(t.Context(), "1.0.0")
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

func TestCheckLatestNoMatchingAssets(t *testing.T) {
	u := newTestUpdater(t, &fakeSource{releases: []selfupdate.SourceRelease{testRelease("2.0.0", false)}})
	info, err := u.CheckLatest(t.Context(), "1.0.0")
	if err != nil {
		t.Fatalf("CheckLatest returned error: %v", err)
	}
	if info.UpdateAvailable {
		t.Error("expected UpdateAvailable to be false when no matching assets")
	}
}
