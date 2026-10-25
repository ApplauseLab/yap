#!/bin/bash
# Build whisper-cli and whisper-server for macOS arm64 from a pinned whisper.cpp tag.
# Usage: scripts/ci/build-whisper-macos.sh <whisper.cpp tag> <source dir>
#
# The layout matches what the app expects at runtime (see internal/transcribe):
# backends are loadable modules (libggml-metal.so, libggml-cpu-apple_*.so) selected
# through GGML_BACKEND_PATH, so GGML_BACKEND_DL requires shared ggml/whisper libraries.
# OpenMP is disabled so the bundle does not need libomp.
set -euo pipefail

VERSION="${1:?whisper.cpp tag required}"
SRC="${2:?source dir required}"

if [ ! -d "$SRC/.git" ]; then
  git clone --depth 1 --branch "$VERSION" https://github.com/ggml-org/whisper.cpp.git "$SRC"
fi

cmake -S "$SRC" -B "$SRC/build" \
  -DCMAKE_BUILD_TYPE=Release \
  -DWHISPER_BUILD_TESTS=OFF -DWHISPER_BUILD_EXAMPLES=ON -DWHISPER_BUILD_SERVER=ON \
  -DWHISPER_SDL2=OFF -DWHISPER_CURL=OFF \
  -DBUILD_SHARED_LIBS=ON -DGGML_BACKEND_DL=ON -DGGML_CPU_ALL_VARIANTS=ON \
  -DGGML_NATIVE=OFF -DGGML_OPENMP=OFF \
  -DGGML_METAL=ON -DGGML_METAL_EMBED_LIBRARY=ON -DGGML_BLAS=ON
cmake --build "$SRC/build" --config Release --target whisper-cli whisper-server -j "$(sysctl -n hw.ncpu)"

ls "$SRC/build/bin"
