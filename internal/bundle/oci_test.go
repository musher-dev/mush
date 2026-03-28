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

func TestVerifySHA256(t *testing.T) {
	t.Run("matching hash", func(t *testing.T) {
		data := []byte("hello world")
		hash := sha256.Sum256(data)
		expected := hex.EncodeToString(hash[:])

		got, err := VerifySHA256(data, expected)
		if err != nil {
			t.Fatalf("VerifySHA256() error = %v", err)
		}

		if !bytes.Equal(got, data) {
			t.Fatalf("VerifySHA256() returned %q, want %q", got, data)
		}
	})

	t.Run("empty expected hash", func(t *testing.T) {
		data := []byte("anything")

		got, err := VerifySHA256(data, "")
		if err != nil {
			t.Fatalf("VerifySHA256() error = %v", err)
		}

		if !bytes.Equal(got, data) {
			t.Fatalf("VerifySHA256() returned %q, want %q", got, data)
		}
	})

	t.Run("trailing newline recovery", func(t *testing.T) {
		// Simulate server stripping trailing newline: original has \n, API returns without.
		original := []byte("hello world\n")
		stripped := []byte("hello world")

		hash := sha256.Sum256(original)
		expected := hex.EncodeToString(hash[:])

		got, err := VerifySHA256(stripped, expected)
		if err != nil {
			t.Fatalf("VerifySHA256() error = %v, want recovery via trailing newline", err)
		}

		if !bytes.Equal(got, original) {
			t.Fatalf("VerifySHA256() returned %d bytes, want %d (with trailing newline)", len(got), len(original))
		}
	})

	t.Run("genuine mismatch", func(t *testing.T) {
		data := []byte("hello world")

		_, err := VerifySHA256(data, "0000000000000000000000000000000000000000000000000000000000000000")
		if err == nil {
			t.Fatal("VerifySHA256() expected error for genuine mismatch, got nil")
		}
	})
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
