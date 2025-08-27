#!/bin/bash
#================
# FILE          : config.sh
#----------------
# PROJECT       : Piccolo OS Production
# COPYRIGHT     : (c) 2025 Piccolo
# AUTHOR        : Piccolo Team
# BELONGS TO    : Operating System images
# DESCRIPTION   : PRODUCTION configuration script for MicroOS appliance
#                 ZERO ACCESS - NO SSH, NO CLOUD-INIT, NO SERIAL CONSOLE
#================

set -euxo pipefail

#==========================================
# PRODUCTION HARDENING - ZERO ACCESS CONFIGURATION
#==========================================

echo "Enabling PRODUCTION services for Piccolo OS - ZERO ACCESS MODE"

# CRITICAL: Only essential services for production appliance
# NO SSH, NO CLOUD-INIT, NO INTERACTIVE ACCESS

#==========================================
# Essential Network and Container Services
#==========================================

# Network management for container networking only
systemctl enable NetworkManager.service

# Container runtime - enable socket for API access only
systemctl enable podman.socket

# Piccolo daemon and health monitoring - PRODUCTION ONLY
systemctl enable piccolod.service
systemctl enable piccolod-health-check.service
# Note: piccolod-failure-handler.service is triggered by OnFailure, not enabled directly

echo "PRODUCTION services enabled successfully"

#==========================================
# PRODUCTION SECURITY HARDENING
#==========================================

echo "Applying PRODUCTION security hardening..."

# DISABLE ALL INTERACTIVE ACCESS METHODS
echo "Disabling all interactive access methods for production..."

# Disable serial console getty services (if any exist)
systemctl disable serial-getty@ttyS0.service 2>/dev/null || true
systemctl disable getty@ttyS0.service 2>/dev/null || true

# Disable any console login attempts
systemctl mask getty@.service 2>/dev/null || true
systemctl mask serial-getty@.service 2>/dev/null || true

# Disable SSH completely (service should not exist in production packages)
systemctl disable sshd.service 2>/dev/null || true
systemctl mask sshd.service 2>/dev/null || true

# Disable cloud-init completely (packages should not exist in production)
systemctl disable cloud-init.target 2>/dev/null || true
systemctl disable cloud-init.service 2>/dev/null || true
systemctl disable cloud-init-local.service 2>/dev/null || true
systemctl disable cloud-config.service 2>/dev/null || true
systemctl disable cloud-final.service 2>/dev/null || true

# Create cloud-init disable marker (defense in depth)
mkdir -p /etc/cloud
touch /etc/cloud/cloud-init.disabled

echo "PRODUCTION security hardening applied successfully"

#==========================================
# PRODUCTION KERNEL COMMAND LINE HARDENING (systemd-boot)
#==========================================

echo "Configuring PRODUCTION kernel parameters for security (systemd-boot)..."

# MicroOS OEM disk image uses systemd-boot - configure /etc/kernel/cmdline
# This will be processed by systemd-boot on USB and installed systems
mkdir -p /etc/kernel
cat > /etc/kernel/cmdline << 'EOF'
console=ttynull quiet loglevel=1 systemd.show_status=false rd.systemd.show_status=false rd.udev.log_level=1 init=/usr/lib/systemd/systemd systemd.unit=multi-user.target panic=10 oops=panic
EOF

echo "PRODUCTION systemd-boot kernel parameters configured for OEM disk image"

# Ensure systemd-boot entries are updated for disk image
if command -v sdbootutil >/dev/null 2>&1; then
  echo "Updating systemd-boot entries for USB/disk deployment..."
  sdbootutil update-all-entries || true
fi

#==========================================
# PRODUCTION LOGGING CONFIGURATION
#==========================================

echo "Configuring PRODUCTION logging - minimal output only..."

# Create production journald configuration for minimal logging
mkdir -p /etc/systemd/journald.conf.d
cat > /etc/systemd/journald.conf.d/99-piccolo-production.conf << 'EOF'
[Journal]
# Production logging configuration - minimal output
Storage=volatile
Compress=yes
Seal=yes
RuntimeMaxUse=50M
RuntimeMaxFileSize=10M
RuntimeMaxFiles=5
MaxRetentionSec=1week

# Reduce logging verbosity for production
MaxLevelStore=warning
MaxLevelSyslog=warning
MaxLevelKMsg=warning
MaxLevelConsole=crit
MaxLevelWall=emerg

# Disable forwarding to console/wall for production
ForwardToConsole=no
ForwardToWall=no
EOF

echo "PRODUCTION logging configuration applied"

echo "PRODUCTION configuration complete - ZERO ACCESS MODE ENABLED"
echo "System configured for:"
echo "  ✓ API-only access via piccolod on port 80"
echo "  ✓ NO SSH access"
echo "  ✓ NO cloud-init"  
echo "  ✓ NO serial console"
echo "  ✓ NO interactive login"
echo "  ✓ Minimal logging"
echo "  ✓ Container runtime via Podman socket only"

# DEV_SERVICES_INSERT_POINT

exit 0