// SPDX-FileCopyrightText: 2026 sb0-0dev
// SPDX-License-Identifier: GPL-2.0-or-later
// This file exposes the API version and capabilities supported by this server.

package main

import "net/http"

// serverVersion is the compatibility and capability document consumed by clients.
// Keep features aligned with routes implemented by this release.
type serverVersion struct {
	APIVersion            int      `json:"apiVersion"`
	MinimumClientVersion  string   `json:"minimumClientVersion"`
	RecommendedAPIVersion int      `json:"recommendedApiVersion"`
	Features              []string `json:"features"`
	Message               string   `json:"message,omitempty"`
}

// version reports the API contract for clients that have completed pairing.
func (s *server) version(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, serverVersion{
		APIVersion:            1,
		MinimumClientVersion:  "0.0.1",
		RecommendedAPIVersion: 1,
		Features:              []string{"controllers", "effects", "lightingProfiles"},
	})
}
