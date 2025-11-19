# Repository Guidelines

## Project Structure & Module Organization
Piccolo OS is organized for image-building work. `kiwi/microos-ots/` holds the KIWI profile (boot scripts, `os.kiwi`, and disk hooks). Operational tooling lives in `scripts/` (build orchestration, VM bring-up, mirrors). Build artifacts drop under `releases/<profile>/`, keeping logs beside the output image for later inspection. The `02_product` symlink points to upstream product docs; update that reference only if the source directory changes.

## Build, Test, and Development Commands
- `./scripts/run-native.sh Standard microos-ots` installs missing deps and invokes `kiwi-ng system build` for the given profile.
- `sudo kiwi-ng system build --description kiwi/microos-ots --target-dir releases/...` is the lower-level call when iterating on KIWI descriptors.
- `./scripts/start-vm.sh releases/microos-ots/Standard_0.1.0-dev/disk.vdi` clones the `piccolo-template` VirtualBox VM and boots the freshly built disk for smoke tests.
- `./scripts/tw_mirror.sh --dest /var/mirrors/tw` keeps an offline openSUSE mirror; point KIWI config to `file:///var/mirrors/tw/oss` when building in air-gapped labs.

## Coding Style & Naming Conventions
Shell scripts must start with `#!/usr/bin/env bash` and `set -euo pipefail`. Use two-space indentation, uppercase vars (e.g., `RELEASE_DIR`), and lowercase kebab-case script names. KIWI XML stays declarative: attributes camelCase, elements snake_case, comments explaining why (not what). Keep helper scripts idempotent and prefer long option flags (e.g., `--profile`, `--dest`).

## Testing Guidelines
There is no automated CI yet, so every PR must include manual validation notes. Run `./scripts/run-native.sh` and attach the resulting `releases/.../kiwi.log`. Boot the VDI via `scripts/start-vm.sh`, confirm `http://piccolo.local` is reachable, and capture console logs if provisioning fails. When touching disk hooks, describe how you verified rollback/reboot behavior (e.g., `btrfs subvolume list` output). Propose new tests inside a `tests/` tree only when you also script how to run them.

## Commit & Pull Request Guidelines
Commits follow the current history: single-line, present-tense summaries (<=72 chars), e.g., `kiwi-clean-3`. Squash fixups locally; avoid merge commits. Each PR should include context (linked issue or rationale), a concise change list, manual test evidence, and screenshots for UI/portal edits. Flag any release-impacting changes so maintainers can refresh published images.

## Security & Environment Notes
Treat release directories as sensitive: remove cached credentials before sharing, and never commit anything under `releases/`. Prefer offline mirrors or LAN-only endpoints when syncing packages; document any external endpoints you must open so others can replicate the build.
