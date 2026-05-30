---
title: "Desktop App"
description: "Download, install, and run the StacyVM desktop app on Linux, macOS, and Windows."
---

The **StacyVM desktop app** packages the web dashboard and runs the StacyVM API
daemon **in-process** — one window, no separate `stacyvm serve` needed. It is a
[Wails](https://wails.io) application and is distributed as a native installer
for each operating system.

<Note>
The desktop app bundles the UI and the API, but it **still needs a sandbox
provider on the host** to actually run sandboxes. Install and start **Docker**
(the default provider) before spawning anything. See
[Prerequisites](/docs/getting-started/prerequisites).
</Note>

This is a **separate download** from the [CLI installer](/docs/getting-started/installation)
(`npx stacyvm-setup`). The CLI builds StacyVM from source; the desktop app is a
prebuilt, double-click application.

## Download

Grab the latest installer for your OS from the
[**Releases page**](https://github.com/StacyOS/stacyvm/releases/latest):

| OS | Download | Notes |
|---|---|---|
| **Windows** | `StacyVM-amd64-installer.exe` | NSIS installer; bootstraps the WebView2 runtime if missing. |
| **macOS** | `StacyVM-macos-universal.dmg` | Universal — runs on Apple Silicon and Intel. |
| **Linux** | `StacyVM-x86_64.AppImage` | Self-contained; no system libraries required. |
| **Linux (alt)** | `StacyVM-linux-amd64.tar.gz` | Bare binary; requires `libwebkit2gtk-4.1` on the host. |

## Install & run

### Windows
1. Run `StacyVM-amd64-installer.exe`.
2. If **SmartScreen** warns that the publisher is unrecognized, click
   **More info → Run anyway** (the app is not yet code-signed).
3. Launch **StacyVM** from the Start menu.

### macOS
1. Open `StacyVM-macos-universal.dmg` and drag **StacyVM** into **Applications**.
2. On first launch, because the app is not yet notarized, macOS may say it is
   from an "unidentified developer." **Right-click (Control-click) the app →
   Open → Open.** Alternatively, clear the quarantine flag:
   ```bash
   xattr -dr com.apple.quarantine /Applications/StacyVM.app
   ```

### Linux
**AppImage (recommended):**
```bash
chmod +x StacyVM-x86_64.AppImage
./StacyVM-x86_64.AppImage
```
**Tarball:** extract and run the binary. It dynamically links the host's
WebKitGTK, so install it first if missing:
```bash
# Debian/Ubuntu
sudo apt-get install -y libwebkit2gtk-4.1-0
tar -xzf StacyVM-linux-amd64.tar.gz
./StacyVM
```

When the window opens, the embedded API daemon starts automatically and the
dashboard loads. With Docker running, you can spawn, exec into, and destroy
sandboxes directly from the UI.

## Build from source

Requires the [Wails CLI](https://wails.io/docs/gettingstarted/installation), Go,
and Node.js. On **Linux** you also need `libgtk-3-dev` and
`libwebkit2gtk-4.1-dev`.

```bash
make build-desktop
```

The frontend is installed and built automatically (via the
`frontend:install`/`frontend:build` steps in `desktop/wails.json`), then Wails
compiles the app. Output: `desktop/build/bin/StacyVM` (`.app` on macOS, `.exe`
on Windows). The `make` target auto-applies the Linux-only `-tags webkit2_41`
build tag; on macOS/Windows it builds without it.
