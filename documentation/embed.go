// SPDX-FileCopyrightText: 2026 sb0-0dev
// SPDX-License-Identifier: GPL-2.0-or-later

// Package apispec embeds the OpenAPI specification so the application can serve it.
package apispec

import _ "embed"

// OpenAPISpec is the OpenAPI 3.1 specification for the HTTP API.
//
//go:embed openapi.yaml
var OpenAPISpec []byte
