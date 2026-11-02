// SPDX-FileCopyrightText: 2026 sb0-0dev
// SPDX-License-Identifier: GPL-2.0-or-later
// This file defines JSON request and response models used by the HTTP API.

package main

import (
	"time"

	openrgb "github.com/sb0-0dev/openrgb-golang/client"
)

// controllerResponse is the small controller view exposed to the browser.
type controllerResponse struct {
	ID       int                `json:"id"`
	Name     string             `json:"name"`
	Type     openrgb.DeviceType `json:"type"`
	TypeName string             `json:"type_name"`
	Vendor   string             `json:"vendor"`
	Model    string             `json:"model"`
	Serial   string             `json:"serial"`
	Modes    int                `json:"modes"`
	Zones    int                `json:"zones"`
	LEDs     int                `json:"leds"`
}

// ProfilesResponse provides a list of available profiles available on the server.
type ProfilesResponse struct {
	Profiles []string `json:"profiles"`
}

// ProfileRequest describes a request to load or save or delete a profile on the OpenRGB server.
type ProfileRequest struct {
	Profile string `json:"profile"`
}

// configResponse adds the path to the editable configuration file.
type configResponse struct {
	Config
	Path string `json:"path"`
}

// pairingCodeResponse represents the response containing a pairing code and its associated metadata.
type pairingCodeResponse struct {
	Code       string    `json:"code"`
	ExpiresAt  time.Time `json:"expires_at"`
	QRImage    string    `json:"qr_image,omitempty"`
	ServerURL  string    `json:"server_url,omitempty"`
	ServerURLs []string  `json:"server_urls,omitempty"`
}

// pairingStartRequest optionally selects which server address the QR code encodes when initiating pairing.
type pairingStartRequest struct {
	ServerURL string `json:"server_url"`
}

// pairingQRPayload represents the payload encoded in the QR code for pairing.
type pairingQRPayload struct {
	ServerURL string    `json:"server_url"`
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expires_at"`
}

// pairingExchangeRequest describes the request to exchange a pairing code for an access token.
type pairingExchangeRequest struct {
	Code       string `json:"code"`
	DeviceName string `json:"device_name"`
}

// pairingExchangeResponse represents the response returned after successfully exchanging a pairing code.
type pairingExchangeResponse struct {
	AccessToken string       `json:"access_token"`
	TokenType   string       `json:"token_type"`
	Device      pairedDevice `json:"device"`
}

// pairedDevicesResponse provides a list of devices that have been paired with the server.
type pairedDevicesResponse struct {
	Devices []pairedDevice `json:"devices"`
}

// ledResponse describes a single LED on a device.
type ledResponse struct {
	Name  string            `json:"name"`
	Value int               `json:"value"`
	Color *openrgb.RGBColor `json:"color,omitempty"`
}

// zoneResponse describes a zone on a device.
type zoneResponse struct {
	Name     string `json:"name"`
	ZoneType int    `json:"zone_type"`
	LEDs     uint32 `json:"leds"`
	Segments uint16 `json:"segments"`
}

// segmentResponse describes a segment on a device zone.
type segmentResponse struct {
	Index int `json:"index"`
	openrgb.Segment
}

// addSegmentsRequest describes segments to add to a device zone.
type addSegmentsRequest struct {
	Zone     int               `json:"zone"`
	ZoneLEDs uint32            `json:"zone_leds"`
	Segments []openrgb.Segment `json:"segments"`
}

// deviceLedsRequest describes a color for each LED on a device.
type deviceLedsRequest struct {
	Colors []openrgb.RGBColor `json:"colors"`
}

// modeResponse describes a device mode on the OpenRGB server.
type modeResponse struct {
	Name                   string             `json:"name"`
	IsCurrent              bool               `json:"is_current"`
	SpeedMin               uint32             `json:"speed_min"`
	SpeedMax               uint32             `json:"speed_max"`
	BrightnessMin          uint32             `json:"brightness_min"`
	BrightnessMax          uint32             `json:"brightness_max"`
	ColorsMin              uint32             `json:"colors_min"`
	ColorsMax              uint32             `json:"colors_max"`
	ColorMode              openrgb.ColorMode  `json:"color_mode"`
	ColorModeName          string             `json:"color_mode_name"`
	Colors                 []openrgb.RGBColor `json:"colors"`
	SupportsSpeed          bool               `json:"supports_speed"`
	SupportsBrightness     bool               `json:"supports_brightness"`
	SupportsDirectionLR    bool               `json:"supports_direction_lr"`
	SupportsDirectionUD    bool               `json:"supports_direction_ud"`
	SupportsDirectionHV    bool               `json:"supports_direction_hv"`
	SupportsUniqueLEDColor bool               `json:"supports_unique_led_color"`
	SupportsModeColor      bool               `json:"supports_mode_color"`
	SupportsRandomColor    bool               `json:"supports_random_color"`
}

// deviceModeRequest describes a request to set the mode of a specific device on the OpenRGB server.
type deviceModeRequest struct {
	Mode         string             `json:"mode"`
	Brightness   *uint32            `json:"brightness"`
	Speed        uint32             `json:"speed"`
	Direction    string             `json:"direction"`
	Colors       []openrgb.RGBColor `json:"colors"`
	ColorMode    openrgb.ColorMode  `json:"color_mode"`
	SaveToDevice bool               `json:"save_to_device"`
}

// deviceColorRequest describes a request to set the color of a specific LED, zone, or segment on a device.
type deviceColorRequest struct {
	R       uint8 `json:"r"`
	G       uint8 `json:"g"`
	B       uint8 `json:"b"`
	LED     *int  `json:"led"`
	Zone    *int  `json:"zone"`
	Segment *int  `json:"segment"`
}

// resizeZoneRequest describes a request to resize a zone on a device.
type resizeZoneRequest struct {
	Zone string `json:"zone"`
	Size uint32 `json:"size"`
}

// effectsResponse provides a list of available effects available on the server.
type effectsResponse struct {
	Effects        []string `json:"effects"`
	EnabledEffects []string `json:"enabled_effects"`
}

// effectRequest describes a request to enable or disable a specific effect on the OpenRGB server.
type effectRequest struct {
	Effect string `json:"effect"`
	Enable bool   `json:"enable"`
}
