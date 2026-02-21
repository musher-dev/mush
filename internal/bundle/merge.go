package bundle

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/pelletier/go-toml/v2"
)

// MergeJSONDocs merges multiple JSON object documents into one object.
func MergeJSONDocs(existing []byte, docs [][]byte) ([]byte, error) {
	merged := map[string]any{}

	if err := unmarshalJSONObject(existing, merged); err != nil {
		return nil, err
	}

	for i, doc := range docs {
		next := map[string]any{}
		if err := unmarshalJSONObject(doc, next); err != nil {
			return nil, fmt.Errorf("parse json doc %d: %w", i+1, err)
		}

		mergeMaps(merged, next)
	}

	out, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal merged json: %w", err)
	}

	return append(out, '\n'), nil
}

// MergeTOMLDocs merges multiple TOML object documents into one object.
func MergeTOMLDocs(existing []byte, docs [][]byte) ([]byte, error) {
	merged := map[string]any{}

	if err := unmarshalTOMLObject(existing, merged); err != nil {
		return nil, err
	}

	for i, doc := range docs {
		next := map[string]any{}
		if err := unmarshalTOMLObject(doc, next); err != nil {
			return nil, fmt.Errorf("parse toml doc %d: %w", i+1, err)
		}

		mergeMaps(merged, next)
	}

	out, err := toml.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("marshal merged toml: %w", err)
	}

	return out, nil
}

func unmarshalJSONObject(in []byte, dst map[string]any) error {
	trimmed := bytes.TrimSpace(in)
	if len(trimmed) == 0 {
		return nil
	}

	if err := json.Unmarshal(trimmed, &dst); err != nil {
		return fmt.Errorf("parse json object: %w", err)
	}

	return nil
}

func unmarshalTOMLObject(in []byte, dst map[string]any) error {
	trimmed := bytes.TrimSpace(in)
	if len(trimmed) == 0 {
		return nil
	}

	if err := toml.Unmarshal(trimmed, &dst); err != nil {
		return fmt.Errorf("parse toml object: %w", err)
	}

	return nil
}

func mergeMaps(dst, src map[string]any) {
	for k, v := range src {
		existing, ok := dst[k]
		if !ok {
			dst[k] = v
		} else {
			srcMap, srcIsMap := v.(map[string]any)

			dstMap, dstIsMap := existing.(map[string]any)
			if srcIsMap && dstIsMap {
				mergeMaps(dstMap, srcMap)
				dst[k] = dstMap
			} else {
				dst[k] = v
			}
		}
	}
}
