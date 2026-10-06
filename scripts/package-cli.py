#!/usr/bin/env python3
import argparse, pathlib, tarfile, zipfile
parser = argparse.ArgumentParser()
parser.add_argument('--binary', required=True)
parser.add_argument('--version', required=True)
parser.add_argument('--os', required=True, choices=['macos','linux','windows'])
parser.add_argument('--arch', required=True, choices=['x64','arm64'])
args = parser.parse_args()
out = pathlib.Path('release-assets'); out.mkdir(exist_ok=True)
name = f'supabase-toys_{args.version}_{args.os}_{args.arch}'
if args.os == 'windows':
    with zipfile.ZipFile(out / (name + '.zip'), 'w', zipfile.ZIP_DEFLATED) as archive:
        archive.write(args.binary, 'supabase-toys.exe')
else:
    with tarfile.open(out / (name + '.tar.gz'), 'w:gz') as archive:
        archive.add(args.binary, arcname='supabase-toys')

