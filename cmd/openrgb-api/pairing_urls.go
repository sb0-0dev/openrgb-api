// SPDX-FileCopyrightText: 2026 sb0-0dev
// SPDX-License-Identifier: GPL-2.0-or-later
// This file validates and discovers server URLs used when pairing mobile clients.

package main

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"sort"
	"strings"
)

// normalizeServerURL validates an address a phone can use and strips any trailing slash.
func normalizeServerURL(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("URL must start with http:// or https://")
	}
	if parsed.Hostname() == "" || parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("URL must be a plain address such as http://192.168.1.10:8742")
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

// inContainer reports whether the process runs in a Docker container, where the
// interfaces it can see are not the addresses phones use.
func inContainer() bool {
	_, err := os.Stat("/.dockerenv")
	return err == nil
}

// pairingServerURLs lists the addresses a phone can use to reach the API, best first.
// A configured public URL wins; otherwise they are derived from the listen address.
func (s *server) pairingServerURLs() []string {
	s.configMu.RLock()
	config := s.config
	s.configMu.RUnlock()

	if publicURL := strings.TrimSpace(config.Server.PublicURL); publicURL != "" {
		if normalized, err := normalizeServerURL(publicURL); err == nil {
			return []string{normalized}
		}
		return nil
	}
	if inContainer() {
		return nil
	}
	addresses, _ := net.InterfaceAddrs()
	return lanServerURLs(config.Server.ListenAddress, addresses)
}

// lanServerURLs derives phone-reachable URLs from a listen address. A wildcard address
// expands to the computer's private IPv4 addresses, and a loopback address yields none
// because a phone could not reach it.
func lanServerURLs(listenAddress string, interfaceAddresses []net.Addr) []string {
	host, port, err := net.SplitHostPort(listenAddress)
	if err != nil {
		return nil
	}
	// If the host is a specific IP address, check if it is loopback or unspecified.
	if ip := net.ParseIP(host); ip != nil && !ip.IsUnspecified() {
		if ip.IsLoopback() {
			return nil
		}
		return []string{"http://" + net.JoinHostPort(host, port)}
	}
	if host != "" && net.ParseIP(host) == nil {
		return []string{"http://" + net.JoinHostPort(host, port)}
	}
	// If the host is a wildcard address, expand to all private IPv4 addresses on the machine.
	var urls []string
	for _, address := range interfaceAddresses {
		network, ok := address.(*net.IPNet)
		if !ok {
			continue
		}
		ip := network.IP.To4()
		if ip == nil || !ip.IsPrivate() || ip.IsLoopback() {
			continue
		}
		urls = append(urls, "http://"+net.JoinHostPort(ip.String(), port))
	}
	sort.Strings(urls)
	return urls
}
