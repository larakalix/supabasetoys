# Release preparation

The repository has CI and a tag-triggered workflow that builds CLI archives and desktop installers, creates SHA-256 checksums, and uploads a **draft GitHub release**. Publishing the draft is the final maintainer action.

## One-time setup

1. Create/push the intended GitHub repository. The workflow uses `github.repository`; installers accept its explicit `OWNER/REPO` identity.
2. Enable Actions and configure a protected `release` environment with the desired reviewer policy.
3. Configure macOS signing/notarization secrets: `APPLE_CERTIFICATE` (base64 P12), `APPLE_CERTIFICATE_PASSWORD`, `APPLE_SIGNING_IDENTITY`, `APPLE_ID`, `APPLE_PASSWORD` (app-specific), and `APPLE_TEAM_ID`.
4. Configure Windows secrets: `WINDOWS_CERTIFICATE` (base64 PFX) and `WINDOWS_CERTIFICATE_PASSWORD`. The desktop executable is signed before NSIS repackaging, then the installer is signed; signing material is never committed.

Public-release desktop jobs refuse to proceed without signing credentials on macOS and Windows. Linux artifacts and CLI archives receive release checksums. Local development builds can use ad hoc/unsigned signing.

## Release checklist

- Run cross-platform Checks, including both supported Supabase versions' Docker acceptance jobs.
- Validate the native desktop flows on macOS, Windows, and Linux: add, Doctor, reviewed repair, start/stop/restart, Studio, environment export, logs, and diagnostics.
- Verify installer checksums, clean-user launch, and upgrades that preserve registration/config/data.
- Update CLI version, root/frontend package, and Wails product versions together; retain `go.mod`, `go.sum`, and `pnpm-lock.yaml`.
- Push a matching `vX.Y.Z` tag. Inspect all draft assets, signing/notarization results, and release notes before publishing.
- Test the versioned shell and PowerShell installers against the draft/published asset names. Document the chosen repository identity in installation examples.

The initial workspace has no Git remote or signing credentials. Workflow runs, signed installers, and public release publication require that external setup; local build success does not confirm them.


The Go CLI is built with CGO disabled for each target. Wails creates the native macOS app, Windows executable/NSIS installer, and Linux executable. Release scripts produce signed/notarized macOS DMGs and Linux AppImage/DEB packages. Linux AppImage tools and runtime have fixed SHA-256 expectations; if upstream changes an asset, the release stops until a maintainer verifies and repins it. AppImage and DEB builds require the host WebKitGTK 4.1 runtime. The macOS minimum is 13.0.
