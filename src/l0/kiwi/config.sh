#!/bin/bash
#================
# FILE          : config.sh
#----------------
# PROJECT       : Piccolo OS
# COPYRIGHT     : (c) 2025 Piccolo
# AUTHOR        : Piccolo Team
# BELONGS TO    : Operating System images
# DESCRIPTION   : configuration script for MicroOS appliance
#
#================

set -euxo pipefail

#==========================================
# Systemd service enablement
#==========================================

echo "Enabling essential services for Piccolo OS..."

# Network and SSH access
systemctl enable NetworkManager.service
systemctl enable sshd.service

# Cloud-init for automated configuration
systemctl enable cloud-init.target

# Container runtime - enable socket for API access
systemctl enable podman.socket

# Piccolo daemon and health monitoring
systemctl enable piccolod.service
systemctl enable piccolod-health-check.service
systemctl enable piccolod-failure-handler.service

echo "Services enabled successfully"

#==========================================
# Additional OS customizations can go here
#==========================================

exit 0