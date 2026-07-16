#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'EOF'
Usage:
  scripts/validate-boot-health-vm.sh [OPTIONS] VDI

Run one destructive boot-health scenario against a disposable copy of a final
Piccolo OS VDI. The input image is never modified.

Options:
  --scenario NAME       Scenario to run (default: first-and-repeat)
  --template NAME       Diskless VirtualBox template (default: piccolo-template)
  --subvol PATH         Root snapshot subvolume (default: @/.snapshots/1/snapshot)
  --disk-size MIB       Disposable disk size (default: 24576)
  --deadline SECONDS    Override the scenario deadline
  --keep                Keep the powered-off disposable workspace
  -h, --help            Show this help

Scenarios:
  first-and-repeat      HTTP 503, first-failure marker, then repeated recovery
  marker-repeat         HTTP 503 with health-check.rebooted already present
  unavailable           Local readiness endpoint remains unavailable
  missing-state         HTTP 503 with health-check.state absent
  failed-rollback       HTTP 503 with an invalid last-working snapshot
  hang                  A test plugin request hangs past TimeoutStartSec
  recover               HTTP 503 on boot one, HTTP 200 on boot two
  offline-healthy       No VM NICs; loopback readiness returns HTTP 200
  intentional-poweroff  Power off while a hung health check is active
  reboot-disabled       Boot with health-checker-reboot=disabled and HTTP 503

The test requires mount privileges, qemu-img, Btrfs tools, and VirtualBox.
Evidence is read from the disposable disk, so assertions do not depend on SSH,
LAN connectivity, or the Piccolo API remaining available.
The reboot-disabled scenario exits 77 when the image does not use the upstream
BLS health-check path that implements that kernel option.
EOF
}

scenario=first-and-repeat
template=piccolo-template
subvol='@/.snapshots/1/snapshot'
disk_size_mib=24576
deadline=
keep=0
artifact=

while [ "$#" -gt 0 ]; do
  case "$1" in
    --scenario)
      [ "$#" -ge 2 ] || { usage; exit 2; }
      scenario=$2
      shift 2
      ;;
    --scenario=*)
      scenario=${1#--scenario=}
      shift
      ;;
    --template)
      [ "$#" -ge 2 ] || { usage; exit 2; }
      template=$2
      shift 2
      ;;
    --template=*)
      template=${1#--template=}
      shift
      ;;
    --subvol)
      [ "$#" -ge 2 ] || { usage; exit 2; }
      subvol=$2
      shift 2
      ;;
    --subvol=*)
      subvol=${1#--subvol=}
      shift
      ;;
    --disk-size)
      [ "$#" -ge 2 ] || { usage; exit 2; }
      disk_size_mib=$2
      shift 2
      ;;
    --disk-size=*)
      disk_size_mib=${1#--disk-size=}
      shift
      ;;
    --deadline)
      [ "$#" -ge 2 ] || { usage; exit 2; }
      deadline=$2
      shift 2
      ;;
    --deadline=*)
      deadline=${1#--deadline=}
      shift
      ;;
    --keep)
      keep=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    --*)
      usage
      exit 2
      ;;
    *)
      if [ -n "$artifact" ]; then
        usage
        exit 2
      fi
      artifact=$1
      shift
      ;;
  esac
done

case "$scenario" in
  first-and-repeat|marker-repeat|unavailable|missing-state|failed-rollback|hang|recover|offline-healthy|intentional-poweroff|reboot-disabled) ;;
  *)
    echo "validate-boot-health-vm: unknown scenario: $scenario" >&2
    usage
    exit 2
    ;;
esac

if [ -z "$artifact" ] || [ ! -f "$artifact" ]; then
  echo "validate-boot-health-vm: VDI not found: ${artifact:-<missing>}" >&2
  exit 2
fi

case "$disk_size_mib" in
  ''|*[!0-9]*)
    echo "validate-boot-health-vm: --disk-size must be an integer" >&2
    exit 2
    ;;
esac

if [[ ! "$subvol" =~ ^@/\.snapshots/[0-9]+/snapshot$ ]]; then
  echo "validate-boot-health-vm: --subvol must match @/.snapshots/NUMBER/snapshot" >&2
  exit 2
fi

case "${deadline:-0}" in
  *[!0-9]*)
    echo "validate-boot-health-vm: --deadline must be an integer" >&2
    exit 2
    ;;
esac

require_tool() {
  local tool=$1
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "validate-boot-health-vm: missing required tool: $tool" >&2
    exit 1
  fi
}

for tool in VBoxManage awk btrfs find grep journalctl mount qemu-img sed sfdisk umount; do
  require_tool "$tool"
done

if [ "$(id -u)" -eq 0 ]; then
  sudo_cmd=()
else
  require_tool sudo
  sudo_cmd=(sudo)
fi

if [ -z "$deadline" ]; then
  case "$scenario" in
    hang) deadline=420 ;;
    intentional-poweroff) deadline=120 ;;
    offline-healthy) deadline=120 ;;
    *) deadline=180 ;;
  esac
fi

tmpdir=$(mktemp -d /tmp/piccolo-boot-health-vm.XXXXXX)
source_raw="$tmpdir/source.raw"
working_vdi="$tmpdir/scenario.vdi"
evidence_raw="$tmpdir/evidence.raw"
top_mount="$tmpdir/top"
root_mount="$tmpdir/root"
var_mount="$root_mount/var"
efi_mount="$tmpdir/efi"
vm_name="piccolo-health-${scenario//[^a-zA-Z0-9]/-}-$$"
vm_registered=0
top_mounted=0
root_mounted=0
var_mounted=0
efi_mounted=0
root_snapshot_was_readonly=0
root_snapshot_path=
root_offset=

mkdir -p "$top_mount" "$root_mount" "$efi_mount"

cleanup_mounts() {
  if [ "$efi_mounted" -eq 1 ]; then
    "${sudo_cmd[@]}" umount "$efi_mount" || true
    efi_mounted=0
  fi
  if [ "$var_mounted" -eq 1 ]; then
    "${sudo_cmd[@]}" umount "$var_mount" || true
    var_mounted=0
  fi
  if [ "$root_mounted" -eq 1 ]; then
    "${sudo_cmd[@]}" umount "$root_mount" || true
    root_mounted=0
  fi
  if [ "$root_snapshot_was_readonly" -eq 1 ] && [ -n "$root_snapshot_path" ] && [ "$top_mounted" -eq 1 ]; then
    "${sudo_cmd[@]}" btrfs property set -ts "$root_snapshot_path" ro true || true
    root_snapshot_was_readonly=0
  fi
  if [ "$top_mounted" -eq 1 ]; then
    "${sudo_cmd[@]}" umount "$top_mount" || true
    top_mounted=0
  fi
}

cleanup() {
  local state
  cleanup_mounts
  if [ "$vm_registered" -eq 1 ]; then
    state=$(VBoxManage showvminfo "$vm_name" --machinereadable 2>/dev/null | sed -n 's/^VMState="\([^"]*\)"/\1/p' || true)
    if [ "$state" = "running" ] || [ "$state" = "paused" ]; then
      VBoxManage controlvm "$vm_name" poweroff >/dev/null 2>&1 || true
    fi
    if [ "$keep" -eq 1 ]; then
      VBoxManage storageattach "$vm_name" --storagectl SATA --port 0 --device 0 --type hdd --medium none >/dev/null 2>&1 || true
      VBoxManage unregistervm "$vm_name" >/dev/null 2>&1 || true
      VBoxManage closemedium disk "$working_vdi" >/dev/null 2>&1 || true
    else
      VBoxManage unregistervm "$vm_name" --delete >/dev/null 2>&1 || true
    fi
    vm_registered=0
  fi
  if [ "$keep" -eq 1 ]; then
    echo "validate-boot-health-vm: kept workspace: $tmpdir" >&2
  else
    rm -rf "$tmpdir"
  fi
}
trap cleanup EXIT

partition_offset() {
  local raw=$1
  local table sector_size start_sector partition_size
  table=$(sfdisk -d "$raw")
  sector_size=$(printf '%s\n' "$table" | awk -F: '
    /sector-size:/ {
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", $2)
      print $2
      exit
    }
  ')
  [ -n "$sector_size" ] || sector_size=512
  read -r start_sector partition_size < <(
    printf '%s\n' "$table" | awk '
      /start=/ && /size=/ {
        start = ""
        size = ""
        for (i = 1; i <= NF; i++) {
          if ($i == "start=" && (i + 1) <= NF) start = $(i + 1)
          else if ($i ~ /^start=/) { start = $i; sub(/^start=/, "", start) }
          if ($i == "size=" && (i + 1) <= NF) size = $(i + 1)
          else if ($i ~ /^size=/) { size = $i; sub(/^size=/, "", size) }
        }
        gsub(/,/, "", start)
        gsub(/,/, "", size)
        if ((size + 0) > max_size) {
          max_size = size + 0
          max_start = start + 0
        }
      }
      END { if (max_size > 0) print max_start, max_size }
    '
  )
  if [ -z "${start_sector:-}" ] || [ -z "${partition_size:-}" ]; then
    echo "validate-boot-health-vm: cannot locate root partition in $raw" >&2
    return 1
  fi
  printf '%s\n' "$((start_sector * sector_size))"
}

mount_image_rw() {
  local raw=$1 offset ro_value
  offset=${root_offset:-$(partition_offset "$raw")}
  "${sudo_cmd[@]}" mount -o "rw,loop,offset=${offset},subvolid=5" "$raw" "$top_mount"
  top_mounted=1
  root_snapshot_path="$top_mount/$subvol"
  if [ ! -d "$root_snapshot_path" ]; then
    echo "validate-boot-health-vm: root snapshot not found: $subvol" >&2
    return 1
  fi
  ro_value=$("${sudo_cmd[@]}" btrfs property get -ts "$root_snapshot_path" ro | awk -F= '{print $2}')
  if [ "$ro_value" = "true" ]; then
    "${sudo_cmd[@]}" btrfs property set -ts "$root_snapshot_path" ro false
    root_snapshot_was_readonly=1
  fi
  "${sudo_cmd[@]}" mount -o "rw,loop,offset=${offset},subvol=${subvol}" "$raw" "$root_mount"
  root_mounted=1
  "${sudo_cmd[@]}" mkdir -p "$var_mount"
  "${sudo_cmd[@]}" mount -o "rw,loop,offset=${offset},subvol=@/var" "$raw" "$var_mount"
  var_mounted=1
}

mount_image_ro() {
  local raw=$1 offset
  offset=${root_offset:-$(partition_offset "$raw")}
  "${sudo_cmd[@]}" mount -o "ro,loop,offset=${offset},subvolid=5" "$raw" "$top_mount"
  top_mounted=1
  root_snapshot_path="$top_mount/$subvol"
  "${sudo_cmd[@]}" mount -o "ro,loop,offset=${offset},subvol=${subvol}" "$raw" "$root_mount"
  root_mounted=1
  "${sudo_cmd[@]}" mkdir -p "$var_mount"
  "${sudo_cmd[@]}" mount -o "ro,loop,offset=${offset},subvol=@/var" "$raw" "$var_mount"
  var_mounted=1
}

mount_efi_rw() {
  local raw=$1 table sector_size start_sector partition_size
  table=$(sfdisk -d "$raw")
  sector_size=$(printf '%s\n' "$table" | awk -F: '
    /sector-size:/ {
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", $2)
      print $2
      exit
    }
  ')
  [ -n "$sector_size" ] || sector_size=512
  read -r start_sector partition_size < <(
    printf '%s\n' "$table" | awk '
      tolower($0) ~ /type=c12a7328-f81f-11d2-ba4b-00a0c93ec93b/ {
        start = ""
        size = ""
        for (i = 1; i <= NF; i++) {
          if ($i == "start=" && (i + 1) <= NF) start = $(i + 1)
          else if ($i ~ /^start=/) { start = $i; sub(/^start=/, "", start) }
          if ($i == "size=" && (i + 1) <= NF) size = $(i + 1)
          else if ($i ~ /^size=/) { size = $i; sub(/^size=/, "", size) }
        }
        gsub(/,/, "", start)
        gsub(/,/, "", size)
        print start, size
        exit
      }
    '
  )
  if [ -z "${start_sector:-}" ] || [ -z "${partition_size:-}" ]; then
    echo "validate-boot-health-vm: cannot locate the EFI system partition" >&2
    return 1
  fi
  "${sudo_cmd[@]}" mount -o "rw,loop,offset=$((start_sector * sector_size)),sizelimit=$((partition_size * sector_size))" "$raw" "$efi_mount"
  efi_mounted=1
}

write_guest_file() {
  local destination=$1 mode=$2 staging
  staging=$(mktemp "$tmpdir/write.XXXXXX")
  tee "$staging" >/dev/null
  "${sudo_cmd[@]}" install -D -m "$mode" "$staging" "$destination"
  rm -f "$staging"
}

apply_guest_selinux_labels() {
  local unit_ref unit_dir_ref exec_ref plugin_ref path
  if ! grep -Eq '^[[:space:]]*SELINUX[[:space:]]*=[[:space:]]*enforcing[[:space:]]*$' "$root_mount/etc/selinux/config" 2>/dev/null; then
    return
  fi
  require_tool chcon
  unit_ref="$root_mount/usr/lib/systemd/system/health-checker.service"
  unit_dir_ref="$root_mount/usr/lib/systemd/system"
  exec_ref="$root_mount/usr/libexec/piccolo/bootstrap-dns.sh"
  plugin_ref="$root_mount/usr/libexec/health-checker/piccolod.sh"

  for path in \
    "$root_mount/etc/systemd/system/health-checker.service.d" \
    "$root_mount/etc/systemd/system/default.target.wants"; do
    "${sudo_cmd[@]}" chcon --reference="$unit_dir_ref" "$path"
  done

  for path in \
    "$root_mount/usr/libexec/piccolo/boot-health-test-http" \
    "$root_mount/usr/libexec/piccolo/boot-health-test-observer"; do
    "${sudo_cmd[@]}" chcon --reference="$exec_ref" "$path"
  done
  "${sudo_cmd[@]}" chcon --reference="$plugin_ref" \
    "$root_mount/usr/libexec/health-checker/zz-piccolo-boot-health-test.sh"

  for path in \
    "$root_mount/etc/systemd/system/piccolo-boot-health-test-observer.service" \
    "$root_mount/etc/systemd/system/piccolo-boot-health-test-result.service" \
    "$root_mount/etc/systemd/system/piccolo-boot-health-test.socket" \
    "$root_mount/etc/systemd/system/piccolo-boot-health-test@.service" \
    "$root_mount/etc/systemd/system/health-checker.service.d/90-piccolo-boot-health-test.conf"; do
    "${sudo_cmd[@]}" chcon --reference="$unit_ref" "$path"
  done
  if [ -e "$root_mount/etc/systemd/system/piccolo-boot-health-test-poweroff.service" ]; then
    "${sudo_cmd[@]}" chcon --reference="$unit_ref" \
      "$root_mount/etc/systemd/system/piccolo-boot-health-test-poweroff.service"
  fi
  for path in \
    "$root_mount/etc/systemd/system/default.target.wants/piccolo-boot-health-test-observer.service" \
    "$root_mount/etc/systemd/system/default.target.wants/piccolo-boot-health-test-result.service" \
    "$root_mount/etc/systemd/system/default.target.wants/piccolo-boot-health-test-poweroff.service"; do
    if [ -L "$path" ]; then
      "${sudo_cmd[@]}" chcon -h --reference="$unit_dir_ref" "$path"
    fi
  done

  "${sudo_cmd[@]}" chcon -R --reference="$var_mount/lib/piccolo" \
    "$var_mount/lib/piccolo-boot-health-test"
  for path in "$var_mount/lib/misc/health-check.state" "$var_mount/lib/misc/health-check.rebooted"; do
    if [ -e "$path" ]; then
      "${sudo_cmd[@]}" chcon --reference="$var_mount/lib/misc" "$path"
    fi
  done
}

inject_fixture() {
  write_guest_file "$root_mount/usr/libexec/piccolo/boot-health-test-http" 0755 <<'EOF'
#!/bin/bash
set -u

state_dir=/var/lib/piccolo-boot-health-test
boot_id=$(< /proc/sys/kernel/random/boot_id)
mkdir -p "$state_dir/boots/$boot_id"

# Consume the proxy request headers before replying on the socket passed by
# systemd. The response schedule is selected by boot, not by request count.
while IFS= read -r line; do
  [ "$line" != $'\r' ] || break
done

if [ ! -f "$state_dir/last-boot-id" ] || [ "$(< "$state_dir/last-boot-id")" != "$boot_id" ]; then
  count=0
  [ ! -f "$state_dir/boot-count" ] || count=$(< "$state_dir/boot-count")
  count=$((count + 1))
  printf '%s\n' "$count" > "$state_dir/boot-count"
  printf '%s\n' "$boot_id" > "$state_dir/last-boot-id"
else
  count=$(< "$state_dir/boot-count")
fi

mode=$(sed -n "${count}p" "$state_dir/schedule")
if [ -z "$mode" ]; then
  mode=$(tail -n 1 "$state_dir/schedule")
fi
printf '%s mode=%s\n' "$(date --iso-8601=seconds)" "$mode" >> "$state_dir/boots/$boot_id/http.log"

case "$mode" in
  200)
    status='200 OK'
    body='{"status":"ready"}'
    ;;
  503)
    status='503 Service Unavailable'
    body='{"status":"not-ready","source":"boot-health-test"}'
    ;;
  *)
    status='500 Internal Server Error'
    body='{"status":"invalid-test-mode"}'
    ;;
esac

printf 'HTTP/1.1 %s\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s' \
  "$status" "${#body}" "$body"
EOF

  write_guest_file "$root_mount/usr/libexec/health-checker/zz-piccolo-boot-health-test.sh" 0755 <<'EOF'
#!/bin/bash
set -u

state_dir=/var/lib/piccolo-boot-health-test
boot_id=$(< /proc/sys/kernel/random/boot_id)

case "${1:-}" in
  check)
    if [ -e "$state_dir/hang-plugin" ]; then
      mkdir -p "$state_dir/boots/$boot_id"
      printf '%s\n' "$(date --iso-8601=seconds)" > "$state_dir/boots/$boot_id/hang-entered"
      exec sleep infinity
    fi
    ;;
  stop)
    ;;
  *)
    exit 2
    ;;
esac
EOF

  write_guest_file "$root_mount/usr/libexec/piccolo/boot-health-test-observer" 0755 <<'EOF'
#!/bin/bash
set -u

action=${1:?missing observer action}
state_dir=/var/lib/piccolo-boot-health-test
boot_id=$(< /proc/sys/kernel/random/boot_id)
boot_dir="$state_dir/boots/$boot_id"
mkdir -p "$boot_dir"

record_state() {
  local unit last_working_id
  printf 'timestamp=%s\n' "$(date --iso-8601=seconds)"
  printf 'boot_id=%s\n' "$boot_id"
  printf 'cmdline=%s\n' "$(< /proc/cmdline)"
  if [ -e /var/lib/misc/health-check.rebooted ]; then
    printf 'marker=present\n'
  else
    printf 'marker=absent\n'
  fi
  if [ -e /var/lib/misc/health-check.state ]; then
    printf 'health_state=present\n'
    last_working_id=$(sed -n 's/^LAST_WORKING_BTRFS_ID=//p' /var/lib/misc/health-check.state | head -n 1)
    printf 'health_state_last_working_btrfs_id=%s\n' "$last_working_id"
  else
    printf 'health_state=absent\n'
  fi
  for unit in health-checker.service piccolod.service NetworkManager.service emergency.target shutdown.target; do
    printf '%s=%s\n' "$unit" "$(systemctl is-active "$unit" 2>/dev/null || true)"
  done
}

case "$action" in
  boot-start)
    record_state > "$boot_dir/boot-start"
    printf '%s\n' "$boot_id" >> "$state_dir/boot-ids"
    ;;
  health-success)
    {
      printf 'SERVICE_RESULT=success\n'
      record_state
    } > "$boot_dir/health-result"
    ;;
  health-stop)
    {
      printf 'SERVICE_RESULT=%s\n' "${SERVICE_RESULT:-unknown}"
      printf 'EXIT_CODE=%s\n' "${EXIT_CODE:-unknown}"
      printf 'EXIT_STATUS=%s\n' "${EXIT_STATUS:-unknown}"
      record_state
    } > "$boot_dir/health-result"
    ;;
  *)
    exit 2
    ;;
esac
EOF

  write_guest_file "$root_mount/etc/systemd/system/piccolo-boot-health-test-observer.service" 0644 <<'EOF'
[Unit]
Description=Piccolo boot-health VM test observer
DefaultDependencies=no
After=local-fs.target
Before=health-checker.service

[Service]
Type=oneshot
ExecStart=/usr/libexec/piccolo/boot-health-test-observer boot-start
RemainAfterExit=yes
EOF

  write_guest_file "$root_mount/etc/systemd/system/piccolo-boot-health-test-result.service" 0644 <<'EOF'
[Unit]
Description=Record successful Piccolo boot-health VM test result
After=health-checker.service

[Service]
Type=oneshot
ExecCondition=/usr/bin/systemctl is-active --quiet health-checker.service
ExecStart=/usr/libexec/piccolo/boot-health-test-observer health-success
EOF

  write_guest_file "$root_mount/etc/systemd/system/piccolo-boot-health-test.socket" 0644 <<'EOF'
[Unit]
Description=Piccolo boot-health VM test HTTP fixture
ConditionPathExists=/var/lib/piccolo-boot-health-test/enable-http

[Socket]
ListenStream=127.0.0.1:18080
Accept=yes
EOF

  write_guest_file "$root_mount/etc/systemd/system/piccolo-boot-health-test@.service" 0644 <<'EOF'
[Unit]
Description=Piccolo boot-health VM test HTTP response

[Service]
Type=oneshot
ExecStart=/usr/libexec/piccolo/boot-health-test-http
StandardInput=socket
StandardOutput=socket
StandardError=journal
EOF

  write_guest_file "$root_mount/etc/systemd/system/health-checker.service.d/90-piccolo-boot-health-test.conf" 0644 <<'EOF'
[Unit]
Wants=piccolo-boot-health-test.socket
After=piccolo-boot-health-test-observer.service piccolo-boot-health-test.socket

[Service]
Environment="http_proxy=http://127.0.0.1:18080"
Environment="HTTP_PROXY=http://127.0.0.1:18080"
Environment="no_proxy="
Environment="NO_PROXY="
ExecStopPost=/usr/libexec/piccolo/boot-health-test-observer health-stop
EOF

  "${sudo_cmd[@]}" mkdir -p "$root_mount/etc/systemd/system/default.target.wants"
  "${sudo_cmd[@]}" ln -sfn ../piccolo-boot-health-test-observer.service \
    "$root_mount/etc/systemd/system/default.target.wants/piccolo-boot-health-test-observer.service"
  "${sudo_cmd[@]}" ln -sfn ../piccolo-boot-health-test-result.service \
    "$root_mount/etc/systemd/system/default.target.wants/piccolo-boot-health-test-result.service"
}

seed_valid_health_state() {
  local default_id default_path current_id
  default_id=$("${sudo_cmd[@]}" btrfs subvolume get-default "$top_mount" | awk '{print $2}')
  default_path=$("${sudo_cmd[@]}" btrfs subvolume get-default "$top_mount" | awk '{for (i=9; i<=NF; i++) printf "%s%s", (i == 9 ? "" : OFS), $i; print ""}')
  current_id=$("${sudo_cmd[@]}" btrfs subvolume show "$root_snapshot_path" | awk -F: '/Subvolume ID:/ {gsub(/^[[:space:]]+/, "", $2); print $2; exit}')
  if [ "$default_id" != "$current_id" ]; then
    echo "validate-boot-health-vm: selected root is not the Btrfs default ($current_id != $default_id)" >&2
    return 1
  fi
  write_guest_file "$var_mount/lib/misc/health-check.state" 0644 <<EOF
LAST_WORKING_BTRFS_ID=$default_id
LAST_WORKING_SNAPSHOT=$default_path
EOF
}

append_reboot_disabled_option() {
  local raw=$1 configs=() config changed=0
  mount_efi_rw "$raw"
  while IFS= read -r -d '' config; do
    configs+=("$config")
  done < <("${sudo_cmd[@]}" find "$efi_mount/loader/entries" -type f -name '*.conf' -print0 2>/dev/null)
  if [ "${#configs[@]}" -eq 0 ]; then
    echo "validate-boot-health-vm: SKIP reboot-disabled; image does not use the health-checker's BLS path" >&2
    exit 77
  fi
  for config in "${configs[@]}"; do
    if "${sudo_cmd[@]}" grep -Eq '^[[:space:]]*options[[:space:]]' "$config"; then
      "${sudo_cmd[@]}" sed -Ei '/^[[:space:]]*options[[:space:]]/ s/$/ health-checker-reboot=disabled/' "$config"
      changed=1
    fi
  done
  if [ "$changed" -ne 1 ]; then
    echo "validate-boot-health-vm: BLS entries contain no kernel options for reboot-disabled scenario" >&2
    return 1
  fi
}

configure_scenario() {
  "${sudo_cmd[@]}" rm -rf "$var_mount/lib/piccolo-boot-health-test"
  "${sudo_cmd[@]}" mkdir -p "$var_mount/lib/piccolo-boot-health-test" "$var_mount/lib/misc"
  "${sudo_cmd[@]}" rm -f "$var_mount/lib/misc/health-check.rebooted" "$var_mount/lib/misc/health-check.state"

  case "$scenario" in
    first-and-repeat|unavailable|recover|reboot-disabled)
      seed_valid_health_state
      ;;
    marker-repeat)
      seed_valid_health_state
      write_guest_file "$var_mount/lib/misc/health-check.rebooted" 0644 <<'EOF'
boot-health-vm-test
EOF
      ;;
    failed-rollback)
      seed_valid_health_state
      "${sudo_cmd[@]}" sed -i 's/^LAST_WORKING_BTRFS_ID=.*/LAST_WORKING_BTRFS_ID=999999999/' "$var_mount/lib/misc/health-check.state"
      ;;
    missing-state|hang|offline-healthy|intentional-poweroff)
      ;;
  esac

  case "$scenario" in
    first-and-repeat|marker-repeat|missing-state|failed-rollback|reboot-disabled)
      write_guest_file "$var_mount/lib/piccolo-boot-health-test/schedule" 0644 <<'EOF'
503
EOF
      ;;
    unavailable)
      write_guest_file "$var_mount/lib/piccolo-boot-health-test/schedule" 0644 <<'EOF'
503
EOF
      ;;
    recover)
      write_guest_file "$var_mount/lib/piccolo-boot-health-test/schedule" 0644 <<'EOF'
503
200
EOF
      ;;
    hang|intentional-poweroff)
      write_guest_file "$var_mount/lib/piccolo-boot-health-test/schedule" 0644 <<'EOF'
200
EOF
      write_guest_file "$var_mount/lib/piccolo-boot-health-test/hang-plugin" 0644 <<'EOF'
enabled
EOF
      ;;
    offline-healthy)
      write_guest_file "$var_mount/lib/piccolo-boot-health-test/schedule" 0644 <<'EOF'
200
EOF
      ;;
  esac

  if [ "$scenario" != "unavailable" ]; then
    write_guest_file "$var_mount/lib/piccolo-boot-health-test/enable-http" 0644 <<'EOF'
enabled
EOF
  fi

  if [ "$scenario" = "intentional-poweroff" ]; then
    write_guest_file "$root_mount/etc/systemd/system/piccolo-boot-health-test-poweroff.service" 0644 <<'EOF'
[Unit]
Description=Power off during Piccolo boot-health VM test
After=local-fs.target

[Service]
Type=oneshot
ExecStart=/bin/bash -c 'for _ in {1..120}; do if find /var/lib/piccolo-boot-health-test/boots -name hang-entered -type f -print -quit 2>/dev/null | grep -q .; then date --iso-8601=seconds > /var/lib/piccolo-boot-health-test/poweroff-triggered; systemctl poweroff; exit 0; fi; sleep 1; done; exit 1'
EOF
    "${sudo_cmd[@]}" ln -sfn ../piccolo-boot-health-test-poweroff.service \
      "$root_mount/etc/systemd/system/default.target.wants/piccolo-boot-health-test-poweroff.service"
  fi

  if [ "$scenario" = "reboot-disabled" ]; then
    append_reboot_disabled_option "$source_raw"
  fi
}

vm_state() {
  VBoxManage showvminfo "$vm_name" --machinereadable 2>/dev/null | sed -n 's/^VMState="\([^"]*\)"/\1/p'
}

wait_for_scenario() {
  local elapsed=0 state
  while [ "$elapsed" -lt "$deadline" ]; do
    state=$(vm_state || true)
    if [ "$state" = "poweroff" ] || [ "$state" = "aborted" ]; then
      if [ "$scenario" = "intentional-poweroff" ] && [ "$state" = "poweroff" ]; then
        return 0
      fi
      echo "validate-boot-health-vm: VM stopped unexpectedly with state=$state" >&2
      return 1
    fi
    sleep 5
    elapsed=$((elapsed + 5))
  done

  if [ "$scenario" = "intentional-poweroff" ]; then
    echo "validate-boot-health-vm: intentional poweroff did not finish within ${deadline}s" >&2
    return 1
  fi
  VBoxManage controlvm "$vm_name" poweroff >/dev/null
}

assert_evidence() {
  local state_dir="$var_mount/lib/piccolo-boot-health-test"
  local boot_count results journal_text boot_journal mode_count
  local -a boot_ids=()
  if [ ! -d "$state_dir/boots" ]; then
    echo "validate-boot-health-vm: no persistent observer evidence found" >&2
    return 1
  fi
  boot_count=$("${sudo_cmd[@]}" find "$state_dir/boots" -mindepth 1 -maxdepth 1 -type d | wc -l)
  results=$("${sudo_cmd[@]}" grep -Rhs '^SERVICE_RESULT=' "$state_dir/boots" || true)
  if [ -f "$state_dir/boot-ids" ]; then
    mapfile -t boot_ids < <("${sudo_cmd[@]}" awk "!seen[\$0]++" "$state_dir/boot-ids")
  fi
  if [ "${#boot_ids[@]}" -lt "$boot_count" ]; then
    echo "validate-boot-health-vm: chronological boot evidence is incomplete" >&2
    return 1
  fi

  if [ -d "$var_mount/log/journal" ]; then
    journal_text=$("${sudo_cmd[@]}" journalctl --directory="$var_mount/log/journal" --no-pager -o short-monotonic 2>/dev/null || true)
  else
    journal_text=
  fi

  assert_failed_boot() {
    local boot_id=$1 expected_mode=$2 result_file="$state_dir/boots/$1/health-result"
    if [ ! -f "$result_file" ] || ! "${sudo_cmd[@]}" grep -Eq '^SERVICE_RESULT=(exit-code|signal|timeout)$' "$result_file"; then
      echo "validate-boot-health-vm: boot $boot_id lacks a failed health-checker result" >&2
      return 1
    fi
    case "$expected_mode" in
      503)
        mode_count=$("${sudo_cmd[@]}" grep -c ' mode=503$' "$state_dir/boots/$boot_id/http.log" 2>/dev/null || true)
        if [ "$mode_count" -lt 15 ]; then
          echo "validate-boot-health-vm: boot $boot_id recorded only $mode_count HTTP 503 responses" >&2
          return 1
        fi
        ;;
      unavailable)
        if [ -z "$journal_text" ]; then
          echo "validate-boot-health-vm: persistent journal required for unavailable-endpoint correlation" >&2
          return 1
        fi
        boot_journal=$("${sudo_cmd[@]}" journalctl --directory="$var_mount/log/journal" --boot="$boot_id" --unit=health-checker.service --no-pager -o cat 2>/dev/null || true)
        mode_count=$(grep -c 'curl_exit=7, http_status=000' <<<"$boot_journal" || true)
        if [ "$mode_count" -lt 15 ]; then
          echo "validate-boot-health-vm: boot $boot_id recorded only $mode_count unavailable-endpoint attempts" >&2
          return 1
        fi
        ;;
    esac
  }

  assert_signal_failure() {
    local boot_id=$1 result_file="$state_dir/boots/$1/health-result"
    if [ ! -f "$result_file" ] || ! "${sudo_cmd[@]}" grep -q '^SERVICE_RESULT=signal$' "$result_file"; then
      echo "validate-boot-health-vm: boot $boot_id did not record the expected signal-terminated health-checker result" >&2
      return 1
    fi
  }

  assert_boot_log() {
    local boot_id=$1 pattern=$2
    if [ -z "$journal_text" ]; then
      echo "validate-boot-health-vm: persistent journal required for branch correlation" >&2
      return 1
    fi
    boot_journal=$("${sudo_cmd[@]}" journalctl --directory="$var_mount/log/journal" --boot="$boot_id" --no-pager -o cat 2>/dev/null || true)
    if ! grep -Eq "$pattern" <<<"$boot_journal"; then
      echo "validate-boot-health-vm: boot $boot_id did not execute expected branch: $pattern" >&2
      return 1
    fi
  }

  echo "validate-boot-health-vm: observed $boot_count boot(s)"
  "${sudo_cmd[@]}" find "$state_dir/boots" -mindepth 2 -maxdepth 2 -type f -print -exec sed -n '1,80p' {} \;

  case "$scenario" in
    offline-healthy)
      [ "$boot_count" -ge 1 ]
      grep -q '^SERVICE_RESULT=success$' <<<"$results"
      mode_count=$("${sudo_cmd[@]}" grep -c ' mode=200$' "$state_dir/boots/${boot_ids[0]}/http.log" 2>/dev/null || true)
      [ "$mode_count" -ge 1 ]
      [ ! -e "$var_mount/lib/misc/health-check.rebooted" ]
      ;;
    recover)
      [ "$boot_count" -ge 2 ]
      assert_failed_boot "${boot_ids[0]}" 503
      grep -q '^SERVICE_RESULT=success$' <<<"$results"
      [ ! -e "$var_mount/lib/misc/health-check.rebooted" ]
      ;;
    intentional-poweroff)
      [ "$boot_count" -eq 1 ]
      [ -f "$state_dir/boots/${boot_ids[0]}/hang-entered" ]
      [ -f "$state_dir/poweroff-triggered" ]
      ;;
    first-and-repeat)
      [ "$boot_count" -ge 3 ]
      "${sudo_cmd[@]}" grep -q '^marker=absent$' "$state_dir/boots/${boot_ids[0]}/boot-start"
      assert_failed_boot "${boot_ids[0]}" 503
      assert_boot_log "${boot_ids[0]}" "Machine didn't come up correctly, trying a reboot"
      "${sudo_cmd[@]}" grep -q '^marker=present$' "$state_dir/boots/${boot_ids[1]}/boot-start"
      assert_failed_boot "${boot_ids[1]}" 503
      assert_signal_failure "${boot_ids[1]}"
      assert_boot_log "${boot_ids[1]}" "Machine didn't come up correctly, starting emergency shell"
      [ -e "$var_mount/lib/misc/health-check.rebooted" ]
      ;;
    unavailable)
      [ "$boot_count" -ge 3 ]
      "${sudo_cmd[@]}" grep -q '^marker=absent$' "$state_dir/boots/${boot_ids[0]}/boot-start"
      assert_failed_boot "${boot_ids[0]}" unavailable
      assert_boot_log "${boot_ids[0]}" "Machine didn't come up correctly, trying a reboot"
      "${sudo_cmd[@]}" grep -q '^marker=present$' "$state_dir/boots/${boot_ids[1]}/boot-start"
      assert_failed_boot "${boot_ids[1]}" unavailable
      assert_signal_failure "${boot_ids[1]}"
      assert_boot_log "${boot_ids[1]}" "Machine didn't come up correctly, starting emergency shell"
      ;;
    marker-repeat)
      [ "$boot_count" -ge 2 ]
      "${sudo_cmd[@]}" grep -q '^marker=present$' "$state_dir/boots/${boot_ids[0]}/boot-start"
      assert_failed_boot "${boot_ids[0]}" 503
      assert_signal_failure "${boot_ids[0]}"
      assert_boot_log "${boot_ids[0]}" "Machine didn't come up correctly, starting emergency shell"
      ;;
    missing-state)
      [ "$boot_count" -ge 2 ]
      "${sudo_cmd[@]}" grep -q '^health_state=absent$' "$state_dir/boots/${boot_ids[0]}/boot-start"
      assert_failed_boot "${boot_ids[0]}" 503
      assert_boot_log "${boot_ids[0]}" "Machine didn't come up correctly, stopping services"
      ;;
    failed-rollback)
      [ "$boot_count" -ge 2 ]
      "${sudo_cmd[@]}" grep -q '^health_state_last_working_btrfs_id=999999999$' "$state_dir/boots/${boot_ids[0]}/boot-start"
      assert_failed_boot "${boot_ids[0]}" 503
      assert_boot_log "${boot_ids[0]}" 'ERROR: btrfs set-default .* failed!'
      ;;
    reboot-disabled)
      [ "$boot_count" -ge 2 ]
      "${sudo_cmd[@]}" grep -Eq '^cmdline=([^[:space:]]+[[:space:]])*health-checker-reboot=disabled([[:space:]]|$)' "$state_dir/boots/${boot_ids[0]}/boot-start"
      assert_failed_boot "${boot_ids[0]}" 503
      assert_signal_failure "${boot_ids[0]}"
      assert_boot_log "${boot_ids[0]}" "Machine didn't come up correctly, starting emergency shell"
      ;;
    hang)
      [ "$boot_count" -ge 2 ]
      [ -f "$state_dir/boots/${boot_ids[0]}/hang-entered" ]
      "${sudo_cmd[@]}" grep -q '^SERVICE_RESULT=timeout$' "$state_dir/boots/${boot_ids[0]}/health-result"
      ;;
  esac

  if [ -n "$journal_text" ]; then
    printf '%s\n' "$journal_text" | grep -E 'health-checker|piccolod is healthy|starting emergency shell|boot-health-test' | tail -n 200 || true
    if [ "$scenario" = "reboot-disabled" ]; then
      grep -q 'health-checker-reboot=disabled' <<<"$journal_text"
    fi
  elif [ "$scenario" = "reboot-disabled" ]; then
    echo "validate-boot-health-vm: persistent journal required for reboot-disabled assertion" >&2
    return 1
  fi
}

echo "validate-boot-health-vm: preparing disposable image for scenario=$scenario" >&2
qemu-img convert -O raw "$artifact" "$source_raw"
qemu-img resize "$source_raw" "${disk_size_mib}M" >/dev/null
root_offset=$(partition_offset "$source_raw")
mount_image_rw "$source_raw"
inject_fixture
configure_scenario
apply_guest_selinux_labels
cleanup_mounts
qemu-img convert -O vdi "$source_raw" "$working_vdi"

echo "validate-boot-health-vm: creating VM $vm_name from template $template" >&2
VBoxManage clonevm "$template" --name "$vm_name" --basefolder "$tmpdir" --register >/dev/null
vm_registered=1
VBoxManage storageattach "$vm_name" --storagectl SATA --port 0 --device 0 --type hdd --medium none >/dev/null 2>&1 || true
VBoxManage storageattach "$vm_name" --storagectl SATA --port 0 --device 0 --type hdd --medium "$working_vdi" >/dev/null
if [ "$scenario" = "offline-healthy" ]; then
  VBoxManage modifyvm "$vm_name" \
    --nic1 none --nic2 none --nic3 none --nic4 none \
    --nic5 none --nic6 none --nic7 none --nic8 none >/dev/null
fi
VBoxManage startvm "$vm_name" --type headless >/dev/null

echo "validate-boot-health-vm: observing for up to ${deadline}s" >&2
wait_for_scenario

qemu-img convert -O raw "$working_vdi" "$evidence_raw"
mount_image_ro "$evidence_raw"
assert_evidence
cleanup_mounts

echo "validate-boot-health-vm: PASS scenario=$scenario"
