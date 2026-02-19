// Package bundle provides bundle cache, validation, and asset mapping for the Mush CLI.
package bundle

import (
	"fmt"
	"strings"
)

// Ref is a parsed bundle reference (slug with optional version).
type Ref struct {
	Slug    string
	Version string // empty = "latest"
}

// ParseRef parses a bundle reference string of the form "slug" or "slug:version".
func ParseRef(arg string) (Ref, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return Ref{}, fmt.Errorf("bundle reference cannot be empty")
	}

	parts := strings.SplitN(arg, ":", 2)
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

	return Ref{Slug: slug, Version: version}, nil
}

// String returns the string representation of a Ref.
func (r Ref) String() string {
	if r.Version == "" {
		return r.Slug
	}

	return r.Slug + ":" + r.Version
}
