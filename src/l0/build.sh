SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
PICCOLO_VERSION="1.0.0"

cd "${SCRIPT_DIR}/../l1/piccolod"
PICCOLOD_OUTPUT=${SCRIPT_DIR}/build/${PICCOLO_VERSION}/piccolod
go build -o $PICCOLOD_OUTPUT cmd/piccolod/main.go

cd "${SCRIPT_DIR}"
./build_piccolo.sh --version ${PICCOLO_VERSION} --binary-path $PICCOLOD_OUTPUT  > build.out 2> build.error.out