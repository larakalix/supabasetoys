#!/bin/bash
set -euo pipefail
for name in APPLE_CERTIFICATE APPLE_CERTIFICATE_PASSWORD APPLE_SIGNING_IDENTITY APPLE_ID APPLE_PASSWORD APPLE_TEAM_ID; do
 test -n "${!name:-}" || { echo "Missing release secret: $name" >&2; exit 1; }
done
signing_dir=$(mktemp -d)
trap 'rm -rf "$signing_dir"' EXIT
python3 - "$signing_dir/certificate.p12" <<'PY'
import base64, os, pathlib, sys
pathlib.Path(sys.argv[1]).write_bytes(base64.b64decode(os.environ['APPLE_CERTIFICATE']))
PY
keychain="$signing_dir/signing.keychain-db"
security create-keychain -p "$APPLE_CERTIFICATE_PASSWORD" "$keychain"
security set-keychain-settings -lut 21600 "$keychain"
security unlock-keychain -p "$APPLE_CERTIFICATE_PASSWORD" "$keychain"
security import "$signing_dir/certificate.p12" -k "$keychain" -P "$APPLE_CERTIFICATE_PASSWORD" -T /usr/bin/codesign
security set-key-partition-list -S apple-tool:,apple: -s -k "$APPLE_CERTIFICATE_PASSWORD" "$keychain"
security list-keychains -d user -s "$keychain" "$HOME/Library/Keychains/login.keychain-db"
app='build/bin/Supabase Toys.app'
codesign --force --deep --options runtime --timestamp --sign "$APPLE_SIGNING_IDENTITY" --entitlements build/darwin/entitlements.plist "$app"
codesign --verify --deep --strict "$app"
ditto -c -k --keepParent "$app" "$signing_dir/app.zip"
xcrun notarytool submit "$signing_dir/app.zip" --apple-id "$APPLE_ID" --password "$APPLE_PASSWORD" --team-id "$APPLE_TEAM_ID" --wait
xcrun stapler staple "$app"

python3 scripts/package-desktop.py --os macos --version "$GITHUB_REF_NAME" --arch universal
codesign --force --timestamp --sign "$APPLE_SIGNING_IDENTITY" release-assets/*.dmg
for disk_image in release-assets/*.dmg; do
 xcrun notarytool submit "$disk_image" --apple-id "$APPLE_ID" --password "$APPLE_PASSWORD" --team-id "$APPLE_TEAM_ID" --wait
 xcrun stapler staple "$disk_image"
done
