VERSION=0.1.0-dev
# Use first argument as the profile, default to "Standard"
PROFILE="${1:-Standard}"
KIWIBASE=${2:-piccolo-microos}

RELEASE_DIR=releases/$KIWIBASE/${PROFILE}_${VERSION}
echo "Building release in $RELEASE_DIR"

sudo rm -rf $RELEASE_DIR
mkdir -p $RELEASE_DIR

kiwi-ng --profile $PROFILE \
  --logfile $RELEASE_DIR/kvm.log \
  system build \
  --description "$(cd "$(dirname "${BASH_SOURCE[0]}")/../kiwi" >/dev/null 2>&1 && pwd)/${KIWIBASE}" \
  --target-dir $RELEASE_DIR
