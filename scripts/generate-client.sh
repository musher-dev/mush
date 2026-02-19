#!/usr/bin/env bash
set -euo pipefail

spec_path="../api/openapi.json"
out_path="internal/client/generated.go"

if [[ ! -f "${spec_path}" ]]; then
  echo "OpenAPI spec not found at ${spec_path}"
  echo "Run 'task api:dev' first to generate the spec"
  exit 1
fi

oapi-codegen -package client -generate types,client "${spec_path}" >"${out_path}"
echo "Generated API client from OpenAPI spec"
