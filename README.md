# DDEV Manager for Firefox

A Firefox toolbar extension that groups DDEV projects by task, opens their surfaces and services, and starts, restarts, or stops individual surfaces or entire tasks.

This is an independent community project maintained by Cortier. It is not affiliated with or endorsed by the DDEV Foundation. DDEV and its logo are trademarks of their respective owners.

The popup follows Firefox Multi-Account Containers’ compact, native-looking presentation, with custom components, light/dark themes, task search, status indicators, and keyboard-accessible controls. No Mozilla source or assets are bundled.

## Requirements

- Desktop Firefox 142 or later.
- DDEV 1.25.4 or later, its supported Docker environment, and Git.
- Linux with a running **systemd user manager**, or macOS with a graphical login session and LaunchAgents.
- Linux/macOS on Intel/AMD 64-bit or ARM64. Windows/WSL and sandboxed Firefox packages such as Snap/Flatpak are not supported by the installer.

## Installation

The extension and companion are installed separately. The companion runs DDEV commands; its small login service handles cleanup when Firefox removes the extension.

1. Extract the companion archive matching your operating system and architecture.
2. In that extracted directory, run:

    ```sh
    ./install.sh
    ```

    To specify executable locations:

    ```sh
    ./install.sh --ddev /absolute/path/to/ddev --git /absolute/path/to/git
    ```

3. Install the **Mozilla-signed** extension `.xpi` using Firefox → Add-ons and themes → gear menu → Install Add-on From File.
4. Open DDEV Manager. It discovers registered projects automatically. Settings → Check connection reports companion and cleanup-service availability.

The repository’s packaged ZIP is **unsigned**. It is suitable for temporary developer loading, not permanent installation in normal Firefox. Install permanent builds from the Mozilla-signed XPI attached to a GitHub release.

Install the companion once per OS user. Each Firefox profile registers independently. Re-running the installer upgrades the binary without changing profile registrations or their uninstall URLs. Run the installer from your usual development shell: it captures that shell’s PATH for DDEV custom commands. Re-run it after changing the toolchain location.

### Developing locally

```sh
npm ci
npm run build
(cd companion && go build -o ../dist/ddev-manager .)
./dist/ddev-manager install
```

Open `about:debugging#/runtime/this-firefox`, choose **Load Temporary Add-on**, and select `dist/extension/manifest.json`. Alternatively, use `npx web-ext run --source-dir dist/extension` with your installed Firefox. Temporary add-ons disappear after Firefox restarts and are only intended for development. Use a dedicated development profile.

The native host name is `com.cortier.ddev_manager`, and the fixed extension ID is `ddev-manager@cortier.com`.

## Tasks, surfaces, and services

- Identical full task branches group together across repositories. `feat/inventory-sync` is displayed as `inventory-sync`; `fix/inventory-sync` stays a separate task.
- `main`, `master`, `develop`, `development`, and `staging` group by product, inferred from the canonical repository directory. Known suffixes are `api`, `app`, `angular`, `admin`, `shop`, `docs`, `checklist`, and `laravel` (displayed as API).
- Detached and non-Git projects remain individual tasks. Missing project directories stay visible with an explanatory status.
- Settings provides JSON naming overrides keyed by DDEV project name, for example:

    ```json
    {
        "example-api": { "product": "Example", "surface": "API" }
    }
    ```

- Task branches group by branch regardless of product overrides. Search retains all surfaces of a matching task.
- Clicking a surface starts it if needed and opens its URL in a new Firefox tab. Existing `ddev url` commands are preferred, with the DDEV primary URL as fallback. The literal `ddev launch` command is not executed because it can open a different default browser.
- Browser services are discovered from `ddev describe` and effective `web_extra_exposed_ports`. The menu is limited to the curated browser-facing services Buggregator, webhook.site, and Storybook; internal HTTP endpoints such as Ministack, Reverb, and Vite are excluded. Buggregator and Storybook use project or global URL commands when present. A service may still be initializing after DDEV reports the project running; its independent readiness is not guaranteed.
- Start all skips running surfaces. Restart all starts stopped/paused surfaces and restarts running ones. Stop all skips stopped surfaces. Batches run serially and continue after failures.
- Commands continue when the popup closes. Firefox itself must remain running. If Firefox exits or the native connection fails during a command, inspect DDEV status before retrying; there is no persistent command daemon or automatic retry.

## Automatic and manual removal

Firefox stores a profile-specific HTTP uninstall URL. On extension removal, it opens a local page served only at `127.0.0.1`. A secret in the URL fragment authenticates a same-origin POST; merely requesting the page cannot delete files. No external resources are loaded, and secrets are not written to request logs.

The cleanup service unregisters that profile. If others remain registered, it stays installed. Otherwise, an independent short-lived helper waits for current DDEV commands, removes the startup registration and installer-owned files, and exits. New lifecycle commands are rejected once final cleanup begins. The page reports that cleanup has **started**, rather than claiming completion before the helper finishes.

**Automatic cleanup is best-effort.** It requires Firefox to open the uninstall page and the service to be running on its registered port. Closing Firefox, disabling the extension, an idle profile, or a dropped native connection never triggers removal. Deleted profiles may leave stale registrations; the service deliberately does not guess that inactivity means uninstall.

Manual removal is always available from an extracted companion release:

```sh
./uninstall.sh
```

This removes the companion for **all** profiles. It leaves Firefox extensions installed, and never stops or deletes DDEV projects, Docker, repositories, or project configuration. If automatic cleanup was interrupted, run the standalone uninstaller again. Unknown files in the installation directory are retained.

### Locations and diagnostics

| Item                     | Linux                                                             | macOS                                                                                      |
| ------------------------ | ----------------------------------------------------------------- | ------------------------------------------------------------------------------------------ |
| Companion/data           | `~/.local/share/cortier-ddev-manager/`                            | `~/Library/Application Support/Cortier DDEV Manager/`                                      |
| Native host registration | `~/.mozilla/native-messaging-hosts/com.cortier.ddev_manager.json` | `~/Library/Application Support/Mozilla/NativeMessagingHosts/com.cortier.ddev_manager.json` |
| Login registration       | `~/.config/systemd/user/com.cortier.ddev-manager.service`         | `~/Library/LaunchAgents/com.cortier.ddev-manager.plist`                                    |

Run the installed binary with `doctor`. On Linux, inspect `systemctl --user status com.cortier.ddev-manager.service` and its user journal. On macOS, inspect `launchctl print gui/$(id -u)/com.cortier.ddev-manager`. A port conflict is reported rather than silently changing an already registered uninstall address.

Installation errors after files are copied are reported explicitly. Correct the problem and re-run the installer, or run the standalone uninstaller. The installer does not require administrator privileges.

## Architecture and protocol

- TypeScript extension: persistent Firefox background context owns native messaging, operation queues, cache, settings, and tab creation. Popup teardown does not own command lifetime.
- Go native host: validates requests, resolves project IDs through DDEV’s registry, calls DDEV/Git with argument arrays, and uses a cross-process operation lock. No content scripts or website access permissions.
- Go cleanup service: loopback-only HTTP for removal and a permission-restricted Unix socket for authenticated profile registration. It never exposes DDEV actions over HTTP.
- Cleanup helper: a separate transient systemd service / launchd job, so it survives stopping the cleanup service that launched it.

Native messages are length-prefixed JSON with `version: 1`, a correlation `id`, and a `method`. Methods: `discover`, `services`, `action`, `register`, `diagnostics`, `configure`. Actions: `start`, `restart`, `stop`, `open`, and `ide`. Service IDs are resolved on the companion; requests never supply arbitrary shell commands or launch URLs. Responses return `result` or `{error: {code, message}}`. Protocol mismatch and disconnected hosts are shown in the UI.

Executable settings only accept absolute executable paths; DDEV and Git also require their expected basename. The IDE setting takes precedence over `$IDE`, which is used only while the setting is empty. The companion passes the registered project directory as one argument without shell interpolation. Profile identifiers, authentication material, and installation configuration remain local. No telemetry, external uninstall callbacks, or container assignment.

## Validation and packaging

```sh
npm run check
npm test
npm run build
npm run lint:extension
npx playwright install firefox
npm run test:ui
(cd companion && go test -race ./... && go vet ./...)
npm run package
scripts/package-companion.sh
```

Unit tests cover grouping, lifecycle behavior, services, URL handling, serialization, framing, profile registration, hostile requests, token replay, helper failure, installation, and idempotent cleanup. Firefox UI tests cover light/dark layouts, long names, keyboard access, search, services, and task actions. Background tests cover popup-independent batches, partial failure, duplicate actions, and reconnects. CI runs Go tests on Linux and macOS and builds all four companion targets.

### Opt-in local integration checks

`DDEV_MANAGER_LIVE=1 go test -run TestLiveDDEVLifecycle -v` (inside `companion/`) creates a disposable DDEV project, exercises its lifecycle, and deletes it afterward. It does not operate on existing projects.

`python3 tests/smoke-installed.py` checks the native protocol, the configured smoke-test services, service restart, and two-profile cleanup. `python3 tests/smoke-firefox.py` loads a test copy of the extension in an isolated headless Firefox profile and verifies actual Firefox uninstall-triggered cleanup. Both scripts require a fresh Linux companion installation with **no registered profiles**, and intentionally remove that test installation. Reinstall between them and afterward. These opt-in checks are separate from CI.

### Signing

Firefox checks the latest GitHub release's `updates.json` asset for versions newer than the installed extension. The feed points to the Mozilla-signed XPI in that same release. The native companion is updated separately by running the installer from the new release.

Maintainers publish with the **Deploy release** GitHub Action. Select `patch`, `minor`, or `major`; the workflow bumps both manifests, validates the extension and companion, submits source and extension archives to Mozilla’s unlisted channel, waits for approval, creates the tag and GitHub release, and publishes the update feed. The `release` environment stores `AMO_JWT_ISSUER` and `AMO_JWT_SECRET`; pull requests never receive those credentials.

For local signing diagnostics, a publisher can run:

```sh
npx web-ext sign --source-dir dist/extension --channel unlisted \
  --api-key "$AMO_JWT_ISSUER" --api-secret "$AMO_JWT_SECRET" \
  --artifacts-dir dist/signed
```

Credentials must never be committed. Only signed XPIs belong in public releases.

References: [Firefox native messaging](https://developer.mozilla.org/en-US/docs/Mozilla/Add-ons/WebExtensions/Native_messaging), [uninstall URLs](https://developer.mozilla.org/en-US/docs/Mozilla/Add-ons/WebExtensions/API/runtime/setUninstallURL), [Mozilla signing](https://extensionworkshop.com/documentation/publish/signing-and-distribution-overview/), [visual reference](https://github.com/mozilla/multi-account-containers/).
