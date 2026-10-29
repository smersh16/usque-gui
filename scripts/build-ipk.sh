#!/bin/sh
# Сборка usque-gui_<версия>_<arch>.ipk
# usage: scripts/build-ipk.sh <GOARCH> <opkg-arch> [GOMIPS]
#   scripts/build-ipk.sh arm64  aarch64-3.10
#   scripts/build-ipk.sh mipsle mipsel-3.4 softfloat
#   scripts/build-ipk.sh mips   mips-3.4   softfloat
set -eu

GOARCH_T="$1"
PKG_ARCH="$2"
GOMIPS_T="${3:-}"

cd "$(dirname "$0")/.."
VERSION="$(tr -d ' \n' < VERSION)"
NAME="usque-gui"
OUT="out"
WORK="$OUT/build/$PKG_ARCH"
FILE="${NAME}_${VERSION}_${PKG_ARCH}.ipk"

rm -rf "$WORK"
mkdir -p "$WORK/control" "$WORK/data/opt/sbin" "$WORK/data/opt/etc/init.d" "$OUT/pkg"

echo "go build ($GOARCH_T)..."
GOOS=linux GOARCH="$GOARCH_T" GOMIPS="$GOMIPS_T" CGO_ENABLED=0 \
    go build -trimpath -ldflags "-s -w -X main.version=$VERSION" \
    -o "$WORK/data/opt/sbin/$NAME" .

install -m 0755 scripts/init.d/S52usque-gui "$WORK/data/opt/etc/init.d/S52usque-gui"

cat > "$WORK/control/control" <<CTL
Package: $NAME
Version: $VERSION
Depends: usque-keenetic
License: MIT
Section: net
URL: https://github.com/smersh16/usque-gui
Architecture: $PKG_ARCH
Maintainer: smersh16
Description: Web interface for usque-keenetic (Cloudflare WARP MASQUE on Keenetic)

CTL
install -m 0755 scripts/ipk/postinst "$WORK/control/postinst"
install -m 0755 scripts/ipk/prerm "$WORK/control/prerm"

# ipk в формате Entware/opkg: tar.gz из debian-binary, control.tar.gz, data.tar.gz
( cd "$WORK/control" && tar --owner=0 --group=0 --numeric-owner -czf ../control.tar.gz . )
( cd "$WORK/data" && tar --owner=0 --group=0 --numeric-owner -czf ../data.tar.gz . )
echo 2.0 > "$WORK/debian-binary"
( cd "$WORK" && tar --owner=0 --group=0 --numeric-owner -czf "../../pkg/$FILE" ./debian-binary ./control.tar.gz ./data.tar.gz )

echo "собрано: $OUT/pkg/$FILE"
