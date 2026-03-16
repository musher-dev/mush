//go:build unix

package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/musher-dev/mush/internal/bundle"
	"github.com/musher-dev/mush/internal/client"
	clierrors "github.com/musher-dev/mush/internal/errors"
	"github.com/musher-dev/mush/internal/output"
)

type bundleSourceKind string

const (
	bundleSourceDir    bundleSourceKind = "dir"
	bundleSourceSample bundleSourceKind = "sample"
	bundleSourceRemote bundleSourceKind = "remote"
)

type bundleSourceOptions struct {
	refArg    string
	dirPath   string
	useSample bool
}

type bundleSourceResult struct {
	Kind      bundleSourceKind
	Resolved  *client.BundleResolveResponse
	CachePath string
	Ref       bundle.Ref
	Cleanup   func()
}

func resolveBundleSource(
	ctx context.Context,
	out *output.Writer,
	logger *slog.Logger,
	opts bundleSourceOptions,
) (*bundleSourceResult, error) {
	if opts.dirPath != "" {
		resolved, cachePath, cleanup, err := bundle.LoadFromDir(opts.dirPath)
		if err != nil {
			return nil, clierrors.Wrap(clierrors.ExitGeneral, "Failed to load bundle from directory", err)
		}

		logger.Info("bundle loaded from local directory", slog.String("bundle.dir", opts.dirPath))

		return &bundleSourceResult{
			Kind:      bundleSourceDir,
			Resolved:  resolved,
			CachePath: cachePath,
			Ref:       bundle.Ref{Namespace: resolved.Namespace, Slug: resolved.Slug},
			Cleanup:   cleanup,
		}, nil
	}

	if opts.useSample {
		resolved, cachePath, cleanup, err := bundle.ExtractSampleBundle()
		if err != nil {
			return nil, clierrors.Wrap(clierrors.ExitGeneral, "Failed to extract sample bundle", err)
		}

		logger.Info("sample bundle extracted")

		return &bundleSourceResult{
			Kind:      bundleSourceSample,
			Resolved:  resolved,
			CachePath: cachePath,
			Ref:       bundle.Ref{Namespace: resolved.Namespace, Slug: resolved.Slug},
			Cleanup:   cleanup,
		}, nil
	}

	ref, err := bundle.ParseRef(opts.refArg)
	if err != nil {
		return nil, &clierrors.CLIError{
			Message: err.Error(),
			Hint:    "Use format: namespace/slug or namespace/slug:version",
			Code:    clierrors.ExitUsage,
		}
	}

	source, apiClient, _, err := tryAPIClient()
	if err != nil {
		return nil, err
	}

	if source != "" {
		out.Print("Using credentials from: %s\n", source)
	} else {
		out.Info("No credentials found; attempting public bundle access")
	}

	resolved, cachePath, err := bundle.Pull(ctx, apiClient, ref.Namespace, ref.Slug, ref.Version, out)
	if err != nil {
		logger.Error("bundle pull failed", slog.String("error", err.Error()))

		if !apiClient.IsAuthenticated() && isForbiddenError(err) {
			return nil, &clierrors.CLIError{
				Message: fmt.Sprintf("Failed to pull bundle: %s", ref.Slug),
				Hint:    "This bundle may be private. Run 'mush auth login' to authenticate",
				Cause:   err,
				Code:    clierrors.ExitAuth,
			}
		}

		return nil, clierrors.Wrap(clierrors.ExitNetwork, "Failed to pull bundle", err).
			WithHint("Check your network connection and bundle reference")
	}

	return &bundleSourceResult{
		Kind:      bundleSourceRemote,
		Resolved:  resolved,
		CachePath: cachePath,
		Ref:       ref,
		Cleanup:   func() {},
	}, nil
}
