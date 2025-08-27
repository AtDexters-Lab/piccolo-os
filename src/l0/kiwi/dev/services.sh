#==========================================
# DEVELOPMENT ADDITIONS: Enable SSH and Cloud-Init
#==========================================

echo "Enabling DEVELOPMENT services (SSH + Cloud-Init)..."

# Remove cloud-init disable marker that was created by production config
echo "--> Removing cloud-init disable marker for development"
rm -f /etc/cloud/cloud-init.disabled

# SSH access for development - unmask first in case production masked it
systemctl unmask sshd.service || true
systemctl enable sshd.service

# Cloud-init for automated testing and configuration - unmask services first
systemctl unmask cloud-init.target || true
systemctl unmask cloud-init.service || true
systemctl unmask cloud-init-local.service || true
systemctl unmask cloud-config.service || true
systemctl unmask cloud-final.service || true

systemctl enable cloud-init.target
systemctl enable cloud-init.service
systemctl enable cloud-init-local.service
systemctl enable cloud-config.service
systemctl enable cloud-final.service

echo "DEVELOPMENT services enabled successfully"
echo "  ✓ SSH access enabled"
echo "  ✓ Cloud-init enabled for testing"

# Override production messaging with development messaging
echo ""
echo "DEVELOPMENT configuration complete - SSH + Cloud-init ENABLED"
echo "System configured for:"
echo "  ✓ API access via piccolod on port 80"
echo "  ✓ SSH access enabled for development"
echo "  ✓ Cloud-init enabled for automated testing"
echo "  ✓ Serial console available"
echo "  ✓ Interactive login available"
echo "  ✓ Standard logging"
echo "  ✓ Container runtime via Podman socket"