# secon

```sh
brew install --cask mikuta0407/apps/secon   # macOS: secon.app + secon command
brew install mikuta0407/apps/secon          # Linux: secon command
```

A SoftEther VPN compatible client for macOS and Linux, written in Go.
Connect as a virtual NIC, or keep everything inside the app and expose it as a SOCKS5 proxy.

[日本語版 README はこちら](README.ja.md)

![Connection manager](docs/images/en/manager.png)

## Quick start (about 5 minutes)

1. Install: `brew install --cask mikuta0407/apps/secon` (macOS, Apple Silicon) or `brew install mikuta0407/apps/secon` (Linux)
2. Register the daemon: `sudo secon service install`
3. Add a profile: open **secon** from Launchpad (Linux: run `secon-gui`), click **New** in the connection manager
4. Connect: click **Connect**, or run `secon connect <profile>`
5. Check: `secon status <profile>` shows the IP address, gateway and traffic

Without the GUI, edit `/etc/secon/config.toml` in step 3 (see [Configuration](#configuration)) and run `secon reload`.

## Features

| Feature | What it does |
|---|---|
| NIC mode | Creates a virtual NIC (Linux: TAP, macOS: utun). Gets an address by DHCP or uses a static one. Works like a normal L2 VPN (tested with SSH, HTTP, SMB) |
| SOCKS5 mode | No virtual NIC, no root. The TCP/IP stack runs inside secon and listens as a SOCKS5 proxy |
| Port forwarding | Listens on a local port and forwards to a host behind the VPN. Works in both modes (for apps without SOCKS support, such as RDP clients) |
| Routing | Adds static routes to the VPN side, optionally makes the VPN the default gateway, and sets the VPN DNS |
| HTTP proxy | Connects to the VPN server through an HTTP proxy (`CONNECT`) |

### Which mode should I use?

| Use | Pick |
|---|---|
| Every app should reach the VPN network | `nic` (needs the root daemon) |
| Only browsers or tools that support SOCKS5 | `socks` (no root, no change to OS routes) |
| One app without SOCKS support, such as RDP | either mode + a port forward |

## GUI

`secon-gui` lives in the menu bar (macOS) or the system tray (Linux). It only talks to the daemon, so start the daemon first (`sudo secon service install`).

- macOS: `secon.app` in `/Applications` (installed by the Homebrew Cask). Open it from Launchpad or Finder; it opens the connection manager. Opening it again while it runs brings the connection manager back.
- Start at login: tray menu → **Start at Login** (macOS: a LaunchAgent; Linux: `~/.config/autostart`). At login it stays in the tray without opening a window.
- Quit: **Quit** closes only the GUI; the daemon and its connections keep running. The tray also has **Stop Daemon** / **Start Daemon** and **Quit and Stop Daemon**. A stopped daemon runs again at the next login (system daemon: next boot). Starting the system daemon asks for an administrator password.
- Linux: build it from source (see [Development](#development)). The tray needs a StatusNotifierItem host (on GNOME: the AppIndicator extension). To list it in the app menu: `install -Dm644 packaging/linux/secon-gui.desktop ~/.local/share/applications/secon-gui.desktop` and `install -Dm644 packaging/linux/secon.png ~/.local/share/icons/hicolor/256x256/apps/secon.png`.
- Language: English or Japanese, chosen from the OS language. Change it from the tray menu: **Language / 言語** → Automatic / English / 日本語.

![Tray menu](docs/images/en/tray.png)

### Connect or disconnect

1. Click the tray icon (green = at least one profile connected)
2. Hover a profile
3. Click **Connect** or **Disconnect**

### Connection manager

Tray menu → **Connection Manager…**.

| Area | Shows |
|---|---|
| Toolbar | Connect / Disconnect / New / Properties / Delete for the selected profile. The same actions are in the right-click menu of each row; double-click opens Properties |
| Table | State, mode, server, hub, IP address, received, sent, uptime (refreshed every 2 seconds) |
| Bottom pane | Gateway, DNS, virtual NIC, SOCKS5 address, forwards, session, server build, packet counts |

### Create or edit a profile

1. In the connection manager, click **New**, or select a profile and click **Properties**
2. Fill in server (`host:port`), virtual hub, user name and password (empty password = anonymous)
3. Choose the mode and the IP address source (**DHCP** or **Static**). Only the fields you need are shown
4. Click **Save**. The daemon rewrites the config file and applies it immediately

![Profile properties](docs/images/en/properties.png)

The properties window of an existing profile also has a Connect/Disconnect button and a **Connection info** tab.

## CLI

| Command | What it does |
|---|---|
| `secon status [profile]` | List profiles, or show details of one (address, gateway, DNS, traffic) |
| `secon connect <profile>` | Connect and wait until connected (60 s timeout) |
| `secon disconnect <profile>` | Disconnect |
| `secon reload` | Re-read the config file (same as `SIGHUP`) |
| `secon service install` / `uninstall` | Register / remove the daemon (launchd or systemd) |

Other commands: `secon version`, `secon daemon` (run in the foreground), `secon debug dump|socks` (connect without the daemon, for troubleshooting).

The CLI follows the OS language. Force it with `SECON_LANG=en` or `SECON_LANG=ja`.

```console
$ secon status
NAME          MODE   STATE         ADDRESS         SERVER
office        nic    connected     10.99.0.137/24  vpn.example.com:443
office-socks  socks  connected     10.99.0.190/24  vpn.example.com:443
datacenter    nic    disconnected  -               vpn.example.com:443
```

### Daemon and permissions

| How you install | Command | Can use |
|---|---|---|
| System daemon (recommended) | `sudo secon service install` | NIC and SOCKS modes |
| User daemon | `secon service install --user` | SOCKS mode only |

- macOS: users in the `admin` group can run `secon` without `sudo`.
- Linux: the first existing group of `secon`, `sudo`, `wheel`. To use a dedicated group: `sudo groupadd secon && sudo usermod -aG secon $USER`, then log in again.
- Logs: `/var/log/secon.log` (macOS), `journalctl -u secon` (Linux). User daemon on macOS: `~/Library/Logs/secon.log`.

## Configuration

`/etc/secon/config.toml` for the system daemon. The user daemon uses `~/.config/secon/config.toml` (Linux) or `~/Library/Application Support/secon/config.toml` (macOS). `secon service install` creates a commented sample.

```toml
[[profile]]
name = "office"
server = "vpn.example.com:443"
hub = "VPN"
user = "alice"
password = "secret"          # empty = anonymous
mode = "nic"                 # "nic" or "socks"
auto_connect = true
cert_sha256 = "0e84ded0..."  # pin a self-signed server certificate
# proxy = "http://user:pass@proxy.example.com:8080"

[profile.static]             # omit this table to use DHCP
address = "10.20.0.50/24"
gateway = "10.20.0.1"
dns = ["10.20.0.1"]

[profile.nic]
routes = ["10.20.0.0/16"]    # send only these networks through the VPN
default_gateway = false      # true = send all traffic through the VPN
dns = true                   # use the VPN's DNS server
dns_domains = ["corp.example"]

[[profile.forward]]
listen = "127.0.0.1:13389"
target = "10.20.0.5:3389"
```

For a SOCKS5 profile, set `mode = "socks"` and use `[profile.socks]` with `listen = "127.0.0.1:1080"` (optional `username` / `password`).

### Static IP address

By default the address, gateway and DNS come from DHCP on the VPN side. Set `[profile.static]` (or choose **Static** in the GUI) to skip DHCP:

- `address` (required): IPv4 address with prefix length, e.g. `10.20.0.50/24`
- `gateway` (optional): must be inside that subnet; needed for `routes` and `default_gateway`
- `dns` (optional): DNS servers, used by SOCKS5 name resolution and by `nic.dns`

It works in both NIC and SOCKS modes.

### Server certificate

1. Default: verified against the OS trust store
2. Self-signed: run `secon debug dump -server vpn.example.com:443 -insecure -duration 1s`, copy the `cert sha256=` value into `cert_sha256` (an authentication error after that line is expected)
3. `insecure_skip_verify = true` disables verification (testing only)

### Routes and the default gateway (NIC mode)

- The VPN subnet (from DHCP or `static.address`) is reachable without any setting.
- `routes`: networks behind the VPN router. They go to the gateway (DHCP router or `static.gateway`); everything else uses your normal connection.
- `default_gateway = true`: all traffic goes through the VPN. The route to the VPN server itself stays on the original gateway.
- Routes and DNS settings are removed on disconnect.

## Limitations

- Authentication: password and anonymous only (no certificate or RADIUS authentication yet)
- One TCP connection per session; no UDP acceleration
- IPv4 only
- Tested against SoftEther VPN Server 5.01

## Uninstall

1. `sudo secon service uninstall`
2. `brew uninstall --cask secon` (macOS) or `brew uninstall secon` (Linux). `brew uninstall --zap --cask secon` also removes GUI settings and the login item
3. Optional: `sudo rm -r /etc/secon`

## Development

- `docs/DESIGN.md`: architecture
- `docs/PROTOCOL.md`: SoftEther protocol notes
- `dev/README.md`: VM-based test environment (Lima + Tart)

Build: `go build ./cmd/secon` (pure Go) and `go build ./cmd/secon-gui` (needs cgo; on Linux also X11/OpenGL headers). macOS app bundle: `packaging/macos/build-app.sh secon-gui secon <version> <outdir>`.

## License

Apache License 2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).

secon is an independent implementation of the SoftEther VPN protocol and contains no SoftEther VPN source code. It is not affiliated with the SoftEther VPN Project.
