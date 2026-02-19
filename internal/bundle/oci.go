package bundle

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"github.com/musher-dev/mush/internal/client"
)

// PullOCI pulls a bundle from an OCI registry and extracts its layers.
func PullOCI(
	ctx context.Context,
	ociRef,
	ociDigest,
	destDir string,
) (*client.BundleManifest, error) {
	ref, err := name.ParseReference(ociRef)
	if err != nil {
		return nil, fmt.Errorf("parse OCI reference %q: %w", ociRef, err)
	}

	imageOptions := []remote.Option{
		remote.WithContext(ctx),
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
	}

	img, err := remote.Image(ref, imageOptions...)
	if err != nil {
		return nil, fmt.Errorf("pull OCI image %q: %w", ociRef, err)
	}

	// Verify manifest digest if provided.
	if ociDigest != "" {
		manifest, mErr := img.Manifest()
		if mErr != nil {
			return nil, fmt.Errorf("get OCI manifest: %w", mErr)
		}

		digest, dErr := img.Digest()
		if dErr != nil {
			return nil, fmt.Errorf("compute OCI digest: %w", dErr)
		}

		_ = manifest // used indirectly via digest

		if digest.String() != ociDigest {
			return nil, fmt.Errorf("OCI digest mismatch: got %s, want %s", digest.String(), ociDigest)
		}
	}

	configBytes, err := img.RawConfigFile()
	if err != nil {
		return nil, fmt.Errorf("read OCI config blob: %w", err)
	}

	metadata, err := decodeOCIConfig(configBytes)
	if err != nil {
		return nil, fmt.Errorf("decode OCI config blob: %w", err)
	}

	layers, err := img.Layers()
	if err != nil {
		return nil, fmt.Errorf("get OCI layers: %w", err)
	}

	contentsBySHA := make(map[string][]byte)

	for i, layer := range layers {
		if collectErr := collectLayerFiles(layer, i, contentsBySHA); collectErr != nil {
			return nil, fmt.Errorf("extract layer %d: %w", i, collectErr)
		}
	}

	manifest, err := materializeFromMetadata(metadata, contentsBySHA, destDir)
	if err != nil {
		return nil, err
	}

	return manifest, nil
}

func collectLayerFiles(layer v1.Layer, index int, bySHA map[string][]byte) (err error) {
	reader, err := layer.Uncompressed()
	if err != nil {
		return fmt.Errorf("uncompress layer: %w", err)
	}
	defer reader.Close()

	layerRoot := fmt.Sprintf("layer-%d", index)

	if err := collectTarContents(reader, layerRoot, bySHA); err != nil {
		return fmt.Errorf("extract tar layer: %w", err)
	}

	return nil
}

func collectTarContents(r io.Reader, layerRoot string, bySHA map[string][]byte) error {
	tarReader := tar.NewReader(r)

	for {
		hdr, err := tarReader.Next()
		if err == io.EOF {
			return nil
		}

		if err != nil {
			return fmt.Errorf("read tar entry: %w", err)
		}

		entryPath := filepath.Clean(hdr.Name)

		if entryPath == "." || entryPath == "" {
			continue
		}

		if strings.HasPrefix(entryPath, "..") || filepath.IsAbs(entryPath) {
			return fmt.Errorf("unsafe tar path in %s: %s", layerRoot, hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeReg:
			data, err := io.ReadAll(tarReader)
			if err != nil {
				return fmt.Errorf("read file %s: %w", hdr.Name, err)
			}

			hash := sha256.Sum256(data)
			hexSHA := hex.EncodeToString(hash[:])

			if _, exists := bySHA[hexSHA]; !exists {
				bySHA[hexSHA] = append([]byte(nil), data...)
			}
		default:
			// Skip unsupported entry types.
			continue
		}
	}
}

type ociConfig struct {
	Assets []ociConfigAsset `json:"assets"`
}

type ociConfigAsset struct {
	LogicalPath   string `json:"logical_path"`
	AssetType     string `json:"asset_type"`
	ContentSHA256 string `json:"content_sha256"`
}

func decodeOCIConfig(configBytes []byte) (*ociConfig, error) {
	trimmed := bytes.TrimSpace(configBytes)

	if len(trimmed) == 0 {
		return &ociConfig{}, nil
	}

	var cfg ociConfig
	if err := json.Unmarshal(trimmed, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal oci config: %w", err)
	}

	return &cfg, nil
}

func materializeFromMetadata(
	cfg *ociConfig,
	contentsBySHA map[string][]byte,
	destDir string,
) (*client.BundleManifest, error) {
	layers := make([]client.BundleLayer, 0, len(cfg.Assets))

	for _, asset := range cfg.Assets {
		if err := ValidateLogicalPath(asset.LogicalPath); err != nil {
			return nil, fmt.Errorf("invalid logical path from OCI config: %w", err)
		}

		content, ok := contentsBySHA[asset.ContentSHA256]
		if !ok {
			return nil, fmt.Errorf("missing OCI payload for asset %s (%s)", asset.LogicalPath, asset.ContentSHA256)
		}

		destPath := filepath.Join(destDir, asset.LogicalPath)
		destPathClean := filepath.Clean(destPath)
		destDirClean := filepath.Clean(destDir)

		if destPathClean != destDirClean && !strings.HasPrefix(destPathClean, destDirClean+string(filepath.Separator)) {
			return nil, fmt.Errorf("materialized path escapes destination: %s", asset.LogicalPath)
		}

		if err := os.MkdirAll(filepath.Dir(destPathClean), 0o755); err != nil { //nolint:gosec // G301: cache path
			return nil, fmt.Errorf("create dir for %s: %w", asset.LogicalPath, err)
		}

		if err := os.WriteFile(destPathClean, content, 0o644); err != nil { //nolint:gosec // G306: cache files are readable
			return nil, fmt.Errorf("write %s: %w", asset.LogicalPath, err)
		}

		layers = append(layers, client.BundleLayer{
			AssetID:       "",
			LogicalPath:   asset.LogicalPath,
			AssetType:     asset.AssetType,
			ContentSHA256: asset.ContentSHA256,
			SizeBytes:     int64(len(content)),
		})
	}

	sort.Slice(layers, func(i, j int) bool {
		return layers[i].LogicalPath < layers[j].LogicalPath
	})

	return &client.BundleManifest{Layers: layers}, nil
}

// verifySHA256 verifies that file content matches the expected SHA256 hex digest.
func verifySHA256(data []byte, expectedHex string) error {
	if expectedHex == "" {
		return nil
	}

	hash := sha256.Sum256(data)
	got := hex.EncodeToString(hash[:])

	if got != expectedHex {
		return fmt.Errorf("SHA256 mismatch: got %s, want %s", got, expectedHex)
	}

	return nil
}
