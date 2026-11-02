# Installers

OpenRGB API has installers for Windows and Linux, plus a Docker image. The Windows and Linux installers install the executable as an automatically starting service with a default configuration. This page covers the [Windows installer](#windows-installer), the [Linux installer](#linux-installer) and [Docker](#docker). Developers can build all of them from source: see [Building from source](#building-from-source).

## Shared setup

All three deployment options connect to the OpenRGB SDK server, which must be running. Its default address is `127.0.0.1:6742`; native installs can use that default. Docker networking is different because `127.0.0.1` inside a container refers to the container, so follow [Connecting to OpenRGB from the container](#connecting-to-openrgb-from-the-container).

The API listener (default port `8742`) is for paired clients and requires a bearer token. The control panel (default port `8743`) is for settings and pairing and should stay local to the host. Pair a phone by generating a code in the control panel and scanning its QR code; consult the [README security notes](../README.md) before exposing the API to a network. Docker's published-port requirements are covered in [Ports, pairing and local access](#ports-pairing-and-local-access).

# Windows Installer

OpenRGB API ships a Windows installer built with [Inno Setup 6](https://jrsoftware.org/isinfo.php). It installs the executable, writes a default configuration file, registers a Windows service that starts automatically, and optionally opens a firewall rule for private-network access.

## Installing

1. Make sure OpenRGB is installed and its SDK server is enabled (OpenRGB > SDK Server > Start Server); see [Shared setup](#shared-setup).
2. Run `OpenRGB-API-Setup-<version>.exe` as an administrator. The installer requires elevation because it registers a service.
3. On the Tasks page, choose whether to **Allow mobile and other devices on private networks to connect**.
   - **Unticked (default):** the service is reachable only from the same computer.
   - **Ticked:** the installer adds a Windows Firewall rule allowing inbound TCP port 8742 on the **Private** network profile only. It does not open the port on Public or Domain networks.
4. Finish the wizard. The service is registered and started automatically.
5. Open the Start menu shortcut **OpenRGB API**, or browse to `http://127.0.0.1:8743` (the control panel).

To connect a phone, follow the pairing flow in [Shared setup](#shared-setup).

## What gets installed

| Item | Location |
| --- | --- |
| Executable | `C:\Program Files\OpenRGB API\openrgb-api.exe` |
| Configuration | `C:\ProgramData\OpenRGB API\config.toml` |
| Paired devices | `C:\ProgramData\OpenRGB API\paired-devices.json`, created on first pairing, next to the configuration file |
| Windows service | Name `openrgb-api`, display name "OpenRGB API", startup type Automatic (delayed) |
| Firewall rule | `OpenRGB API (Private)`, only if network access was selected |
| Start menu shortcut | Opens the control panel at `http://127.0.0.1:8743` |

The service is configured to restart automatically five seconds after a failure.

## Configuration

The installer writes one of two starter files, only if no `config.toml` exists yet. Existing configuration is never overwritten, and it is kept when you uninstall.

```toml
[server]
listen_address = '127.0.0.1:8742'
admin_address = '127.0.0.1:8743'
# public_url = 'http://192.168.1.10:8742'
host = '127.0.0.1'
port = '6742'
client_name = 'openrgb-api'
```

- `listen_address` is the address of the API used by paired phones. `127.0.0.1:8742` accepts connections from the same computer only. The network-access option installs a configuration with `0.0.0.0:8742`, which accepts connections from other devices (the firewall rule still limits this to Private networks). Every request needs a bearer token from pairing, and this address serves no web page. To use a specific network address or another port, edit the file and update the firewall rule to match.
- `admin_address` is where the control panel (settings and pairing) is served. Keep it on `127.0.0.1`: only this computer can open it, and no firewall rule is needed. It must differ from `listen_address` and cannot be empty.
- `public_url` (optional) is the address remote devices use, which is encoded in the pairing QR code. When it is empty the server uses your computer's private network addresses if `listen_address` is a wildcard such as `0.0.0.0`, or `listen_address` itself if it is a specific address. With `127.0.0.1` there is no QR code because a phone cannot reach it. If several addresses are found, the control panel lets you choose which one the QR code uses.
- `host` and `port` locate the OpenRGB SDK server (default `127.0.0.1:6742`).

Edit the file, or use the settings form in the web interface, then restart the service for the change to apply:

```powershell
openrgb-api.exe -config "C:\ProgramData\OpenRGB API\config.toml" restart
```

## Managing the service

The executable doubles as a service manager. Run these from an elevated prompt:

```powershell
openrgb-api.exe -config "C:\ProgramData\OpenRGB API\config.toml" status      # running, stopped, or unknown
openrgb-api.exe -config "C:\ProgramData\OpenRGB API\config.toml" start
openrgb-api.exe -config "C:\ProgramData\OpenRGB API\config.toml" stop
openrgb-api.exe -config "C:\ProgramData\OpenRGB API\config.toml" restart
openrgb-api.exe -config "C:\ProgramData\OpenRGB API\config.toml" install     # register the service
openrgb-api.exe -config "C:\ProgramData\OpenRGB API\config.toml" uninstall   # stop and remove the service
```

The service can also be managed from `services.msc`. Optional flags `-listen`, `-host`, and `-port` override individual settings and are stored in the service definition when you run `install`.

## Upgrading

Run a newer installer over an existing installation. Before copying files it stops and removes the old service and the old firewall rule, then registers and starts the new service. Your configuration and paired devices are preserved. Existing configurations are not rewritten: before upgrading from the former defaults, change `listen_address` to `127.0.0.1:8742` (or `0.0.0.0:8742` for LAN access), `admin_address` to `127.0.0.1:8743`, and update `public_url` to use port `8742` if set. Otherwise the upgraded service keeps listening on the old ports while the new shortcut and firewall rule use the new ports. Restart the service after changing the configuration.

## Uninstalling

Use **Settings > Apps > Installed apps** or the Start menu uninstaller. It stops and removes the service and deletes the firewall rule.

`C:\ProgramData\OpenRGB API` is intentionally left in place so a reinstall keeps your settings and paired phones. Delete that folder manually to remove them. Deleting `paired-devices.json` revokes every paired device.

## Building the installer

Requirements: Go 1.26.1 or newer and Inno Setup 6.

```powershell
.\scripts\build-windows-installer.ps1 -Version x.x.x
```

The script finds `ISCC.exe` (on `PATH`, or in a standard Inno Setup 6 location), builds the executable into a temporary `build` folder (deleted afterwards), then compiles [build/openrgb-api.iss](../build/openrgb-api.iss). The installer is written to `release\OpenRGB-API-Setup-<version>.exe`. `-Version` must be in `major.minor.patch` form.

The two starter configuration files live in `build/`: `config-local.toml` (local only) and `config-lan.toml` (network access).

## Troubleshooting

| Symptom | Check |
| --- | --- |
| The web page does not load | Run `status`. If stopped, run `start` and check the Windows Event Viewer for service errors. |
| Page loads but shows "OpenRGB unavailable" | Start the OpenRGB SDK server and confirm `host` and `port` in `config.toml`. The service reconnects by itself within about 30 seconds; no restart is needed. |
| Phone cannot reach the server | Confirm `listen_address` is not `127.0.0.1`, the network profile is **Private**, and the phone is on the same network. |
| Pairing code rejected | Codes last five minutes, work once, and are invalidated after five wrong attempts (anyone who can reach the API port can use up the active code). Generate a new one from the control panel. To revoke every device, stop the service and delete `paired-devices.json`. |
| Port 8742 already in use | Change `listen_address` to a free port, update the firewall rule to match, and restart the service. |
| Setup logs | The installer writes a setup log (`SetupLogging=yes`) to your temp folder, named `Setup Log <date>.txt`. |

# Linux Installer

The Linux installer is a `.tar.gz` archive containing the executable, a hardened systemd unit and two scripts. It requires a systemd-based distribution and root access.

## Install

```sh
tar -xzf openrgb-api-<version>-linux-amd64.tar.gz
cd openrgb-api-<version>-linux-amd64
sudo sh install.sh          # local access only (127.0.0.1:8742)
sudo sh install.sh --lan    # allow other devices (0.0.0.0:8742)
```

The script creates a dedicated `openrgb-api` system user, installs the executable to `/usr/local/bin/openrgb-api`, writes the configuration to `/etc/openrgb-api/config.toml` (mode 0600), and enables and starts the `openrgb-api` service.

- An existing configuration is never overwritten, so upgrades keep your settings and paired devices. `--lan` only applies to a fresh install. Existing configurations are not automatically moved from the former ports; change `listen_address` to `127.0.0.1:8742` or `0.0.0.0:8742`, `admin_address` to `127.0.0.1:8743`, and the `public_url` port to `8742` if configured, then restart the service.
- The script does not change your firewall. With `--lan`, allow TCP 8742 from your trusted network only, for example `ufw allow from 192.168.1.0/24 to any port 8742 proto tcp`.

## Manage the service

```sh
systemctl status openrgb-api
sudo systemctl restart openrgb-api
journalctl -u openrgb-api
```

## Uninstall

```sh
sudo sh uninstall.sh          # keeps /etc/openrgb-api
sudo sh uninstall.sh --purge  # also removes configuration, paired devices and the service user
```

## Build the archive

From a Windows or Linux PowerShell session at the repository root:

```powershell
.\scripts\build-linux-installer.ps1 -Version 0.1.0 -Architecture amd64   # or arm64
```

The archive is written to `release\openrgb-api-<version>-linux-<arch>.tar.gz`.

# Docker

The [Dockerfile](../Dockerfile) builds a small (about 8 MB) image containing only the executable. Configuration and paired devices are stored in the `/data` volume.

The OpenRGB SDK server must be running on the Docker host. The container reaches it at `host.docker.internal:6742`; see [Connecting to OpenRGB from the container](#connecting-to-openrgb-from-the-container). Override with `-host` and `-port` arguments. The container exits if the SDK server is unreachable, so use a restart policy.

```sh
docker compose up -d --build
```

Or manually:

```sh
docker build -t openrgb-api .
docker run -d --name openrgb-api --restart unless-stopped \
  --add-host host.docker.internal:host-gateway \
  -p 8742:8742 \
  -p 127.0.0.1:8743:8743 \
  -e OPENRGB_API_LOCAL_NETWORKS=172.16.0.0/12 \
  -e OPENRGB_API_PUBLIC_URL=http://<host-ip>:8742 \
  -v openrgb-api-data:/data openrgb-api
```

The image downloads the public [`openrgb-golang` v0.0.2](https://github.com/sb0-0dev/openrgb-golang/releases/tag/v0.0.2) Go module during the build. No GitHub token or repository credentials are needed.

## Ports, pairing and local access

The listener roles and pairing flow are described in [Shared setup](#shared-setup). In Docker, publish the ports as follows:

The container runs two listeners, so the pairing interface never has to share a port with the network:

| Port | Purpose | Publish it on |
| --- | --- | --- |
| 8742 | API for paired phones. Every request needs a bearer token and it serves no web page. | Your choice: `127.0.0.1` (this computer only) or `0.0.0.0` (your network). Set `OPENRGB_API_BIND`. |
| 8743 | Control panel, settings and pairing. | **Always `127.0.0.1`.** Never publish it on the network. |

For phone pairing, set `OPENRGB_API_PUBLIC_URL` to the address the phone will use (for example `http://192.168.1.183:8742`), then open `http://localhost:8743` on the Docker host and generate a code. A container cannot detect the host's LAN address, so without this setting you get a code but no QR code.

- Both listeners bind to `0.0.0.0` inside the container so Docker can forward to them. **Leave the API address and control panel address at `0.0.0.0` in the settings page** (the image sets them, and the control panel address cannot be edited away with a saved value). Changing either to `127.0.0.1` inside the container stops Docker from reaching it. Whether a port is public or private is decided only by the published address in `compose.yaml`, which is why 8743 must be published on `127.0.0.1`.
- The control panel also rejects any request whose `Host` header is not `localhost` or a loopback address, and any cross-origin request. Behind Docker's port mapping the host browser appears to come from the Docker network gateway, so `OPENRGB_API_LOCAL_NETWORKS` (comma-separated CIDRs) tells the server to treat those addresses as local.
- Publishing 8742 on your network (`OPENRGB_API_BIND=0.0.0.0`, the `compose.yaml` default) is needed for phones to connect. Set `OPENRGB_API_BIND=127.0.0.1` if you do not want that.
- On Linux hosts, `--network host` is an alternative: leave `OPENRGB_API_LOCAL_NETWORKS` unset and the app detects your LAN addresses itself.

## Connecting to OpenRGB from the container

Inside the container, `127.0.0.1` is the container itself, so the web app connects to `host.docker.internal:6742`. That connection arrives at your computer on its network address, not on loopback. OpenRGB's SDK server listens on `127.0.0.1` by default, so it refuses the connection unless you change that.

1. In OpenRGB, open the **SDK Server** tab and click **Start Server** (or launch OpenRGB with `--server`).
2. Make it listen on all interfaces, for example `--server-host 0.0.0.0`, or the equivalent option in your version.
3. Check the binding with `netstat -ano | findstr 6742` on Windows or `ss -ltn | grep 6742` on Linux. `127.0.0.1:6742` is not reachable from a container. `0.0.0.0:6742` is.
4. Restrict the port in the firewall to the Docker network. The OpenRGB SDK has no authentication, so do not allow it from the wider network or the internet.

If you would rather not open the SDK port, run the web app natively (the Windows installer or Linux archive), or on Linux use `docker run --network host` with `-host 127.0.0.1`.

## Docker troubleshooting

| Symptom | Cause and fix |
| --- | --- |
| Log shows `connect to OpenRGB: failed to connect to OpenRGB server at host.docker.internal:6742 ... connection refused` | The SDK server is not running, or only listens on `127.0.0.1`. See the section above. The container keeps running and retries in the background, so the API returns `502` until the connection succeeds. |
| Same error with `i/o timeout` | A firewall is dropping the connection. Allow TCP 6742 from the Docker network. |
| `no such host` for `host.docker.internal` | Linux hosts need `--add-host host.docker.internal:host-gateway` (already in `compose.yaml`). |
| `go mod download` fails during build | Check that Docker can reach the Go module proxy and GitHub, then rebuild with `docker compose build --no-cache`. |
| Control panel at `http://localhost:8743` refused or 403 | Publish port 8743 on `127.0.0.1`, set `OPENRGB_API_LOCAL_NETWORKS` to include the Docker gateway range (`172.16.0.0/12` covers the defaults), and browse using `localhost`. A LAN address or hostname is deliberately rejected. |
| `bearer token required` on port 8742 | Expected. The API port always needs a token, even from this computer. Use the control panel on port 8743 for the web page and pairing. |
| Pairing shows a code but no QR code | The container cannot detect your LAN address. Set `OPENRGB_API_PUBLIC_URL` (for example `http://192.168.1.183:8742`). |
| Settings page shows `0.0.0.0` for the API or control panel address | Expected in Docker. Leave both at `0.0.0.0`. The `-listen` flag and `OPENRGB_API_ADMIN_ADDRESS` override the saved values, so editing them in the page has no effect. It does not expose your LAN: only the published addresses in `compose.yaml` decide who can connect. |
| Settings are lost after recreating the container | Mount a volume at `/data`. Without it the configuration and paired devices are discarded with the container. |
| Phone cannot connect | Port 8742 is published on `127.0.0.1` only. Publish it with `OPENRGB_API_BIND=0.0.0.0` after reading the security notes. |

# Building from source

This section is for developers who want to build the executable or an installer themselves. End users should use a prebuilt installer.

## Prerequisites

| Tool | Needed for |
| --- | --- |
| Go 1.26.1 or newer (see `go.mod`) | Every build |
| Internet access to download public Go modules, including `github.com/sb0-0dev/openrgb-golang` v0.0.2 | First build |
| [Inno Setup 6](https://jrsoftware.org/isinfo.php) | The Windows installer only |
| PowerShell 5.1 or newer, and `tar` | The build scripts. `tar` is included with Windows 10 and later |
| Docker with BuildKit | The container image only |

The scripts are PowerShell and run from the repository root. You can build the Linux archive from Windows because it is a cross-compile, so no Linux machine is needed.

## Getting the SDK dependency

The module depends on the public `github.com/sb0-0dev/openrgb-golang` SDK at v0.0.2. Download dependencies with:

```sh
go mod download
```

To build against a local checkout of the SDK while changing both projects, use a temporary `replace` directive in a copied module file:

```powershell
Copy-Item go.mod "$env:TEMP\local.mod"; Copy-Item go.sum "$env:TEMP\local.sum"
Add-Content "$env:TEMP\local.mod" "`nreplace github.com/sb0-0dev/openrgb-golang => C:/path/to/openrgb-go"
go test -modfile "$env:TEMP\local.mod" ./...
```

Do not commit a `replace` directive.

## Run and test

```sh
go run ./cmd/openrgb-api      # control panel at http://127.0.0.1:8743
go vet ./...
go test ./...
go test -cover ./...
```

`go run` uses the per-user configuration file (`openrgb-api/config.toml` under your user config directory). Pass `-config ./dev.toml` to use a separate file, and `-host` and `-port` to point at a different OpenRGB SDK server. Running it directly (not as a service) needs no administrator rights.

## Build the executable only

```powershell
go build -trimpath -o release\openrgb-api.exe .\cmd\openrgb-api
```

The web interface under `cmd/openrgb-api/web` is embedded in the executable, so no separate asset step is needed. On Windows the executable also manages its own service (`install`, `start`, `stop`, `restart`, `uninstall`, `status`), which is useful for testing without an installer. See [Managing the service](#windows-installer) above and run those commands from an elevated prompt. On Linux the service is managed by systemd instead.

## Build the installers

| Target | Command | Output |
| --- | --- | --- |
| Windows | `.\scripts\build-windows-installer.ps1 -Version 0.2.0` | `release\OpenRGB-API-Setup-0.2.0.exe` |
| Linux | `pwsh ./scripts/build-linux-installer.ps1 -Version 0.2.0 -Architecture amd64` | `release/openrgb-api-0.2.0-linux-amd64.tar.gz` |
| Docker | `docker compose build` | local image `openrgb-api` |

- `-Version` must be `major.minor.patch`. It sets the version shown by the installer and the archive name. It is not compiled into the executable.
- The Windows script builds `openrgb-api.exe` in a temporary `build\` folder, locates `ISCC.exe` (on `PATH` or in a standard Inno Setup 6 folder), and compiles [build/openrgb-api.iss](../build/openrgb-api.iss) with the version.
- The Linux script cross-compiles a static binary (`CGO_ENABLED=0`), copies in `install.sh`, `uninstall.sh`, `openrgb-api.service` and the two starter configs, converts the shell scripts to LF line endings, writes the archive. Use `-Architecture arm64` for Raspberry Pi class devices. It restores your `GOOS`, `GOARCH` and `CGO_ENABLED` settings afterwards.
- The `release/` folder is gitignored. Don't commit installers, since each one is added to the repository history permanently. Publish them as GitHub release assets instead.

For a quick run without installing, `go run ./cmd/openrgb-api -config ./dev.toml` works the same on Linux. Docker builds are identical on every platform.

## Where things live

| Path | Contents |
| --- | --- |
| `cmd/openrgb-api/` | Application source, tests and the embedded `web/` interface |
| `build/openrgb-api.iss` | Windows installer script (service, firewall rule, upgrade logic) |
| `build/config-local.toml`, `build/config-lan.toml` | Starter configurations used by both installers |
| `build/linux/` | `install.sh`, `uninstall.sh` and the systemd unit |
| `scripts/` | Installer build scripts |
| `Dockerfile`, `compose.yaml` | Container image and example deployment |

## Checking a build

- **Windows:** run the installer on a clean machine or VM, then confirm the **OpenRGB API** service is running and `http://127.0.0.1:8743` opens. Uninstall and check the service is removed and `C:\ProgramData\OpenRGB API` is kept.
- **Linux:** extract the archive, run `sudo sh install.sh`, and check `systemctl status openrgb-api`. Syntax-check the scripts with `sh -n install.sh uninstall.sh`.
- **Both:** pair a phone or run `curl -X POST http://127.0.0.1:8743/api/pairing/start` and confirm a code is returned.
