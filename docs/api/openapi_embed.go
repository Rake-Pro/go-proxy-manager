// Package openapidoc embeds the REST API's OpenAPI 3.1 specification
// (openapi.yaml, next to this file) so internal/server can serve it at
// GET /api/openapi.yaml without shipping the docs/ tree separately. It exists
// purely as a workaround for go:embed's restriction against ascending path
// elements: only a Go file inside docs/api can embed docs/api/openapi.yaml
// directly, and the spec's canonical home has to stay under docs/ (see
// docs/api/README.md).
package openapidoc

import _ "embed"

// Spec is the raw contents of openapi.yaml.
//
//go:embed openapi.yaml
var Spec []byte
