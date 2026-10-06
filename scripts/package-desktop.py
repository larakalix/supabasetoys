#!/usr/bin/env python3
"""Package the Wails build; signing remains an explicit release workflow step."""
import argparse
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument('--os', required=True, choices=['macos', 'linux'])
parser.add_argument('--version', required=True)
parser.add_argument('--appimagetool')
parser.add_argument('--runtime-file')
parser.add_argument('--arch', choices=['arm64', 'x64', 'universal'])
args = parser.parse_args()
root = Path(__file__).resolve().parent.parent
out = root / 'release-assets'
out.mkdir(exist_ok=True)
version = args.version.removeprefix('v')
if args.os == 'macos':
    app = next((root / 'build/bin').glob('*.app'))
    architectures = subprocess.check_output(['lipo', '-archs', str(app / 'Contents/MacOS/supabase-toys-desktop')], text=True).split()
    arch = 'universal' if len(architectures) > 1 else ('arm64' if architectures == ['arm64'] else 'x64')
    if args.arch and args.arch != arch:
        raise RuntimeError(f'Expected {args.arch} app, found {arch}')
    with tempfile.TemporaryDirectory() as temporary:
        stage = Path(temporary)
        shutil.copytree(app, stage / 'Supabase Toys.app', symlinks=True)
        (stage / 'Applications').symlink_to('/Applications')
        subprocess.run(['hdiutil', 'create', '-volname', 'Supabase Toys', '-srcfolder', str(stage),
                        '-ov', '-format', 'UDZO', str(out / f'Supabase-Toys_{args.version}_macos_{arch}.dmg')], check=True)
else:
    if not args.appimagetool or not args.runtime_file:
        parser.error('--appimagetool and --runtime-file are required on Linux')
    with tempfile.TemporaryDirectory() as temporary:
        stage = Path(temporary)
        appdir = stage / 'SupabaseToys.AppDir'
        binarydir = appdir / 'usr/bin'
        binarydir.mkdir(parents=True)
        binary = binarydir / 'supabase-toys-desktop'
        shutil.copy2(root / 'build/bin/supabase-toys-desktop', binary)
        binary.chmod(0o755)
        icon = appdir / 'supabase-toys.png'
        shutil.copy2(root / 'build/appicon.png', icon)
        desktop = '[Desktop Entry]\nType=Application\nName=Supabase Toys\nExec=supabase-toys-desktop\nIcon=supabase-toys\nCategories=Development;\nTerminal=false\n'
        (appdir / 'supabase-toys.desktop').write_text(desktop)
        apprun = appdir / 'AppRun'
        apprun.write_text('#!/bin/sh\napp_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)\nexec "$app_dir/usr/bin/supabase-toys-desktop" "$@"\n')
        apprun.chmod(0o755)
        subprocess.run([str(Path(args.appimagetool).resolve()), '--runtime-file', str(Path(args.runtime_file).resolve()), str(appdir), str(out / f'Supabase-Toys_{args.version}_linux_x64.AppImage')],
                        env={**os.environ, 'ARCH': 'x86_64'}, check=True)
        deb = stage / 'deb'
        (deb / 'DEBIAN').mkdir(parents=True)
        (deb / 'usr/bin').mkdir(parents=True)
        (deb / 'usr/share/applications').mkdir(parents=True)
        (deb / 'usr/share/icons/hicolor/256x256/apps').mkdir(parents=True)
        shutil.copy2(binary, deb / 'usr/bin/supabase-toys-desktop')
        (deb / 'usr/share/applications/supabase-toys.desktop').write_text(desktop)
        shutil.copy2(icon, deb / 'usr/share/icons/hicolor/256x256/apps/supabase-toys.png')
        (deb / 'DEBIAN/control').write_text(f'Package: supabase-toys\nVersion: {version}\nArchitecture: amd64\nMaintainer: Supabase Toys contributors\nDepends: libgtk-3-0t64, libwebkit2gtk-4.1-0\nDescription: Local Supabase development without project conflicts\n')
        subprocess.run(['dpkg-deb', '--build', '--root-owner-group', str(deb), str(out / f'Supabase-Toys_{args.version}_linux_x64.deb')], check=True)
