#!/bin/bash
# Bundle PortAudio and the from-source whisper.cpp runtime into Yap.app, make every
# reference bundle-relative, audit the result, smoke test it and sign it.
# Signing is delegated to sign-macos-app.sh, which is ad-hoc unless MACOS_SIGN_IDENTITY
# is set. The post-signing smoke test is what catches Hardened Runtime breaking the
# ggml backends, so it must keep running after any signing change.
# Usage: scripts/ci/bundle-macos-runtime.sh <path/to/Yap.app> <whisper.cpp source dir> [model]
#
# Layout (resolved by internal/transcribe relative to Contents/Resources/bin):
#   Contents/Frameworks/libportaudio.2.dylib
#   Contents/Resources/bin/whisper-cli, whisper-server
#   Contents/Resources/bin/libggml-cpu-apple_*.so -> ../libexec/libggml-cpu-apple_*.so
#   Contents/Resources/lib/libwhisper.1.dylib, libggml.0.dylib, libggml-base.0.dylib
#   Contents/Resources/libexec/libggml-metal.so, libggml-blas.so, libggml-cpu-apple_*.so
#
# [model] is a real Whisper model (CI uses ggml-tiny.en) so the smoke test can check the
# transcript. Without it, whisper.cpp's random-weight test model only proves inference runs.
set -euo pipefail

# Resolved before the smoke tests cd into a scratch directory, which would otherwise
# break a relative path to the sibling signing script.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

APP="$(cd "${1:?app bundle path required}" && pwd)"
WHISPER_SRC="$(cd "${2:?whisper.cpp source dir required}" && pwd)"
MODEL="${3:-}"
if [ -n "$MODEL" ]; then
  MODEL="$(cd "$(dirname "$MODEL")" && pwd)/$(basename "$MODEL")"
fi
WHISPER_BIN="$WHISPER_SRC/build/bin"
SMOKE_DIR="$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/yap-smoke.XXXXXX")"

RES="$APP/Contents/Resources"
mkdir -p "$APP/Contents/Frameworks" "$RES/bin" "$RES/lib" "$RES/libexec"

# The soname differs by build system: Homebrew's autotools build links
# libportaudio.2.dylib, the from-source CMake build links @rpath/libportaudio.dylib.
# Take the name from what the executable actually references rather than hardcoding it.
PORTAUDIO_DEP="$(otool -L "$APP/Contents/MacOS/Yap" | awk 'NR > 1 && $1 ~ /libportaudio/ {print $1; exit}')"
[ -n "$PORTAUDIO_DEP" ] || { echo "::error::Yap does not link PortAudio"; exit 1; }
PORTAUDIO_LIB="$(basename "$PORTAUDIO_DEP")"

PORTAUDIO_PREFIX="${PORTAUDIO_PREFIX:-$(brew --prefix portaudio)}"
cp -L "$PORTAUDIO_PREFIX/lib/$PORTAUDIO_LIB" "$APP/Contents/Frameworks/"
chmod u+w "$APP/Contents/Frameworks/$PORTAUDIO_LIB"
install_name_tool -id "@executable_path/../Frameworks/$PORTAUDIO_LIB" "$APP/Contents/Frameworks/$PORTAUDIO_LIB"
install_name_tool -change "$PORTAUDIO_DEP" "@executable_path/../Frameworks/$PORTAUDIO_LIB" "$APP/Contents/MacOS/Yap"

cp "$WHISPER_BIN/whisper-cli" "$WHISPER_BIN/whisper-server" "$RES/bin/"
chmod +x "$RES/bin/whisper-cli" "$RES/bin/whisper-server"
for lib in libwhisper.1.dylib libggml.0.dylib libggml-base.0.dylib; do
  cp -L "$WHISPER_BIN/$lib" "$RES/lib/"
done
cp "$WHISPER_BIN"/libggml-*.so "$RES/libexec/"
ls "$RES/libexec"/libggml-cpu-apple_*.so "$RES/libexec/libggml-metal.so" >/dev/null

for lib in "$RES/lib"/*.dylib; do
  install_name_tool -id "@loader_path/../lib/$(basename "$lib")" "$lib"
done
for bin in "$RES/bin"/* "$RES/lib"/*.dylib "$RES/libexec"/*; do
  otool -L "$bin" | awk 'NR > 1 && $1 ~ /^(@rpath\/)?lib(whisper|ggml)/ {print $1}' | while read -r dep; do
    install_name_tool -change "$dep" "@loader_path/../lib/$(basename "$dep")" "$bin"
  done
  # Drop build-tree rpaths; every bundled reference is @loader_path-relative now.
  otool -l "$bin" | awk '/cmd LC_RPATH/ {getline; getline; print $2}' | while read -r rpath; do
    install_name_tool -delete_rpath "$rpath" "$bin"
  done
done

# Yap points GGML_BACKEND_PATH at a single module (libggml-metal.so or a CPU variant), and
# ggml finds the rest by scanning the directory of the running binary. Metal needs a CPU
# backend next to it, otherwise whisper.cpp aborts with GGML_ASSERT(device). Symlinks
# rather than copies: dyld resolves both paths to one image, so the CPU backend that
# GGML_BACKEND_PATH names in CPU mode is not registered twice.
for lib in "$RES/libexec"/libggml-cpu-apple_*.so; do
  ln -sfn "../libexec/$(basename "$lib")" "$RES/bin/$(basename "$lib")"
done

# LSMinimumSystemVersion is only a claim; dyld enforces each binary's own minos and
# refuses to load one built for a newer OS than the host. A dependency compiled on a
# newer runner than the declared floor therefore crashes the app at launch on older
# systems, so the claim is checked against every bundled binary rather than trusted.
MIN_OS="$(/usr/libexec/PlistBuddy -c 'Print :LSMinimumSystemVersion' "$APP/Contents/Info.plist")"
echo "Declared LSMinimumSystemVersion: $MIN_OS"

# Compare dotted versions as sortable tuples: 11.0 must not be "greater" than 9.0.
version_gt() {
  [ "$(printf '%s\n%s\n' "$1" "$2" | sort -t. -k1,1n -k2,2n -k3,3n | head -1)" != "$1" ]
}

# Every load command must point at the OS or inside the bundle.
echo "Auditing bundled Mach-O dependencies"
failed=0
while IFS= read -r -d '' file; do
  file "$file" | grep -q 'Mach-O' || continue
  echo "== ${file#"$APP"/}"
  otool -L "$file" | tail -n +2
  minos="$(otool -l "$file" | awk '/LC_BUILD_VERSION/{f=1} f&&/minos/{print $2; exit}')"
  if [ -n "$minos" ] && version_gt "$minos" "$MIN_OS"; then
    echo "::error::${file#"$APP"/} requires macOS $minos but the app claims $MIN_OS"
    failed=1
  fi
  if otool -L "$file" | tail -n +2 | awk '{print $1}' | grep -vE '^(/System/Library/|/usr/lib/|@executable_path/|@loader_path/)' >/dev/null; then
    echo "::error::Dependency outside the OS or app bundle in $file"
    failed=1
  fi
  if otool -l "$file" | awk '/cmd LC_RPATH/ {getline; getline; print $2}' | grep -vE '^(@executable_path|@loader_path)/' >/dev/null; then
    echo "::error::Non-relative LC_RPATH in $file"
    failed=1
  fi
done < <(find "$APP/Contents" -type f -print0)
[ "$failed" -eq 0 ]

CPU_BRAND="$(sysctl -n machdep.cpu.brand_string || true)"
case "$CPU_BRAND" in
  *M4*) CPU_BACKEND_NAME="libggml-cpu-apple_m4.so" ;;
  *M2*|*M3*) CPU_BACKEND_NAME="libggml-cpu-apple_m2_m3.so" ;;
  *) CPU_BACKEND_NAME="libggml-cpu-apple_m1.so" ;;
esac
CPU_BACKEND="$RES/libexec/$CPU_BACKEND_NAME"
[ -f "$CPU_BACKEND" ] || CPU_BACKEND="$RES/libexec/libggml-cpu-apple_m1.so"

WAV="$WHISPER_SRC/samples/jfk.wav"
METAL_BACKEND="$RES/libexec/libggml-metal.so"
EXPECT=""
if [ -n "$MODEL" ]; then
  EXPECT="country"
else
  MODEL="$WHISPER_SRC/models/for-tests-ggml-tiny.bin"
fi

# check_run <mode> <what> <log> <transcript>
check_run() {
  local mode="$1" what="$2" log="$3" text="$4"
  if [ -n "$EXPECT" ] && ! grep -qi "$EXPECT" "$text"; then
    cat "$log"
    echo "::error::$what returned an unexpected transcript: $(head -c 200 "$text")"
    return 1
  fi
  if [ "$mode" = cpu ]; then
    # The bin/ symlink and GGML_BACKEND_PATH should resolve to one CPU backend image.
    grep -qE 'devices += 1$' "$log" || echo "::warning::$what registered more than one CPU device"
    return 0
  fi
  if ! grep -qE 'loaded CPU backend from .*/Resources/bin/libggml-cpu-' "$log"; then
    cat "$log"
    echo "::error::$what did not load a CPU backend from Resources/bin next to Metal"
    return 1
  fi
  if grep -qE 'using MTL[0-9]+ backend' "$log"; then
    echo "$what: Metal backend in use"
  elif grep -q 'ggml_metal_device_init: GPU name' "$log"; then
    cat "$log"
    echo "::error::$what found a Metal GPU but did not use it"
    return 1
  else
    echo "::warning::$what: no Metal GPU on this machine; only checked that the CPU backend loads next to Metal"
  fi
}

# backend_for <mode>: GGML_BACKEND_PATH as Yap sets it (internal/transcribe).
backend_for() {
  if [ "$1" = metal ]; then echo "$METAL_BACKEND"; else echo "$CPU_BACKEND"; fi
}

run_cli() {
  local mode="$1" stage="$2"
  local out="$SMOKE_DIR/cli-$mode-$stage" what="whisper-cli ($mode, $stage signing)"
  local args=()
  [ "$mode" = cpu ] && args=(--no-gpu)
  if ! GGML_BACKEND_PATH="$(backend_for "$mode")" "$RES/bin/whisper-cli" ${args[@]+"${args[@]}"} \
    -m "$MODEL" -f "$WAV" -nt -otxt -of "$out" >"$out.log" 2>&1; then
    cat "$out.log"
    echo "::error::$what failed"
    return 1
  fi
  check_run "$mode" "$what" "$out.log" "$out.txt"
}

run_server() {
  local mode="$1" stage="$2"
  local out="$SMOKE_DIR/server-$mode-$stage" what="whisper-server ($mode, $stage signing)"
  local args=()
  [ "$mode" = cpu ] && args=(--no-gpu)
  local port=18089
  GGML_BACKEND_PATH="$(backend_for "$mode")" "$RES/bin/whisper-server" ${args[@]+"${args[@]}"} \
    -m "$MODEL" --host 127.0.0.1 --port "$port" --no-timestamps >"$out.log" 2>&1 &
  local pid=$!
  local ok=0
  for _ in $(seq 1 120); do
    if curl -fs -o /dev/null "http://127.0.0.1:$port/"; then ok=1; break; fi
    kill -0 "$pid" 2>/dev/null || break
    sleep 0.5
  done
  if [ "$ok" -eq 1 ]; then
    curl -fsS -F file=@"$WAV" -F response_format=text \
      "http://127.0.0.1:$port/inference" >"$out.txt" || ok=0
  fi
  kill "$pid" 2>/dev/null || true
  wait "$pid" 2>/dev/null || true
  if [ "$ok" -ne 1 ]; then
    cat "$out.log"
    echo "::error::$what did not serve requests"
    return 1
  fi
  check_run "$mode" "$what" "$out.log" "$out.txt"
}

smoke_test() {
  local stage="$1"
  echo "Smoke testing bundled whisper runtime ($stage signing)"
  "$RES/bin/whisper-cli" --help >"$SMOKE_DIR/cli-help.txt" 2>&1
  # Metal first, then the CPU fallback, the order Yap tries them in.
  for mode in metal cpu; do
    run_cli "$mode" "$stage"
    run_server "$mode" "$stage"
  done
}

# ggml also searches the current directory for backends; run from an empty one so only
# the bundle layout counts.
cd "$SMOKE_DIR"
smoke_test before

"$SCRIPT_DIR/sign-macos-app.sh" "$APP"

smoke_test after
cd /
rm -rf "$SMOKE_DIR"
echo "Bundled runtime dependency audit complete"
