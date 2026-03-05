// Package bundle provides bundle cache, validation, and asset mapping for the Mush CLI.
package bundle

import (
	"fmt"
	"strings"
)

// Ref is a parsed bundle reference (namespace/slug with optional version).
type Ref struct {
	Namespace string
	Slug      string
	Version   string // empty = "latest"
}

// ParseRef parses a bundle reference string of the form "namespace/slug" or "namespace/slug:version".
func ParseRef(arg string) (Ref, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return Ref{}, fmt.Errorf("bundle reference cannot be empty")
	}

	namespace, remainder, hasSlash := strings.Cut(arg, "/")
	if !hasSlash {
		return Ref{}, fmt.Errorf("bundle reference must include namespace: namespace/slug[:version]")
	}

	if namespace == "" {
		return Ref{}, fmt.Errorf("bundle namespace cannot be empty")
	}

	parts := strings.SplitN(remainder, ":", 2)
	slug := parts[0]

	if slug == "" {
		return Ref{}, fmt.Errorf("bundle slug cannot be empty")
	}

	version := ""
	if len(parts) == 2 {
		version = parts[1]
		if version == "" {
			return Ref{}, fmt.Errorf("bundle version cannot be empty after ':'")
		}
	}

	return Ref{Namespace: namespace, Slug: slug, Version: version}, nil
}

// String returns the string representation of a Ref.
func (r Ref) String() string {
	base := r.Namespace + "/" + r.Slug
	if r.Version == "" {
		return base
	}

	return base + ":" + r.Version
}
