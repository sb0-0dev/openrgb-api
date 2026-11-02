# OpenRGB API

OpenRGB API is a local HTTP API for controlling devices through an OpenRGB SDK server. The browser interface and API are bundled into the `openrgb-api` executable.

It is built on the [openrgb-golang](https://github.com/sb0-0dev/openrgb-golang) client library and exposes controllers, zones, segments, modes, profiles and effects over simple JSON endpoints. Applications can pair with a short-lived code and then control lighting with a bearer token.

## Requirements

- Go 1.26.1 or newer, when building or running from source.
- OpenRGB running with its SDK server enabled and reachable.
- The SDK server must be reachable at the host and port the API is configured to use; the default is `127.0.0.1:6742`.

## Install

Prebuilt installers and a container image are available:

- **Windows:** an installer that sets up a Windows service, with an option to allow private-network access.
	- Networks in windows often default to public, which may block private-network access required by the service. Simply add public to the scope of the firewall rule allowed networks if needed.
- **Linux:** a `.tar.gz` containing `install.sh` (systemd service, `--lan` option) and `uninstall.sh`.
- **Docker:** a `Dockerfile` and `compose.yaml`.

See [documentation/setup.md](documentation/setup.md) for installation, configuration and upgrade steps. To build from source, see [Build](#build).

## Run

```sh
go run ./cmd/openrgb-api
```

By default, the API listens at `127.0.0.1:8742`, there is a control panel (settings and pairing) at `127.0.0.1:8743`, and the server connects to the OpenRGB SDK at `127.0.0.1:6742`. Settings are read from the platform's user configuration directory under `openrgb-api/config.toml`. Use `-config` to select a different file, or `-listen`, `-host`, and `-port` to override individual settings.

Application errors are written to `openrgb-api.log` beside the configuration file and to standard error. The active log is limited to 5 MB, with up to three rotated backup files.

The API reference is in [documentation/api_reference.md](documentation/api_reference.md), with an [OpenAPI 3.1 specification](documentation/openapi.yaml) for Swagger UI and other OpenAPI tools. 

## Usage

Open the control panel at `http://127.0.0.1:8743` on the computer running the service to start pairing. Enter or scan the one-time code from the client, which exchanges it for a bearer token. Use that token with every API request:

```sh
# Check the service is running (no token needed)
curl http://127.0.0.1:8742/health

# Check API compatibility and supported features (paired token required)
curl -H "Authorization: Bearer TOKEN" http://127.0.0.1:8742/api/version

# List controllers
curl -H "Authorization: Bearer TOKEN" http://127.0.0.1:8742/api/controllers

# Set controller 0 to red
curl -X POST -H "Authorization: Bearer TOKEN" -H "Content-Type: application/json" \
	-d '{"r":255,"g":0,"b":0}' "http://127.0.0.1:8742/api/device/color?id=0"
```

Replace `127.0.0.1` with the computer's address when connecting from another device on your network. Controller IDs and LED, zone and segment indices are zero-based.

### Supported features

- List discovered controllers and request a rescan.
- Read LEDs, zones, segments and lighting modes, including option ranges and capabilities.
- Set colors for a whole device, a zone, a single LED or a segment, or every LED at once.
- Select lighting modes with speed, brightness, direction and color options.
- Resize zones and manage LED segments.
- List, load, save and delete profiles.
- List effects and enable or disable them.
- Pair devices with one-time codes and revoke them from the control panel.
- Automatically reconnect to the OpenRGB SDK server after a failed operation.

### Try it in Swagger UI

Open `http://127.0.0.1:8743/docs` on the computer running the service to explore and call the API interactively. It loads a pinned Swagger UI release (5.29.1, verified with Subresource Integrity hashes) from cdnjs, so it needs internet access.

## Documentation

- [API reference](documentation/api_reference.md) describes every endpoint, request body, response and error.
- [OpenAPI specification](documentation/openapi.yaml) is the machine-readable equivalent.
- [Setup guide](documentation/setup.md) covers the Windows and Linux installers, Docker, and building from source.

## Security

OpenRGB API is designed for use on a trusted local network only.

- The application runs two listeners. The API (`127.0.0.1:8742` by default) is what paired phones use. It serves no web page, and every request needs a bearer token. The control panel (`127.0.0.1:8743`) serves the settings, pairing and paired-device management, and accepts only requests from the same computer with a loopback `Host` header. Keep it on `127.0.0.1` and never forward or publish it.
- Other devices, such as the mobile app, must pair using a short-lived code and then send a bearer token with every request. Pairing codes expire after 5 minutes, work once, and are invalidated after 5 failed attempts. The attempt limit is per code, not per client, so someone on your network can burn the active code by guessing; generate a new one from the control panel. Tokens are stored only as SHA-256 hashes in `paired-devices.json` (owner-only permissions). See [Pairing and Authentication](documentation/api_reference.md#pairing-and-authentication) for recovery steps.
- Traffic is served over plain HTTP and is **not encrypted**. Anyone who can observe your network traffic could capture a pairing code or access token and control your lighting.

**Do not forward this port to the internet** (port forwarding, UPnP, a public reverse proxy, or a tunnel without its own authentication and encryption). Doing so exposes an unencrypted service to anyone who can reach it, and the bearer token is the only protection. There is no HTTPS, rate limiting by client, or audit logging.

If you need remote access, use a VPN such as WireGuard or Tailscale, which keeps the service off the public internet and encrypts the connection. Alternatively, place it behind a reverse proxy that provides HTTPS, and only then allow external access.

Only enable the private-network option in the installer on networks you trust. If a paired phone is lost, revoke it from the browser interface on the host computer.

The OpenRGB SDK server has no authentication. If you make it listen on all interfaces (for example so a Docker container can reach it), anything that can reach port 6742 can control your lighting. Allow that port through the firewall only for the machines that need it (such as Docker), never from the internet, and prefer binding it to `127.0.0.1` when the web app runs on the same machine.

When running in Docker, publish the control panel port (8743) on `127.0.0.1` only. Publishing the API port (8742) on your network is what lets phones connect, and it is safe to do on a trusted network because it needs a token and serves no control panel. `OPENRGB_API_LOCAL_NETWORKS` makes requests from the listed networks count as the local computer for the control panel check. See [documentation/setup.md](documentation/setup.md#docker).

### Sharing lighting with stream viewers

Safe approach:

- **Use a bridge on your own computer.** Have a chat bot, channel point redemption, or tool such as Streamer.bot call the API on `127.0.0.1`. Restrict it to a fixed set of actions, such as loading named profiles or setting colours, and rate-limit it. Nothing from the internet then reaches this service.

## Build

```sh
go build -trimpath -o openrgb-api ./cmd/openrgb-api
```

On Windows, `scripts/build-windows-installer.ps1` builds the executable and installer. The installer supports local-only access by default, with an option to allow private-network connections.

For Linux, `scripts/build-linux-installer.ps1` cross-compiles a `.tar.gz` containing `install.sh` (systemd service, `--lan` option) and `uninstall.sh`. A `Dockerfile` and `compose.yaml` are also provided. See [documentation/setup.md](documentation/setup.md).

## Troubleshooting

- If the API reports that OpenRGB is unavailable or returns `502`, confirm OpenRGB is running, its SDK server is enabled, and `host` and `port` in the configuration are correct. The service starts even when OpenRGB is not running, and keeps retrying the connection in the background with an increasing delay, so no restart is needed once OpenRGB is up.
- If a request returns `401`, send `Authorization: Bearer TOKEN` with the token returned by pairing. If a paired phone stops working, it may have been revoked; pair it again.
- If pairing is rejected, codes last five minutes, work once, and are invalidated after five wrong attempts. Generate a new one from the control panel. To revoke every paired device at once, stop the service and delete `paired-devices.json`.
- If the control panel returns `403`, open it from the computer running the service using `localhost` or `127.0.0.1`.
- If a phone cannot connect, confirm `listen_address` is not `127.0.0.1` and that the phone is on the same trusted network.
- If a device request returns `404`, use the controller `id` from `GET /api/controllers`.
- If direct color changes do not appear, use `/api/device/color`, which switches the device to custom mode first.
- Check `openrgb-api.log` beside the configuration file for application errors.

See [documentation/setup.md](documentation/setup.md) for installer and Docker-specific troubleshooting.

## Status

🚧 Work in progress

## Copyright

Copyright © 2026 sb0-0dev

Licensed under GPL-2.0-or-later. See [LICENSE](LICENSE).

## Warranty

This software is provided "as is", without warranty of any kind, express or implied, including but not limited to the warranties of merchantability, fitness for a particular purpose and noninfringement. In no event shall the authors or copyright holders be liable for any claim, damages or other liability, whether in an action of contract, tort or otherwise, arising from, out of or in connection with the software or the use or other dealings in the software.

## Acknowledgements

This project depends on communication with OpenRGB, an open-source RGB lighting control application.
Thanks to Adam Honse and the OpenRGB development team for developing and maintaining the OpenRGB project.
