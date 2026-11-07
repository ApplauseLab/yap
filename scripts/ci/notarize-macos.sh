#!/bin/bash
# Submit a .app or .dmg to Apple's notary service and staple the ticket to it.
# Usage: scripts/ci/notarize-macos.sh <path/to/Yap.app|path/to/Yap.dmg>
#
# Requires an App Store Connect API key in the environment:
#   AC_KEY_ID, AC_ISSUER_ID, AC_PRIVATE_KEY (contents of the .p8)
#
# Both the .app and the .dmg are notarized and stapled. Notarizing only the .dmg leaves
# the app without its own ticket, so a copy dragged out of the image (which is what
# `brew install --cask` does) needs an online Gatekeeper check to launch the first time.
# Stapling the app as well keeps first launch working offline.
set -euo pipefail
umask 077

TARGET="${1:?path to .app or .dmg required}"
TARGET="$(cd "$(dirname "$TARGET")" && pwd)/$(basename "$TARGET")"

: "${AC_KEY_ID:?AC_KEY_ID required}"
: "${AC_ISSUER_ID:?AC_ISSUER_ID required}"
: "${AC_PRIVATE_KEY:?AC_PRIVATE_KEY required}"

WORK="$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/yap-notarize.XXXXXX")"
KEY_FILE="$WORK/AuthKey.p8"
cleanup() { rm -rf "$WORK"; }
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
printf '%s' "$AC_PRIVATE_KEY" > "$KEY_FILE"
unset AC_PRIVATE_KEY

# The notary service only accepts zip, dmg and pkg, so a bundle is zipped for upload
# while the ticket is still stapled to the original .app on disk.
UPLOAD="$TARGET"
case "$TARGET" in
  *.app)
    UPLOAD="$WORK/$(basename "$TARGET").zip"
    # ditto preserves symlinks and extended attributes; `zip` would flatten the
    # Resources/bin backend symlinks and invalidate the signature.
    ditto -c -k --keepParent --sequesterRsrc "$TARGET" "$UPLOAD"
    ;;
esac

echo "Submitting $(basename "$UPLOAD") to the notary service"
SUBMIT_LOG="$WORK/submit.json"
set +e
xcrun notarytool submit "$UPLOAD" \
  --key "$KEY_FILE" --key-id "$AC_KEY_ID" --issuer "$AC_ISSUER_ID" \
  --wait --timeout 45m --output-format json > "$SUBMIT_LOG"
SUBMIT_STATUS=$?
set -e

cat "$SUBMIT_LOG"
SUBMISSION_ID="$(/usr/bin/python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get("id",""))' "$SUBMIT_LOG" 2>/dev/null || true)"
STATUS="$(/usr/bin/python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get("status",""))' "$SUBMIT_LOG" 2>/dev/null || true)"

if [ "$SUBMIT_STATUS" -ne 0 ] || [ "$STATUS" != "Accepted" ]; then
  echo "::error::Notarization failed with status: ${STATUS:-unknown}"
  # The log holds the per-binary reasons; without it a rejection is undiagnosable.
  if [ -n "$SUBMISSION_ID" ]; then
    echo "Fetching notary log for $SUBMISSION_ID"
    xcrun notarytool log "$SUBMISSION_ID" \
      --key "$KEY_FILE" --key-id "$AC_KEY_ID" --issuer "$AC_ISSUER_ID" || true
  fi
  exit 1
fi

echo "Stapling ticket to $(basename "$TARGET")"
xcrun stapler staple "$TARGET"
xcrun stapler validate "$TARGET"
echo "Notarized and stapled: $TARGET"
