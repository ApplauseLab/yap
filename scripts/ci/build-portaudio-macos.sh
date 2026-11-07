#!/bin/bash
# Build PortAudio for macOS arm64 against a fixed deployment target.
# Usage: scripts/ci/build-portaudio-macos.sh <install prefix> [deployment target]
#
# Homebrew's bottle is compiled for the OS it was bottled on, so a runner on macOS 26
# yields libportaudio with minos 26.0 and the app refuses to launch on anything older,
# whatever LSMinimumSystemVersion claims. Building from source pins the floor instead,
# the same reason whisper.cpp is built from source here.
#
# CMake rather than configure: configure.in appends -Werror after the caller's CFLAGS,
# and this release predates -Wimplicit-const-int-float-conversion, so autotools cannot
# compile it with current clang without patching the build system.
set -euo pipefail

PREFIX="${1:?install prefix required}"
DEPLOYMENT_TARGET="${2:-${MACOS_DEPLOYMENT_TARGET:-11.0}}"

# Matches the Homebrew formula's source, so this is the same code that shipped before,
# only rebuilt with a lower deployment target.
VERSION="190700_20210406"
URL="https://files.portaudio.com/archives/pa_stable_v${VERSION}.tgz"
SHA256="47efbf42c77c19a05d22e627d42873e991ec0c1357219c0d74ce6a2948cb2def"

WORK="$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/portaudio.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT

curl -fsSL --retry 5 --retry-delay 5 --retry-all-errors -o "$WORK/pa.tgz" "$URL"
ACTUAL="$(shasum -a 256 "$WORK/pa.tgz" | cut -d' ' -f1)"
if [ "$ACTUAL" != "$SHA256" ]; then
  echo "::error::PortAudio checksum mismatch: expected $SHA256, got $ACTUAL"
  exit 1
fi
tar xzf "$WORK/pa.tgz" -C "$WORK"

# CMAKE_POLICY_VERSION_MINIMUM: the release declares cmake_minimum_required(VERSION 2.8),
# which CMake 4 refuses outright.
#
# CMAKE_INSTALL_NAME_DIR gives the dylib an absolute install name, matching how the
# Homebrew build behaved. The default @rpath install name breaks `wails build`, whose
# binding generator runs the freshly linked binary before bundle-macos-runtime.sh has
# rewritten anything, and that binary has no LC_RPATH to resolve @rpath against.
cmake -S "$WORK/portaudio" -B "$WORK/build" \
  -DCMAKE_POLICY_VERSION_MINIMUM=3.5 \
  -DCMAKE_BUILD_TYPE=Release \
  -DCMAKE_OSX_DEPLOYMENT_TARGET="$DEPLOYMENT_TARGET" \
  -DCMAKE_OSX_ARCHITECTURES=arm64 \
  -DBUILD_SHARED_LIBS=ON \
  -DPA_BUILD_TESTS=OFF -DPA_BUILD_EXAMPLES=OFF \
  -DCMAKE_INSTALL_PREFIX="$PREFIX" \
  -DCMAKE_INSTALL_NAME_DIR="$PREFIX/lib"
cmake --build "$WORK/build" -j "$(sysctl -n hw.ncpu)"
cmake --install "$WORK/build"

LIB="$PREFIX/lib/libportaudio.dylib"
[ -f "$LIB" ] || { echo "::error::PortAudio build produced no $LIB"; exit 1; }

ACTUAL_MINOS="$(otool -l "$LIB" | awk '/LC_BUILD_VERSION/{f=1} f&&/minos/{print $2; exit}')"
if [ "$ACTUAL_MINOS" != "$DEPLOYMENT_TARGET" ]; then
  echo "::error::PortAudio minos is $ACTUAL_MINOS, expected $DEPLOYMENT_TARGET"
  exit 1
fi
echo "Built PortAudio $VERSION for macOS $DEPLOYMENT_TARGET+ at $PREFIX"
