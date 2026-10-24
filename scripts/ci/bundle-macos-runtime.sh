#!/bin/bash
# Bundle PortAudio and the from-source whisper.cpp runtime into Yap.app, make every
# reference bundle-relative, audit the result, smoke test it and ad-hoc sign it.
# Usage: scripts/ci/bundle-macos-runtime.sh <path/to/Yap.app> <whisper.cpp source dir>
#
# Layout (resolved by internal/transcribe relative to Contents/Resources/bin):
#   Contents/Frameworks/libportaudio.2.dylib
#   Contents/Resources/bin/whisper-cli, whisper-server
#   Contents/Resources/lib/libwhisper.1.dylib, libggml.0.dylib, libggml-base.0.dylib
#   Contents/Resources/libexec/libggml-metal.so, libggml-blas.so, libggml-cpu-apple_*.so
set -euo pipefail

APP="${1:?app bundle path required}"
WHISPER_SRC="${2:?whisper.cpp source dir required}"
WHISPER_BIN="$WHISPER_SRC/build/bin"
SMOKE_DIR="$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/yap-smoke.XXXXXX")"

RES="$APP/Contents/Resources"
mkdir -p "$APP/Contents/Frameworks" "$RES/bin" "$RES/lib" "$RES/libexec"

PORTAUDIO_PREFIX="$(brew --prefix portaudio)"
cp -L "$PORTAUDIO_PREFIX/lib/libportaudio.2.dylib" "$APP/Contents/Frameworks/"
chmod u+w "$APP/Contents/Frameworks/libportaudio.2.dylib"
install_name_tool -id "@executable_path/../Frameworks/libportaudio.2.dylib" "$APP/Contents/Frameworks/libportaudio.2.dylib"
otool -L "$APP/Contents/MacOS/Yap" | awk '/libportaudio\.2\.dylib/ {print $1}' | while read -r dep; do
  install_name_tool -change "$dep" "@executable_path/../Frameworks/libportaudio.2.dylib" "$APP/Contents/MacOS/Yap"
done

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

# Every load command must point at the OS or inside the bundle.
echo "Auditing bundled Mach-O dependencies"
failed=0
while IFS= read -r -d '' file; do
  file "$file" | grep -q 'Mach-O' || continue
  echo "== ${file#"$APP"/}"
  otool -L "$file" | tail -n +2
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

smoke_test() {
  local label="$1"
  echo "Smoke testing bundled whisper runtime ($label)"
  "$RES/bin/whisper-cli" --help >"$SMOKE_DIR/cli-help.txt" 2>&1
  GGML_BACKEND_PATH="$CPU_BACKEND" "$RES/bin/whisper-server" --help >"$SMOKE_DIR/server-help.txt" 2>&1

  # Real inference on the bundled CPU backend with whisper.cpp's tiny test model.
  GGML_BACKEND_PATH="$CPU_BACKEND" "$RES/bin/whisper-cli" --no-gpu \
    -m "$WHISPER_SRC/models/for-tests-ggml-tiny.bin" -f "$WHISPER_SRC/samples/jfk.wav" \
    -d 2000 -otxt -of "$SMOKE_DIR/cli" >"$SMOKE_DIR/cli-run.log" 2>&1 || {
    cat "$SMOKE_DIR/cli-run.log"
    echo "::error::whisper-cli inference failed ($label)"
    return 1
  }
  test -f "$SMOKE_DIR/cli.txt"

  # whisper-server must start with the bundled CPU backend and answer HTTP.
  local port=18089
  GGML_BACKEND_PATH="$CPU_BACKEND" "$RES/bin/whisper-server" --no-gpu \
    -m "$WHISPER_SRC/models/for-tests-ggml-tiny.bin" --host 127.0.0.1 --port "$port" \
    >"$SMOKE_DIR/server.log" 2>&1 &
  local pid=$!
  local ok=0
  for _ in $(seq 1 60); do
    if curl -fs -o /dev/null "http://127.0.0.1:$port/"; then ok=1; break; fi
    kill -0 "$pid" 2>/dev/null || break
    sleep 0.5
  done
  if [ "$ok" -eq 1 ]; then
    curl -fsS -F file=@"$WHISPER_SRC/samples/jfk.wav" -F response_format=json \
      "http://127.0.0.1:$port/inference" >"$SMOKE_DIR/server-inference.json" || ok=0
  fi
  kill "$pid" 2>/dev/null || true
  wait "$pid" 2>/dev/null || true
  if [ "$ok" -ne 1 ]; then
    cat "$SMOKE_DIR/server.log"
    echo "::error::whisper-server did not serve requests ($label)"
    return 1
  fi

  # Informational only: with GGML_BACKEND_PATH pointing at the Metal module no CPU backend is
  # loaded, which whisper.cpp rejects (the previous Homebrew bundle behaved the same on
  # machines without Homebrew's ggml). The app then falls back to the CPU backend.
  if GGML_BACKEND_PATH="$RES/libexec/libggml-metal.so" "$RES/bin/whisper-cli" \
    -m "$WHISPER_SRC/models/for-tests-ggml-tiny.bin" -f "$WHISPER_SRC/samples/jfk.wav" \
    -d 2000 >"$SMOKE_DIR/metal-run.log" 2>&1; then
    echo "Metal backend inference works ($label)"
  else
    tail -n 20 "$SMOKE_DIR/metal-run.log"
    echo "::warning::Metal backend inference failed ($label); the app would fall back to CPU"
  fi
}

smoke_test "before signing"

echo "Signing bundled macOS runtime files"
for bin in "$RES/bin"/* "$RES/lib"/*.dylib "$RES/libexec"/* "$APP/Contents/Frameworks"/*.dylib; do
  [ -f "$bin" ] || continue
  codesign --force --sign - "$bin"
  codesign --verify --strict --verbose=2 "$bin"
done
codesign --force --sign - "$APP/Contents/MacOS/Yap"
codesign --verify --strict --verbose=2 "$APP/Contents/MacOS/Yap"
codesign --force --deep --sign - "$APP"
codesign --verify --deep --strict --verbose=2 "$APP"

smoke_test "after signing"
rm -rf "$SMOKE_DIR"
echo "Bundled runtime dependency audit complete"
