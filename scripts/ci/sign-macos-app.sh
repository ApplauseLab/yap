#!/bin/bash
# Sign Yap.app inside-out with the Hardened Runtime.
# Usage: scripts/ci/sign-macos-app.sh <path/to/Yap.app>
#
# MACOS_SIGN_IDENTITY selects the identity and defaults to "-" (ad-hoc) so local builds
# work without a certificate. Ad-hoc signatures cannot be notarized; release builds set
# it to a "Developer ID Application: ..." identity.
#
# Signing is explicit and bottom-up rather than --deep: --deep applies one set of
# entitlements to every nested binary and is documented by Apple as unsuitable for
# shipping. Nested code here is signed in dependency order instead:
#   1. libraries and ggml backend modules (no entitlements; they are not main executables)
#   2. whisper-cli / whisper-server  (helper entitlements)
#   3. Contents/MacOS/Yap            (app entitlements)
#   4. the bundle itself             (app entitlements; seals Resources and the symlinks)
set -euo pipefail

APP="$(cd "${1:?app bundle path required}" && pwd)"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

IDENTITY="${MACOS_SIGN_IDENTITY:--}"
ENTITLEMENTS="${MACOS_ENTITLEMENTS:-$REPO_ROOT/build/darwin/entitlements.plist}"
HELPER_ENTITLEMENTS="${MACOS_HELPER_ENTITLEMENTS:-$REPO_ROOT/build/darwin/entitlements.helper.plist}"

[ -f "$ENTITLEMENTS" ] || { echo "::error::missing entitlements: $ENTITLEMENTS"; exit 1; }
[ -f "$HELPER_ENTITLEMENTS" ] || { echo "::error::missing entitlements: $HELPER_ENTITLEMENTS"; exit 1; }

RES="$APP/Contents/Resources"

# A secure timestamp is required for notarization but unavailable to ad-hoc signatures.
TIMESTAMP=(--timestamp)
if [ "$IDENTITY" = "-" ]; then
  TIMESTAMP=(--timestamp=none)
  echo "Signing $APP ad-hoc (not notarizable; set MACOS_SIGN_IDENTITY for a release build)"
else
  echo "Signing $APP as: $IDENTITY"
fi

sign() {
  local entitlements="$1"; shift
  local args=(--force --options runtime "${TIMESTAMP[@]}" --sign "$IDENTITY")
  [ -n "$entitlements" ] && args+=(--entitlements "$entitlements")
  for target in "$@"; do
    # bin/ holds symlinks to ../libexec backends; the app signature seals them and
    # codesign would otherwise sign the same file twice through two paths.
    [ -f "$target" ] && [ ! -L "$target" ] || continue
    codesign "${args[@]}" "$target"
    codesign --verify --strict --verbose=2 "$target"
  done
}

MAIN_EXECUTABLE="$APP/Contents/MacOS/Yap"
# sign() skips anything missing so unmatched globs are harmless, which would also let a
# missing main executable through as a silent no-op and ship an unsigned binary.
[ -f "$MAIN_EXECUTABLE" ] || { echo "::error::missing main executable: $MAIN_EXECUTABLE"; exit 1; }
for required in "$RES"/bin/whisper-cli "$RES"/bin/whisper-server; do
  [ -f "$required" ] || { echo "::error::missing bundled helper: $required"; exit 1; }
done

sign "" "$RES"/lib/*.dylib "$RES"/libexec/* "$APP"/Contents/Frameworks/*.dylib
sign "$HELPER_ENTITLEMENTS" "$RES"/bin/*
sign "$ENTITLEMENTS" "$MAIN_EXECUTABLE"

codesign --force --options runtime "${TIMESTAMP[@]}" --entitlements "$ENTITLEMENTS" \
  --sign "$IDENTITY" "$APP"
codesign --verify --strict --verbose=2 "$APP"

# --verify checks the seal; --display proves the flags and entitlements we rely on
# actually landed, which is what notarization and the runtime care about.
echo "Verifying signature of $APP"
# Captured rather than piped straight into grep: under `set -o pipefail`, `grep -q` exits
# on the first match and the SIGPIPE it sends codesign fails the whole pipeline, which
# inverts these checks into false negatives once the bundle is big enough that codesign
# is still writing when grep leaves.
SIGNATURE_INFO="$(codesign --display --verbose=2 "$APP" 2>&1)"
SIGNATURE_ENTITLEMENTS="$(codesign --display --entitlements - "$APP" 2>&1)"

printf '%s\n' "$SIGNATURE_INFO" | grep -E '^(Authority|TeamIdentifier|Identifier|Signature)' || true
printf '%s\n' "$SIGNATURE_ENTITLEMENTS"

# codesign reports this as flags=0x10002(adhoc,runtime) or 0x10000(runtime); match the
# decoded name rather than the hex, which varies with the other flags that are set.
if ! printf '%s\n' "$SIGNATURE_INFO" | grep -qE 'flags=0x[0-9a-f]+\(.*runtime'; then
  echo "::error::Hardened Runtime flag missing from $APP"
  printf '%s\n' "$SIGNATURE_INFO"
  exit 1
fi

if ! printf '%s\n' "$SIGNATURE_ENTITLEMENTS" | grep -q 'com.apple.security.device.audio-input'; then
  echo "::error::audio-input entitlement missing from $APP; the microphone will fail at runtime"
  exit 1
fi

if [ "$IDENTITY" != "-" ]; then
  # Gatekeeper still rejects an un-notarized app here; this only confirms the signature
  # is well formed and trusted, so a later notarization failure is unambiguous.
  codesign --verify --deep --strict --verbose=2 "$APP"
  echo "Signed and verified with a real identity; notarization is required before release"
fi
