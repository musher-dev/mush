package update

import "testing"

func TestDetectInstallSource(t *testing.T) {
	tests := []struct {
		name string
		path string
		want InstallSource
	}{
		{
			name: "homebrew cellar path",
			path: "/opt/homebrew/Cellar/mush/1.2.3/bin/mush",
			want: InstallSourceHomebrew,
		},
		{
			name: "standalone path",
			path: "/usr/local/bin/mush",
			want: InstallSourceStandalone,
		},
		{
			name: "empty path",
			path: "",
			want: InstallSourceUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DetectInstallSource(tt.path); got != tt.want {
				t.Errorf("DetectInstallSource(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestUpgradeHint(t *testing.T) {
	if got := UpgradeHint(InstallSourceHomebrew); got != "brew upgrade mush" {
		t.Errorf("UpgradeHint(homebrew) = %q", got)
	}

	if got := UpgradeHint(InstallSourceStandalone); got != "" {
		t.Errorf("UpgradeHint(standalone) = %q, want empty", got)
	}
}
