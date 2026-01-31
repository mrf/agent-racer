#!/bin/bash
set -e

INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
CONFIG_DIR="${HOME}/.config/agent-racer"
BINARY_NAME="agent-racer"

echo "Building agent-racer..."

cd "$(dirname "$0")/.."
make build

echo "Installing to ${INSTALL_DIR}..."

if [ -w "$INSTALL_DIR" ]; then
  cp "${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
else
  sudo cp "${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
fi

# Install default config
mkdir -p "$CONFIG_DIR"
if [ ! -f "${CONFIG_DIR}/config.yaml" ]; then
  cp config.yaml "${CONFIG_DIR}/config.yaml"
  echo "Installed default config to ${CONFIG_DIR}/config.yaml"
fi

echo ""
echo "Installation complete!"
echo "  Binary: ${INSTALL_DIR}/${BINARY_NAME}"
echo "  Config: ${CONFIG_DIR}/config.yaml"
echo ""
echo "Usage:"
echo "  agent-racer                    # Real mode - monitors actual Claude sessions"
echo "  agent-racer --mock             # Mock mode - demo with simulated sessions"
echo "  agent-racer --config path.yaml # Custom config"
echo "  agent-racer --port 9090        # Custom port"
echo ""
echo "Open http://localhost:8080 in your browser"
