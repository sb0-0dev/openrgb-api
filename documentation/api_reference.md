# OpenRGB API Reference

This document describes the HTTP API served by the OpenRGB API application. It controls the connected OpenRGB SDK server.

An [OpenAPI 3.1 specification](openapi.yaml) is available for Swagger UI and other compatible tools. To try the API interactively, open **http://127.0.0.1:8743/docs** on the computer running the service; it serves Swagger UI (loaded from cdnjs as a pinned, integrity-checked release, so it needs internet access) against the running API with no token required. The spec is also served at `/openapi.yaml` on the control panel.

Contents: [Conventions](#conventions) · [Health](#health) · [Pairing and Authentication](#pairing-and-authentication) · [Configuration](#configuration) · [Controllers and Server](#controllers-and-server) · [Device Operations](#device-operations) · [Error Responses](#error-responses) · [Quick Examples](#quick-examples)

## Conventions

- All paths are relative to the web server's base URL, for example `http://localhost:8742`.
- Requests with a body use JSON and should send `Content-Type: application/json`.
- Successful JSON responses use `Content-Type: application/json`. Errors have the shape `{"error":"message"}`. `204 No Content` has no response body.
- API requests require `Authorization: Bearer <access-token>`, except pairing-code exchange and `/health`. `/api/version` requires the paired device token like other API routes. The API listener never uses the client's address or `Host` header to skip the token. The control panel listener rejects cross-origin requests (an `Origin` header that does not match the request's scheme and host).
- The application serves two listeners. The **API** (`listen_address`, default `127.0.0.1:8742`) serves everything in this document except the pairing management and configuration endpoints, and always requires a bearer token (except `/health` and pairing-code exchange). The **control panel** (`admin_address`, default `127.0.0.1:8743`) serves the web page, `/api/pairing/start`, `/api/pairing/devices` and `/api/config`, in addition to the API endpoints. It needs no token, but only accepts requests from the local machine or addresses in `OPENRGB_API_LOCAL_NETWORKS`, and requires a loopback `Host` header. It also rejects cross-origin requests (an `Origin` header that does not match the request's scheme and host). The trusted-network override is intended for container host access; see the Docker setup notes before configuring it. Both listeners are always started, and `admin_address` is required.
- Controller IDs and LED, zone, and segment indices are zero-based.
- Request bodies are limited to 64 KB (8 KB for pairing exchange). Larger or malformed bodies return `400`.
- Unknown routes return `404`, and a known route with the wrong method returns `405`.
- Traffic is plain HTTP and intended for a trusted local network. See the Security section of the README before exposing the service beyond it.

Example authenticated request:

```sh
curl -H 'Authorization: Bearer <access-token>' \
	'http://localhost:8742/api/controllers'
```

Common errors include `400 Bad Request` for malformed or invalid input, `401 Unauthorized` for a missing/invalid bearer token or pairing code, `403 Forbidden` for disallowed origins or local-only operations, `404 Not Found` for unknown controllers or devices, and `502 Bad Gateway` when an OpenRGB operation fails. See [Error Responses](#error-responses) for details.

When an OpenRGB operation returns `502` because the connection to the SDK server failed (closed, reset, or timed out), the API starts reconnecting in the background, retrying with an increasing delay (up to 30 seconds). A `502` for any other reason, such as a missing plugin or unsupported protocol version, does not trigger a reconnect. The failed request is not replayed, so clients can safely decide whether to retry it; subsequent requests work once the connection has been restored.

The service does not need OpenRGB to be running when it starts. If the SDK server is unreachable at startup, both listeners still come up, `/health`, pairing and settings work, and every OpenRGB-backed endpoint returns `502` with `OpenRGB is not connected; reconnecting in the background` until the background connection succeeds.

## Health

| Method and path | Description | Success |
| --- | --- | --- |
| `GET /health` | Reports that the HTTP process is running. Does not require API authentication. | `200` `{"status":"ok"}` |

## Version and capabilities

`GET /api/version` reports the API compatibility floor, recommended API level, and features provided by this release. It requires a paired device bearer token and is checked by the mobile client after pairing.

```json
{
  "apiVersion": 1,
  "minimumClientVersion": "0.0.1",
  "recommendedApiVersion": 1,
  "features": ["controllers", "effects", "lightingProfiles"]
}
```

`apiVersion` and `recommendedApiVersion` are positive integer API levels starting at `1`. `minimumClientVersion` is the oldest compatible OpenRGB Link client and remains a three-part app version. The current backend reports `0.0.1`; keep this value aligned with `version.go` when compatibility requirements change. The `features` array lists available capabilities; clients can hide functionality that is not advertised. The optional `message` field can provide an upgrade or compatibility notice. Update these values when backend compatibility or supported capabilities change, and keep the feature list aligned with implemented routes.

## Pairing and Authentication

Pair a mobile client by starting pairing in the local browser, then exchange the one-time code from the client. The exchange returns a bearer token; store it securely and send it with subsequent API requests.

Pairing code rules:

- A code is 13 characters (letters and digits). When entered by hand, case, spaces, and dashes are ignored.
- Codes expire after five minutes and can be used once.
- Only one code is active at a time; starting pairing again replaces the previous code.
- After five incorrect attempts the active code is invalidated and a new one must be generated on the host computer.
- Failed exchanges return `401` with `pairing code is invalid or expired`, whether the code is wrong, expired, used, or locked out.

Pairing security:

- Access tokens are random 256-bit values. Only a SHA-256 hash is stored, in `paired-devices.json` next to the configuration file (permissions `0600`); the token itself is shown once, in the exchange response.
- Starting pairing, listing devices, revoking devices and changing settings are only served on the control panel listener and are protected by its local-request guard, so a paired phone cannot pair other devices or alter settings. When `OPENRGB_API_LOCAL_NETWORKS` is configured, clients in those CIDRs count as local; this override supports Docker and should be scoped narrowly.
- The five-attempt limit applies to the active code, not to a client address. Anyone who can reach the API listener can therefore use up the active code by guessing, which makes a genuine exchange fail until a new code is generated. This stops brute-forcing; it does not leak anything.

Recovery:

- **Locked out or expired code:** open the control panel on the host computer and start pairing again. A new code replaces the old one and resets the attempt counter.
- **Lost or stolen phone:** revoke it from the paired devices list in the control panel (`DELETE /api/pairing/devices/{id}`). Its token stops working immediately.
- **Start over:** stop the service, delete `paired-devices.json` to revoke every device, then start the service and pair again.
- **Corrupt `paired-devices.json`:** the service refuses to start and reports an invalid paired device record. Fix or delete the file as above.

| Method and path | Description | Success |
| --- | --- | --- |
| `POST /api/pairing/start` | Starts pairing from the local browser. | `200`, described below |
| `POST /api/pairing/exchange` | Exchanges a pairing code for an access token. This is the only API route that does not require an existing token or local browser. | `201`, described below |
| `GET /api/pairing/devices` | Lists paired devices; local browser only. | `200` `{"devices":[...]}` |
| `DELETE /api/pairing/devices/{id}` | Revokes the paired device with the given ID; local browser only. | `204 No Content` |

Start pairing response:

```json
{
	"code": "ONE-TIME-CODE",
	"expires_at": "2026-10-11T12:05:00Z",
	"qr_image": "data:image/png;base64,..."
}
```

`qr_image` is omitted when the server cannot work out an address a phone could use. The QR address comes from `server.public_url` if set, otherwise from the computer's private network addresses (see `listen_address`), never from the address the control panel was opened with. Response fields `server_url` and `server_urls` give the address in the QR code and the alternatives. The request body may be `{"server_url": "..."}` to pick one of the listed addresses. This endpoint is served only on the control panel address (`admin_address`, default `127.0.0.1:8743`) and is subject to its local-request and loopback-Host checks. The QR image is a 320x320 PNG data URL that encodes a JSON payload containing `server_url`, `code`, and `expires_at`.

Exchange request and response:

```http
POST /api/pairing/exchange
Content-Type: application/json

{"code":"ONE-TIME-CODE","device_name":"Living Room Phone"}
```

`device_name` must contain 1 to 64 non-whitespace characters.

```json
{
	"access_token": "...",
	"token_type": "Bearer",
	"device": {
		"id": "...",
		"name": "Living Room Phone",
		"created_at": "2026-10-11T12:00:00Z"
	}
}
```

The paired-device list contains the same device fields, ordered oldest first, but never includes access tokens. Revoking an unknown device ID returns `404`. A revoked token is rejected immediately with `401`.

Pairing endpoints return `403` when called from another machine, even with a valid token.

## Configuration

| Method and path | Description | Success |
| --- | --- | --- |
| `GET /api/config` | Returns the current in-memory settings and configuration file path. | `200`, response below |
| `PUT /api/config` | Validates, persists, and updates the in-memory settings. | `200`, updated response below |

Both responses contain top-level `server`, `openrgb`, and `path` properties. The request body for `PUT` is the configuration object without `path`:

```json
{
	"server": {
		"listen_address":"127.0.0.1:8742",
		"admin_address":"127.0.0.1:8743",
		"public_url":""
	},
	"openrgb": {
		"host":"127.0.0.1",
		"port":"6742",
		"client_name":"openrgb-api"
	}
}
```

`listen_address`, `admin_address`, `host`, `port` and `client_name` are required and must not be blank; `public_url` is optional. `admin_address` must be `host:port` and differ from `listen_address`, and `public_url` must be an `http://` or `https://` address without a path. Otherwise the request fails with `400` and nothing is saved. The response shape is `{"server":{...},"openrgb":{...},"path":"..."}`.

Changes to `listen_address`, `admin_address`, `public_url`, `host`, `port`, and `client_name` are saved immediately but take effect only after the service is restarted. These endpoints are served only on the control panel listener, so paired devices cannot change settings.

## Controllers and Server

| Method and path | Description | Success |
| --- | --- | --- |
| `GET /api/controllers` | Lists discovered controllers and summary counts. | `200`, array of controller objects |
| `POST /api/server/rescan` | Requests device rediscovery from the OpenRGB server. | `200` `{"status":"rescanned"}` |
| `GET /api/server/profiles` | Lists available profiles. | `200` `{"profiles":["..."]}` |
| `POST /api/server/profiles/load` | Loads a profile. | `200` `{"status":"loaded","profile":"..."}` |
| `POST /api/server/profiles/save` | Saves the current state as a profile. | `200` `{"status":"saved","profile":"..."}` |
| `POST /api/server/profiles/delete` | Deletes a profile. | `200` `{"status":"deleted","profile":"..."}` |
| `GET /api/server/effects` | Lists available effects and enabled effects. | `200` `{"effects":[...],"enabled_effects":[...]}` |
| `POST /api/server/effect` | Enables or disables an effect. | `200` `{"status":"updated","effect":"..."}` |

Each profile operation takes `{"profile":"Profile Name"}`. The effect update body is `{"effect":"Effect Name","enable":true}`. The `effect` name must not be blank.

Controller objects contain `id`, `name`, numeric `type`, readable `type_name`, `vendor`, `model`, `serial`, and the `modes`, `zones`, and `leds` counts:

```json
{"id":0,"name":"ASUS Aura","type":1,"type_name":"Motherboard","vendor":"ASUS","model":"","serial":"","modes":12,"zones":3,"leds":18}
```

Profile and effect names are not validated against the server's lists. OpenRGB may silently ignore a name that does not exist, and a failure reported as `502` for such a request does not by itself trigger a reconnect.

## Device Operations

All routes under `/api/device/` select a controller with the `id` query parameter, for example `/api/device/leds?id=0`. A missing or non-numeric `id` returns `400`; an ID that does not match a discovered controller returns `404`. Server-level routes under `/api/server/` do not take an `id`.

### LEDs and Colors

| Method and path | Description | Success |
| --- | --- | --- |
| `GET /api/device/leds?id={id}` | Lists LEDs as `{name,value,color?}` objects; `color` contains the current RGB value when available. | `200`, array of LEDs |
| `POST /api/device/leds?id={id}` | Sets every LED to its corresponding color. The number of colors must exactly match the controller's LED count. | `200` `{"status":"updated","leds":N}` |
| `POST /api/device/color?id={id}` | Enters custom/direct mode and sets the color of the whole device, a zone, one LED, or one segment. | `200` `{"status":"updated","target":"..."}` |

For per-LED updates, send an array in controller LED order:

```json
{"colors":[{"R":255,"G":0,"B":0},{"R":0,"G":0,"B":255}]}
```

RGB channel values are integers from 0 to 255; values outside that range fail request decoding with `400`. Note that per-LED arrays use uppercase `R`, `G`, `B` keys, while the single-color update below uses lowercase `r`, `g`, `b`. Color updates use this request shape:

```json
{"r":255,"g":32,"b":96}
```

Optional selectors `led`, `zone`, or `segment` target one LED index, zone index, or segment index. A `zone` target updates every LED in that zone using `UpdateZoneLeds`. A segment target requires both `zone` and `segment`; an LED target cannot be combined with either. Omit all selectors to color the whole device. Color updates switch the controller to custom/direct mode first.

The `target` in the response identifies what was changed: `device`, `zone:N`, `led:N`, or `segment:Z/S`. Out-of-range indices return `400`.

### Zones and Segments

| Method and path | Description | Success |
| --- | --- | --- |
| `GET /api/device/zones?id={id}` | Lists zones with `name`, numeric `zone_type`, LED count (`leds`), and segment count (`segments`). | `200`, array of zones |
| `POST /api/device/zones/resize?id={id}` | Resizes the named zone. | `200` `{"status":"resized","zone":"..."}` |
| `GET /api/device/segments?id={id}&zone={zone}` | Lists segments in a zone by zero-based zone index. | `200`, array of segments |
| `POST /api/device/segments?id={id}` | Submits a complete, non-overlapping segment layout for a zone index. Segments must cover every zone LED exactly once. | `200` `{"status":"added","zone":N,"segments":N}` |
| `DELETE /api/device/segments?id={id}&zone={zone}` | Clears all segments from the zone index. | `200` `{"status":"cleared","zone":N}` |

Zone resize request:

```json
{"zone":"Zone Name","size":30}
```

Resizing is only available with `POST`. It applies to zones that support resizing; other zones return `400`.

Add-segments request:

```json
{
	"zone": 0,
	"zone_leds": 60,
	"segments": [
		{"name":"Left","segment_type":0,"start_idx":0,"num_leds":30},
		{"name":"Right","segment_type":0,"start_idx":30,"num_leds":30}
	]
}
```

At least one segment is required. `zone_leds` preserves the intended zone size if clearing the previous segments restores the controller's original size. Segment names must not be blank, and their nonzero LED counts and start indexes must cover exactly `zone_leds` LEDs without gaps or overlap.

### Modes and Refresh

| Method and path | Description | Success |
| --- | --- | --- |
| `GET /api/device/modes?id={id}` | Refreshes the device first, then lists modes, current-mode status, option ranges, colors, color mode, and capability flags. | `200`, array of modes |
| `POST /api/device/mode?id={id}` | Selects a mode and applies its options. | `200` `{"status":"updated","mode":"..."}` |
| `POST /api/device/refresh?id={id}` | Refreshes the selected controller state. | `200` `{"status":"refreshed","id":N}` |

Mode update request:

```json
{
	"mode":"Breathing",
	"brightness":75,
	"speed":2,
	"direction":"left",
	"colors":[{"R":255,"G":0,"B":0}],
	"color_mode":1,
	"save_to_device":false
}
```

`mode` is required and must match a mode name returned by `GET /api/device/modes`; an unknown mode returns `400`. `brightness` is optional: omit it to keep the mode's current brightness, or send `0` to set it to zero. `direction` accepts `left`, `right`, `up`, `down`, `horizontal`, or `vertical` (case-insensitive); an empty value defaults to `left`. `speed` and `color_mode` are numeric OpenRGB enum values. The returned mode objects expose `speed_min`, `speed_max`, `brightness_min`, `brightness_max`, `colors_min`, `colors_max`, `color_mode`, `color_mode_name`, and the `supports_*` capability flags.

## Error Responses

JSON errors have this form:

```json
{"error":"description of the error"}
```

| Status | Meaning |
| --- | --- |
| `400` | Invalid query parameters, malformed or oversized JSON, missing required fields, unsupported directions, out-of-range indices, or invalid segment layouts. |
| `401` | Missing or invalid bearer token, or an invalid pairing code. |
| `403` | Cross-origin request, or a local-only operation attempted from another machine. |
| `404` | Unknown controller ID, device, or paired device. |
| `405` | Method not allowed for the route. |
| `500` | Unexpected server failure; details are logged but not returned. |
| `502` | The OpenRGB SDK server failed or is unreachable. |

Missing or invalid bearer tokens return `401` and include `WWW-Authenticate: Bearer realm="openrgb-api"`. Failures communicating with the OpenRGB SDK server return `502`.

## Quick Examples

Replace `TOKEN` with the token returned by pairing. The API listener always requires it; the control panel listener (`127.0.0.1:8743`) does not.

```sh
# List controllers
curl -H "Authorization: Bearer TOKEN" http://192.168.1.20:8742/api/controllers

# Load a saved profile
curl -X POST -H "Authorization: Bearer TOKEN" -H "Content-Type: application/json" \
	-d '{"profile":"Gaming"}' http://192.168.1.20:8742/api/server/profiles/load

# Set controller 0 to red
curl -X POST -H "Authorization: Bearer TOKEN" -H "Content-Type: application/json" \
	-d '{"r":255,"g":0,"b":0}' "http://192.168.1.20:8742/api/device/color?id=0"

# Set one zone of controller 0 to blue
curl -X POST -H "Authorization: Bearer TOKEN" -H "Content-Type: application/json" \
	-d '{"r":0,"g":0,"b":255,"zone":1}' "http://192.168.1.20:8742/api/device/color?id=0"
```
