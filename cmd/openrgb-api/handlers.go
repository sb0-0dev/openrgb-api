// SPDX-FileCopyrightText: 2026 sb0-0dev
// SPDX-License-Identifier: GPL-2.0-or-later
// This file implements HTTP handlers for pairing, configuration, and OpenRGB control.

package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	openrgb "github.com/sb0-0dev/openrgb-golang/client"
	"github.com/skip2/go-qrcode"
)

// index serves the embedded single-page configuration UI.
func (s *server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.ServeFileFS(w, r, webFiles, "web/index.html")
}

// health reports that the HTTP process is running.
func (s *server) health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// startPairing creates a short-lived code that can be exchanged once by a mobile app.
func (s *server) startPairing(w http.ResponseWriter, r *http.Request) {
	if !isLocalBrowserRequest(r) {
		writeError(w, http.StatusForbidden, fmt.Errorf("pairing must be started from this computer"))
		return
	}
	var request pairingStartRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4*1024)).Decode(&request); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, fmt.Errorf("decode pairing request: %w", err))
		return
	}
	// retrieve the list of available pairing server URLs
	serverURLs := s.pairingServerURLs()
	selected := ""
	if len(serverURLs) > 0 {
		selected = serverURLs[0]
	}
	// select the default server URL if none is requested
	if requested := strings.TrimSpace(request.ServerURL); requested != "" {
		normalized, err := normalizeServerURL(requested)
		if err != nil || !slices.Contains(serverURLs, normalized) {
			writeError(w, http.StatusBadRequest, fmt.Errorf("server_url must be one of the listed addresses"))
			return
		}
		selected = normalized
	}
	// initiate the pairing process and obtain a short-lived code and its expiration time
	code, expiresAt, err := s.auth.startPairing()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	// prepare the response with the pairing code, expiration time, and available server URLs
	response := pairingCodeResponse{Code: code, ExpiresAt: expiresAt, ServerURLs: serverURLs}
	if selected != "" {
		response.ServerURL = selected
		response.QRImage, err = generatePairingQR(selected, code, expiresAt)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, response)
}

// generatePairingQR encodes the server address and short-lived pairing credentials as a PNG data URL.
func generatePairingQR(serverURL, code string, expiresAt time.Time) (string, error) {
	payload, err := json.Marshal(pairingQRPayload{ServerURL: serverURL, Code: code, ExpiresAt: expiresAt})
	if err != nil {
		return "", fmt.Errorf("encode pairing QR payload: %w", err)
	}
	// generate the QR code image from the payload
	image, err := qrcode.Encode(string(payload), qrcode.Medium, 320)
	if err != nil {
		return "", fmt.Errorf("generate pairing QR code: %w", err)
	}
	// encode the QR code image as a base64 data URL
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(image), nil
}

// exchangePairingCode consumes a local pairing code and returns a persistent device token.
func (s *server) exchangePairingCode(w http.ResponseWriter, r *http.Request) {
	var request pairingExchangeRequest
	// decode the incoming pairing exchange request
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8*1024))
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("decode pairing request: %w", err))
		return
	}
	// validate the device name length
	if strings.TrimSpace(request.DeviceName) == "" || len(strings.TrimSpace(request.DeviceName)) > 64 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("device_name must contain 1 to 64 characters"))
		return
	}
	// exchange the pairing code for a persistent device token
	issued, err := s.auth.exchangePairingCode(request.Code, request.DeviceName)
	if errors.Is(err, errInvalidPairingCode) {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, pairingExchangeResponse{
		AccessToken: issued.AccessToken,
		TokenType:   "Bearer",
		Device:      issued.Device,
	})
}

// listPairedDevices exposes paired device metadata only to the local machine.
func (s *server) listPairedDevices(w http.ResponseWriter, r *http.Request) {
	if !isLocalBrowserRequest(r) {
		writeError(w, http.StatusForbidden, fmt.Errorf("paired devices can only be managed from this computer"))
		return
	}
	writeJSON(w, http.StatusOK, pairedDevicesResponse{Devices: s.auth.listDevices()})
}

// revokePairedDevice revokes a device token from the local management page.
func (s *server) revokePairedDevice(w http.ResponseWriter, r *http.Request) {
	if !isLocalBrowserRequest(r) {
		writeError(w, http.StatusForbidden, fmt.Errorf("paired devices can only be managed from this computer"))
		return
	}
	if err := s.auth.revokeDevice(r.PathValue("id")); err != nil {
		if errors.Is(err, errPairedDeviceNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// getConfig returns the current settings without exposing internal state.
func (s *server) getConfig(w http.ResponseWriter, _ *http.Request) {
	s.configMu.RLock()
	response := configResponse{Config: s.config, Path: s.configPath}
	s.configMu.RUnlock()
	writeJSON(w, http.StatusOK, response)
}

// updateConfig validates and persists settings submitted by the browser.
func (s *server) updateConfig(w http.ResponseWriter, r *http.Request) {
	var config Config
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024))
	if err := decoder.Decode(&config); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("decode config: %w", err))
		return
	}
	if err := SaveConfig(s.configPath, config); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	s.configMu.Lock()
	s.config = config
	s.configMu.Unlock()
	writeJSON(w, http.StatusOK, configResponse{Config: config, Path: s.configPath})
}

// controllers returns a browser-friendly summary of discovered controllers.
func (s *server) controllers(w http.ResponseWriter, r *http.Request) {
	client := requireOpenRGBClient(s, w, r)
	if client == nil {
		return
	}
	count, err := client.GetControllerCount()
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}

	controllers := make([]controllerResponse, 0, count)
	for index := 0; index < count; index++ {
		controller, err := client.GetControllerData(uint32(index))
		if err != nil {
			writeError(w, http.StatusBadGateway, fmt.Errorf("get controller %d: %w", index, err))
			return
		}
		controllers = append(controllers, controllerResponse{
			ID:       controller.ID,
			Name:     controller.Name,
			Type:     controller.Type,
			TypeName: controller.Type.String(),
			Vendor:   controller.Vendor,
			Model:    controller.Model,
			Serial:   controller.Serial,
			Modes:    len(controller.Modes),
			Zones:    len(controller.Zones),
			LEDs:     len(controller.LEDs),
		})
	}
	writeJSON(w, http.StatusOK, controllers)
}

// profiles returns a list of available profiles on the OpenRGB server.
func (s *server) profiles(w http.ResponseWriter, r *http.Request) {
	client := requireOpenRGBClient(s, w, r)
	if client == nil {
		return
	}
	profiles, err := client.GetProfiles()
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, ProfilesResponse{Profiles: profiles})
}

// loadProfile loads a profile on the OpenRGB server.
func (s *server) loadProfile(w http.ResponseWriter, r *http.Request) {
	var request ProfileRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024))
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("decode load profile request: %w", err))
		return
	}
	if strings.TrimSpace(request.Profile) == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("profile is required"))
		return
	}

	client := requireOpenRGBClient(s, w, r)
	if client == nil {
		return
	}
	if err := client.LoadProfile(request.Profile); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "loaded",
		"profile": request.Profile,
	})
}

// saveProfile saves a profile on the OpenRGB server.
func (s *server) saveProfile(w http.ResponseWriter, r *http.Request) {
	var request ProfileRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024))
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("decode save profile request: %w", err))
		return
	}
	if strings.TrimSpace(request.Profile) == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("profile is required"))
		return
	}

	client := requireOpenRGBClient(s, w, r)
	if client == nil {
		return
	}
	if err := client.SaveProfile(request.Profile); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "saved",
		"profile": request.Profile,
	})
}

// deleteProfile deletes a profile on the OpenRGB server.
func (s *server) deleteProfile(w http.ResponseWriter, r *http.Request) {
	var request ProfileRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024))
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("decode delete profile request: %w", err))
		return
	}
	if strings.TrimSpace(request.Profile) == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("profile is required"))
		return
	}

	client := requireOpenRGBClient(s, w, r)
	if client == nil {
		return
	}
	if err := client.DeleteProfile(request.Profile); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "deleted",
		"profile": request.Profile,
	})
}

// rescan requests that the OpenRGB server re-detect all connected controllers.
func (s *server) rescan(w http.ResponseWriter, r *http.Request) {
	client := requireOpenRGBClient(s, w, r)
	if client == nil {
		return
	}
	if err := client.RescanDevices(); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "rescanned",
	})
}

// deviceFromRequest extracts the controller ID from the request and returns the corresponding device.
func (s *server) deviceFromRequest(w http.ResponseWriter, r *http.Request) *openrgb.Device {
	id, err := strconv.ParseUint(r.URL.Query().Get("id"), 10, 32)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("id must be a controller ID"))
		return nil
	}
	client := requireOpenRGBClient(s, w, r)
	if client == nil {
		return nil
	}
	device, err := client.GetDeviceByID(uint32(id))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return nil
	}
	return device
}

// getDeviceLeds returns a list of LEDs for the specified controller.
func (s *server) getDeviceLeds(w http.ResponseWriter, r *http.Request) {
	device := s.deviceFromRequest(w, r)
	if device == nil {
		return
	}
	if err := device.RefreshDevice(); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	ledData, err := device.GetLEDs()
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	controller, err := openRGBClientFromRequest(s, r).GetController(device.ControllerID)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	leds := make([]ledResponse, 0, len(ledData))
	for index, led := range ledData {
		var color *openrgb.RGBColor
		if index < len(controller.Colors) {
			currentColor := controller.Colors[index]
			color = &currentColor
		}
		leds = append(leds, ledResponse{Name: led.Name, Value: led.Value, Color: color})
	}
	writeJSON(w, http.StatusOK, leds)
}

// updateDeviceLeds updates every LED with its corresponding color in the request.
func (s *server) updateDeviceLeds(w http.ResponseWriter, r *http.Request) {
	var request deviceLedsRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4*1024*1024))
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("decode LED colors request: %w", err))
		return
	}
	device := s.deviceFromRequest(w, r)
	if device == nil {
		return
	}
	leds, err := device.GetLEDs()
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if len(request.Colors) != len(leds) {
		writeError(w, http.StatusBadRequest, fmt.Errorf("expected %d LED colors, got %d", len(leds), len(request.Colors)))
		return
	}
	if err := device.UpdateLedsWithColors(request.Colors); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := openRGBClientFromRequest(s, r).FlushWrites(); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "updated", "leds": len(request.Colors)})
}

// getDeviceZones returns a list of zones for the specified controller.
func (s *server) getDeviceZones(w http.ResponseWriter, r *http.Request) {
	device := s.deviceFromRequest(w, r)
	if device == nil {
		return
	}
	if err := device.RefreshDevice(); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	zoneData, err := device.GetZones()
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	zones := make([]zoneResponse, 0, len(zoneData))
	for _, zone := range zoneData {
		zones = append(zones, zoneResponse{
			Name: zone.Name, ZoneType: zone.ZoneType, LEDs: zone.NumLeds, Segments: zone.Segments,
		})
	}
	writeJSON(w, http.StatusOK, zones)
}

// getDeviceSegments returns the segments for a specified device zone.
func (s *server) getDeviceSegments(w http.ResponseWriter, r *http.Request) {
	device := s.deviceFromRequest(w, r)
	if device == nil {
		return
	}
	zoneIndex, err := strconv.Atoi(r.URL.Query().Get("zone"))
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("zone must be a zone index"))
		return
	}
	segments, err := device.GetSegments(zoneIndex)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	response := make([]segmentResponse, 0, len(segments))
	for index, segment := range segments {
		response = append(response, segmentResponse{Index: index, Segment: segment})
	}
	writeJSON(w, http.StatusOK, response)
}

// addDeviceSegments adds segments to a zone on the specified controller.
func (s *server) addDeviceSegments(w http.ResponseWriter, r *http.Request) {
	var request addSegmentsRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024))
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("decode add segments request: %w", err))
		return
	}
	device := s.deviceFromRequest(w, r)
	if device == nil {
		return
	}
	// refresh the device to get the latest zone and segment information
	if err := device.RefreshDevice(); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	// retrieve the current zones from the device
	zones, err := device.GetZones()
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	// validate the requested zone index against the retrieved zones
	if request.Zone < 0 || request.Zone >= len(zones) {
		writeError(w, http.StatusBadRequest, fmt.Errorf("zone index %d is out of range", request.Zone))
		return
	}
	// ensure that at least one segment is specified in the request
	if len(request.Segments) == 0 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("at least one segment is required"))
		return
	}
	// determine the target LED count for the zone, defaulting to the current zone size if not specified
	zone := zones[request.Zone]
	targetLEDCount := request.ZoneLEDs
	if targetLEDCount == 0 {
		targetLEDCount = zone.NumLeds
	}
	// validate the segment layout against the target LED count
	if err := validateSegmentLayout(request.Segments, targetLEDCount); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	// resize the zone if the target LED count differs from the current zone size
	if zone.NumLeds != targetLEDCount {
		if err := device.ResizeZone(zone.Name, targetLEDCount); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := openRGBClientFromRequest(s, r).FlushWrites(); err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		// refresh the device to ensure the zone resize took effect
		if err := device.RefreshDevice(); err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		// update the local zone reference after the refresh
		zones, err = device.GetZones()
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		// re-validate the target LED count against the updated zone information
		zone = zones[request.Zone]
		if zone.NumLeds != targetLEDCount {
			writeError(w, http.StatusBadRequest, fmt.Errorf("zone has %d LEDs after resize, expected %d", zone.NumLeds, targetLEDCount))
			return
		}
	}
	// create a new segment collection for the zone
	collection, err := device.NewSegment(zone.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	// populate the segment collection with the requested segments
	collection.Segments = make([]openrgb.SegmentItem, len(request.Segments))
	for index, segment := range request.Segments {
		if segment.NumLeds == 0 || uint64(segment.StartIdx)+uint64(segment.NumLeds) > uint64(collection.NumLeds) {
			writeError(w, http.StatusBadRequest, fmt.Errorf("segment %d exceeds zone LED bounds", index))
			return
		}
		collection.Segments[index] = openrgb.SegmentItem{SegmentIdx: int32(index), Segment: segment}
	}
	// add the segment collection to the device
	if err := device.AddSegment(*collection); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := openRGBClientFromRequest(s, r).FlushWrites(); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if err := device.RefreshDevice(); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "added", "zone": request.Zone, "segments": len(request.Segments)})
}

// validateSegmentLayout requires segments to cover every zone LED exactly once without gaps.
func validateSegmentLayout(segments []openrgb.Segment, ledCount uint32) error {
	// Sort a copy so validation does not change the order supplied by the caller.
	ordered := append([]openrgb.Segment(nil), segments...)
	sort.Slice(ordered, func(left, right int) bool {
		return ordered[left].StartIdx < ordered[right].StartIdx
	})

	var nextExpectedLED uint64
	for index, segment := range segments {
		if strings.TrimSpace(segment.Name) == "" {
			return fmt.Errorf("segment %d name is required", index)
		}
	}
	for index, segment := range ordered {
		start := uint64(segment.StartIdx)
		end := start + uint64(segment.NumLeds)
		if segment.NumLeds == 0 || end > uint64(ledCount) {
			return fmt.Errorf("segment %d exceeds zone LED bounds", index)
		}
		if start < nextExpectedLED {
			return fmt.Errorf("segment %d starts at LED %d before the previous segment ends at %d", index+1, start, nextExpectedLED)
		}
		if start > nextExpectedLED {
			return fmt.Errorf("segment %d starts at LED %d; expected LED %d", index+1, start, nextExpectedLED)
		}
		nextExpectedLED = end
	}
	if nextExpectedLED != uint64(ledCount) {
		return fmt.Errorf("segments cover %d of %d LEDs in the zone", nextExpectedLED, ledCount)
	}
	return nil
}

// clearDeviceSegments removes all segments from the specified device zone.
func (s *server) clearDeviceSegments(w http.ResponseWriter, r *http.Request) {
	device := s.deviceFromRequest(w, r)
	if device == nil {
		return
	}
	// convert the zone query parameter to an integer index
	zoneIndex, err := strconv.Atoi(r.URL.Query().Get("zone"))
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("zone must be a zone index"))
		return
	}
	zones, err := device.GetZones()
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if zoneIndex < 0 || zoneIndex >= len(zones) {
		writeError(w, http.StatusBadRequest, fmt.Errorf("zone index %d is out of range", zoneIndex))
		return
	}
	// clear all segments from the specified zone
	if err := device.ClearSegments(zones[zoneIndex].Name); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := openRGBClientFromRequest(s, r).FlushWrites(); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if err := device.RefreshDevice(); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "cleared", "zone": zoneIndex})
}

// refreshDevice refreshes the selected controller's cached state from the server.
func (s *server) refreshDevice(w http.ResponseWriter, r *http.Request) {
	device := s.deviceFromRequest(w, r)
	if device == nil {
		return
	}
	if err := device.RefreshDevice(); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "refreshed", "id": device.ControllerID})
}

// getDeviceModes returns a list of modes for the specified controller.
func (s *server) getDeviceModes(w http.ResponseWriter, r *http.Request) {
	device := s.deviceFromRequest(w, r)
	if device == nil {
		return
	}
	if err := device.RefreshDevice(); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	modeData, err := device.GetModes()
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	modes := make([]modeResponse, 0, len(modeData))
	for index, mode := range modeData {
		modes = append(modes, modeResponse{
			Name: mode.Name, IsCurrent: index == device.ModeIdx,
			SpeedMin: mode.SpeedMin, SpeedMax: mode.SpeedMax,
			BrightnessMin: mode.BrightnessMin, BrightnessMax: mode.BrightnessMax,
			ColorsMin: mode.ColorsMin, ColorsMax: mode.ColorsMax, ColorMode: mode.ColorMode,
			ColorModeName: mode.ColorMode.String(),
			Colors:        mode.Colors, SupportsSpeed: mode.Flags.SupportsSpeed(),
			SupportsBrightness: mode.Flags.SupportsBrightness(), SupportsDirectionLR: mode.Flags.SupportsDirectionLR(),
			SupportsDirectionUD: mode.Flags.SupportsDirectionUD(), SupportsDirectionHV: mode.Flags.SupportsDirectionHV(),
			SupportsUniqueLEDColor: mode.Flags.SupportsUniqueLEDColor(), SupportsModeColor: mode.Flags.SupportsModeColor(),
			SupportsRandomColor: mode.Flags.SupportsRandomColor(),
		})
	}
	writeJSON(w, http.StatusOK, modes)
}

// setDeviceMode sets a supported mode for the specified controller including mode options
func (s *server) setDeviceMode(w http.ResponseWriter, r *http.Request) {
	var request deviceModeRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024))
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("decode set device mode request: %w", err))
		return
	}
	device := s.deviceFromRequest(w, r)
	if device == nil {
		return
	}
	// validate that a mode is specified
	if strings.TrimSpace(request.Mode) == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("mode is required"))
		return
	}
	// parse and validate direction
	var direction openrgb.Direction
	switch strings.ToLower(strings.TrimSpace(request.Direction)) {
	case "", "left":
		direction = openrgb.LEFT
	case "right":
		direction = openrgb.RIGHT
	case "up":
		direction = openrgb.UP
	case "down":
		direction = openrgb.DOWN
	case "horizontal":
		direction = openrgb.HORIZONTAL
	case "vertical":
		direction = openrgb.VERTICAL
	default:
		writeError(w, http.StatusBadRequest, fmt.Errorf("unsupported direction %q", request.Direction))
		return
	}
	// set the device mode options based on the request
	device.ModeOptions = openrgb.ModeOptions{
		Colors:       request.Colors,
		ColorMode:    request.ColorMode,
		Speed:        openrgb.Speed(request.Speed),
		Direction:    direction,
		SaveToDevice: request.SaveToDevice,
	}
	// set the brightness if provided in the request
	if request.Brightness != nil {
		device.ModeOptions.Brightness = int(*request.Brightness)
	}
	if err := device.SetMode(request.Mode); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	// The SDK treats a zero brightness in the mode options as "unset", so an explicit
	// zero has to go through SetBrightness.
	if request.Brightness != nil && *request.Brightness == 0 {
		if err := device.SetBrightness(0); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	if err := openRGBClientFromRequest(s, r).FlushWrites(); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "updated",
		"mode":   request.Mode,
	})
}

// setDeviceColor updates all device LEDs, all LEDs in a zone, one LED, or one segment.
func (s *server) setDeviceColor(w http.ResponseWriter, r *http.Request) {
	var request deviceColorRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024))
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("decode set device color request: %w", err))
		return
	}
	device := s.deviceFromRequest(w, r)
	if device == nil {
		return
	}
	// validate that at least one target (LED, segment, or zone) is specified
	if request.Segment != nil && request.Zone == nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("segment requires a zone"))
		return
	}
	if request.LED != nil && (request.Zone != nil || request.Segment != nil) {
		writeError(w, http.StatusBadRequest, fmt.Errorf("LED cannot be combined with a zone or segment"))
		return
	}
	if err := device.SetCustomMode(); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	color := openrgb.RGBColor{R: request.R, G: request.G, B: request.B}
	var err error
	target := "device"
	// determine the target based on the request
	switch {
	case request.LED != nil:
		leds, getErr := device.GetLEDs()
		if getErr != nil {
			err = getErr
			break
		}
		if *request.LED < 0 || *request.LED >= len(leds) {
			err = fmt.Errorf("LED index %d is out of range", *request.LED)
			break
		}
		target = fmt.Sprintf("led:%d", *request.LED)
		err = device.UpdateLedWithOptions(leds[*request.LED].Name, color)
	case request.Segment != nil:
		if *request.Zone < 0 {
			err = fmt.Errorf("zone index %d is out of range", *request.Zone)
			break
		}
		if err := device.RefreshDevice(); err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		target = fmt.Sprintf("segment:%d/%d", *request.Zone, *request.Segment)
		err = device.UpdateSegmentLeds(*request.Zone, *request.Segment, color)
	case request.Zone != nil:
		zones, getErr := device.GetZones()
		if getErr != nil {
			err = getErr
			break
		}
		if *request.Zone < 0 || *request.Zone >= len(zones) {
			err = fmt.Errorf("zone index %d is out of range", *request.Zone)
			break
		}
		target = fmt.Sprintf("zone:%d", *request.Zone)
		err = device.SetZoneColor(zones[*request.Zone].Name, color)
	default:
		err = device.SetColor(color)
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := openRGBClientFromRequest(s, r).FlushWrites(); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated", "target": target})
}

// resizeDeviceZones resizes a zone on the specified controller.
func (s *server) resizeDeviceZones(w http.ResponseWriter, r *http.Request) {
	var request resizeZoneRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024))
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("decode resize zone request: %w", err))
		return
	}
	device := s.deviceFromRequest(w, r)
	if device == nil {
		return
	}
	if strings.TrimSpace(request.Zone) == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("zone is required"))
		return
	}

	if err := device.ResizeZone(request.Zone, request.Size); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := openRGBClientFromRequest(s, r).FlushWrites(); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if err := device.RefreshDevice(); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "resized",
		"zone":   request.Zone,
	})
}

// getEffects returns a list of configured effects on the OpenRGB server.
func (s *server) getEffects(w http.ResponseWriter, r *http.Request) {
	client := requireOpenRGBClient(s, w, r)
	if client == nil {
		return
	}
	effects, err := client.GetEffectList()
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	// extract the effect names and enabled effect names from the retrieved effects
	effectNames := make([]string, 0, len(effects))
	enabledEffectNames := make([]string, 0, len(effects))
	for _, effect := range effects {
		effectNames = append(effectNames, effect.Name)
		if effect.Enabled {
			enabledEffectNames = append(enabledEffectNames, effect.Name)
		}
	}
	writeJSON(w, http.StatusOK, effectsResponse{
		Effects:        effectNames,
		EnabledEffects: enabledEffectNames,
	})
}

// setEffect sets a configured effect on the OpenRGB server.
func (s *server) setEffect(w http.ResponseWriter, r *http.Request) {
	var request effectRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024))
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("decode set effect request: %w", err))
		return
	}
	if strings.TrimSpace(request.Effect) == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("effect is required"))
		return
	}

	client := requireOpenRGBClient(s, w, r)
	if client == nil {
		return
	}
	if err := client.SetEffect(request.Effect, request.Enable); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "updated",
		"effect": request.Effect,
	})
}
