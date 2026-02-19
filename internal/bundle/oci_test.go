package bundle

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestMaterializeFromMetadata(t *testing.T) {
	workDir := t.TempDir()

	content := []byte("hello from oci")
	hash := sha256.Sum256(content)
	sha := hex.EncodeToString(hash[:])

	cfg := &ociConfig{
		Assets: []ociConfigAsset{
			{
				LogicalPath:   "skills/web/SKILL.md",
				AssetType:     "skill",
				ContentSHA256: sha,
			},
		},
	}

	manifest, err := materializeFromMetadata(cfg, map[string][]byte{sha: content}, workDir)
	if err != nil {
		t.Fatalf("materializeFromMetadata() error = %v", err)
	}

	if manifest == nil || len(manifest.Layers) != 1 {
		t.Fatalf("materializeFromMetadata() layers = %#v, want 1 layer", manifest)
	}

	gotPath := filepath.Join(workDir, "skills", "web", "SKILL.md")

	gotData, err := os.ReadFile(gotPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", gotPath, err)
	}

	if !bytes.Equal(gotData, content) {
		t.Fatalf("materialized file content = %q, want %q", string(gotData), string(content))
	}
}

func TestMaterializeFromMetadataRejectsTraversal(t *testing.T) {
	workDir := t.TempDir()

	cfg := &ociConfig{
		Assets: []ociConfigAsset{
			{
				LogicalPath:   "../escape.txt",
				AssetType:     "skill",
				ContentSHA256: "abc",
			},
		},
	}

	_, err := materializeFromMetadata(cfg, map[string][]byte{"abc": []byte("x")}, workDir)
	if err == nil {
		t.Fatal("materializeFromMetadata() expected error, got nil")
	}
}
