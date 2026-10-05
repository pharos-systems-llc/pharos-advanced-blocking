#!/usr/bin/env bash
set -e

# Premium-looking output formatting
GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${BLUE}======================================================${NC}"
echo -e "${BLUE}       Pharos Advanced Blocking (pab) Installer       ${NC}"
echo -e "${BLUE}======================================================${NC}"

# Detect architecture
ARCH=$(uname -m)
if [ "$ARCH" = "x86_64" ]; then
    ARCH="amd64"
elif [ "$ARCH" = "aarch64" ] || [ "$ARCH" = "arm64" ]; then
    ARCH="arm64"
else
    echo -e "${RED}Error: Unsupported architecture: $ARCH. Only 64-bit x86 and ARM architectures are supported.${NC}"
    exit 1
fi

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
if [ "$OS" != "linux" ]; then
    echo -e "${RED}Error: Unsupported OS: $OS. Only Linux is supported.${NC}"
    exit 1
fi

echo -e "${GREEN}Detected OS: $OS, Architecture: $ARCH${NC}"

REPO="pharos-systems-llc/pharos-advanced-blocking"
LATEST_URL="https://api.github.com/repos/$REPO/releases/latest"

echo -e "${BLUE}Fetching latest version info...${NC}"
# Check if curl or wget is installed
if command -v curl &> /dev/null; then
    VERSION=$(curl -sL $LATEST_URL | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
elif command -v wget &> /dev/null; then
    VERSION=$(wget -qO- $LATEST_URL | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
else
    echo -e "${RED}Error: curl or wget is required to download releases.${NC}"
    exit 1
fi

download_file() {
    local url=$1
    local dest=$2
    if command -v curl &> /dev/null; then
        if ! curl -sLf -o "$dest" "$url"; then
            echo -e "${RED}Error: Failed to download from $url${NC}"
            exit 1
        fi
    elif command -v wget &> /dev/null; then
        if ! wget -qO "$dest" "$url"; then
            echo -e "${RED}Error: Failed to download from $url${NC}"
            exit 1
        fi
    fi
}

if [ -z "$VERSION" ]; then
    echo -e "${YELLOW}Warning: Could not fetch latest release version. Maybe the repository doesn't have releases yet, or isn't public.${NC}"
    echo -e "${YELLOW}Falling back to dummy version v1.0.0 for local testing.${NC}"
    VERSION="v1.0.0"
fi

TAR_NAME="pharos-advanced-blocking_${VERSION#v}_${OS}_${ARCH}.tar.gz"
CHECKSUMS_NAME="checksums.txt"
SIGNATURE_NAME="checksums.txt.sig"
CERT_NAME="checksums.txt.pem"
# Wait, goreleaser config specifies --bundle=${signature}bundle for cosign sign-blob.
# So the bundle file would be checksums.txt.sigbundle maybe?
# The config has: 
#      - sign-blob
#      - "--yes"
#      - "--bundle=${signature}bundle"
#      - "${artifact}"
# But we can just use the signature or print instructions

DOWNLOAD_URL="https://github.com/$REPO/releases/download/$VERSION/$TAR_NAME"
CHECKSUM_URL="https://github.com/$REPO/releases/download/$VERSION/$CHECKSUMS_NAME"

echo -e "${BLUE}Downloading $TAR_NAME...${NC}"
download_file "$DOWNLOAD_URL" "$TAR_NAME"

echo -e "${BLUE}Downloading $CHECKSUMS_NAME...${NC}"
download_file "$CHECKSUM_URL" "$CHECKSUMS_NAME"

echo -e "${BLUE}Verifying checksums...${NC}"
if command -v sha256sum &> /dev/null; then
    if ! grep "$TAR_NAME" "$CHECKSUMS_NAME" | sha256sum -c - > /dev/null 2>&1; then
        echo -e "${RED}Error: Checksum verification failed for $TAR_NAME!${NC}"
        exit 1
    fi
    echo -e "${GREEN}Checksum verification passed.${NC}"
else
    echo -e "${YELLOW}Warning: sha256sum not found, skipping checksum verification.${NC}"
fi

echo -e "${BLUE}Verifying signatures...${NC}"
if command -v cosign &> /dev/null; then
    echo -e "${YELLOW}Notice: cosign is installed. Signature verification is available but requires downloading the signature bundle.${NC}"
    echo -e "You can manually verify by downloading the bundle from the release page and running:"
    echo -e "  cosign verify-blob $CHECKSUMS_NAME --bundle <bundle-file> --certificate-identity \"https://github.com/$REPO/.github/workflows/release.yml@refs/tags/$VERSION\" --certificate-oidc-issuer \"https://token.actions.githubusercontent.com\""
else
    echo -e "${YELLOW}Notice: cosign is not installed. To verify signatures manually in the future:${NC}"
    echo -e "  1. Install cosign (https://docs.sigstore.dev/cosign/installation/)"
    echo -e "  2. Run: cosign verify-blob $CHECKSUMS_NAME --bundle <bundle-file> --certificate-identity \"https://github.com/$REPO/.github/workflows/release.yml@refs/tags/$VERSION\" --certificate-oidc-issuer \"https://token.actions.githubusercontent.com\""
fi

echo -e "${BLUE}Extracting binary...${NC}"
if ! tar -xzf "$TAR_NAME" pab; then
    echo -e "${RED}Error: Failed to extract $TAR_NAME${NC}"
    exit 1
fi

INSTALL_DIR="/usr/local/bin"
USE_SUDO="sudo"

# Check if we have write permission to /usr/local/bin
if [ ! -w "$INSTALL_DIR" ]; then
    if command -v sudo &> /dev/null && sudo -n true 2>/dev/null; then
        echo -e "${BLUE}Installing binary to $INSTALL_DIR (using sudo)...${NC}"
    else
        INSTALL_DIR="$HOME/.local/bin"
        USE_SUDO=""
        echo -e "${YELLOW}Cannot write to /usr/local/bin and sudo is unavailable or declined.${NC}"
        echo -e "${BLUE}Falling back to local installation in $INSTALL_DIR...${NC}"
        mkdir -p "$INSTALL_DIR"
    fi
else
    echo -e "${BLUE}Installing binary to $INSTALL_DIR...${NC}"
    USE_SUDO=""
fi

if [ -n "$USE_SUDO" ]; then
    if ! sudo mv pab "$INSTALL_DIR/"; then
        echo -e "${RED}Error: Failed to install pab to $INSTALL_DIR${NC}"
        exit 1
    fi
    sudo chmod +x "$INSTALL_DIR/pab"
else
    if ! mv pab "$INSTALL_DIR/"; then
        echo -e "${RED}Error: Failed to install pab to $INSTALL_DIR${NC}"
        exit 1
    fi
    chmod +x "$INSTALL_DIR/pab"
fi

echo -e "${GREEN}======================================================${NC}"
echo -e "${GREEN}  pab installed successfully to $INSTALL_DIR/pab!  ${NC}"
echo -e "${GREEN}======================================================${NC}"
if [[ "$INSTALL_DIR" == "$HOME/.local/bin" ]]; then
    if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
        echo -e "${YELLOW}Note: $INSTALL_DIR is not in your PATH.${NC}"
        echo -e "Add 'export PATH=\"\$HOME/.local/bin:\$PATH\"' to your ~/.bashrc or ~/.zshrc"
    fi
fi
