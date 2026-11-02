// SPDX-FileCopyrightText: 2026 sb0-0dev
// SPDX-License-Identifier: GPL-2.0-or-later
// This file serves the OpenAPI specification and a Swagger UI page from the control panel.

package main

import (
	"bytes"
	"net/http"

	apispec "github.com/sb0-0dev/openrgb-api/documentation"
)

// swaggerUIPage loads a pinned Swagger UI release from cdnjs, so viewing it needs internet
// access. Subresource Integrity makes the browser refuse files that differ from that release.
const swaggerUIPage = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>OpenRGB API - Swagger UI</title>
<link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/5.29.1/swagger-ui.css" integrity="sha512-eZpfl9qlKnbDvlJ2brfdx3nhlP1FMsA23w65motxKdYsUcfMcdO2bcLPr7mXhvyzmDZwuzYCJKrl/sEo1ditVQ==" crossorigin="anonymous" referrerpolicy="no-referrer">
</head>
<body>
<div id="swagger-ui"></div>
<script src="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/5.29.1/swagger-ui-bundle.js" integrity="sha512-iDdi6WwSTimAFh1NhPyFZpHJWr16m/PupHztElqQL+gQoQDavUATsP9hcvgs9Yci+EUA//WvZYZsCTDhRDKc3g==" crossorigin="anonymous" referrerpolicy="no-referrer"></script>
<script>
SwaggerUIBundle({ url: "/openapi.yaml", dom_id: "#swagger-ui" });
</script>
</body>
</html>
`

// openAPISpec serves the specification with relative URLs so Swagger requests stay on
// the exact origin used to load the control panel, including localhost versus 127.0.0.1.
func (s *server) openAPISpec(w http.ResponseWriter, _ *http.Request) {
	spec := apispec.OpenAPISpec
	// AdminHandler also registers the API routes, so relative URLs make all
	// Swagger operations same-origin and avoid CORS checks between listeners.
	for _, defaultURL := range []string{"http://127.0.0.1:8742", "http://127.0.0.1:8743"} {
		spec = bytes.ReplaceAll(spec, []byte(defaultURL), []byte("/"))
	}
	w.Header().Set("Content-Type", "application/yaml")
	_, _ = w.Write(spec)
}

// swaggerUI serves an interactive API explorer for testing.
func (s *server) swaggerUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(swaggerUIPage))
}
