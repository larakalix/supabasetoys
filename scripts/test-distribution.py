#!/usr/bin/env python3
"""Offline release archive and Unix installer smoke tests."""
import hashlib
import os
from pathlib import Path
import platform
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parent.parent


class DistributionTests(unittest.TestCase):
    def test_archive_layout(self):
        import tarfile
        import zipfile
        with tempfile.TemporaryDirectory() as temporary:
            work = Path(temporary)
            binary = work / 'binary'
            binary.write_bytes(b'fixture')
            for operating_system in ('macos', 'linux', 'windows'):
                subprocess.run([sys.executable, str(ROOT / 'scripts/package-cli.py'),
                                '--binary', str(binary), '--version', 'v0.1.0',
                                '--os', operating_system, '--arch', 'x64'], cwd=work, check=True)
                extension = 'zip' if operating_system == 'windows' else 'tar.gz'
                archive = work / 'release-assets' / f'supabase-toys_v0.1.0_{operating_system}_x64.{extension}'
                if extension == 'zip':
                    with zipfile.ZipFile(archive) as package:
                        self.assertEqual(package.namelist(), ['supabase-toys.exe'])
                        self.assertEqual(package.read('supabase-toys.exe'), b'fixture')
                else:
                    with tarfile.open(archive) as package:
                        self.assertEqual(package.getnames(), ['supabase-toys'])
                        self.assertEqual(package.extractfile('supabase-toys').read(), b'fixture')

    @unittest.skipIf(os.name == 'nt', 'Unix installer; use PowerShell on Windows')
    def test_installer_verifies_before_replacing_binary(self):
        with tempfile.TemporaryDirectory() as temporary:
            work = Path(temporary)
            binary = work / 'binary'
            binary.write_text('#!/bin/sh\necho fixture-ok\n')
            binary.chmod(0o755)
            operating_system = 'macos' if platform.system() == 'Darwin' else 'linux'
            arch = 'arm64' if platform.machine().lower() in ('arm64', 'aarch64') else 'x64'
            subprocess.run([sys.executable, str(ROOT / 'scripts/package-cli.py'),
                            '--binary', str(binary), '--version', 'v0.1.0',
                            '--os', operating_system, '--arch', arch], cwd=work, check=True)
            assets = work / 'release-assets'
            archive = next(assets.glob('*.tar.gz'))
            checksums = assets / 'SHA256SUMS'
            checksums.write_text(f'{hashlib.sha256(archive.read_bytes()).hexdigest()}  {archive.name}\n')
            mock_bin = work / 'mock-bin'
            mock_bin.mkdir()
            curl = mock_bin / 'curl'
            curl.write_text(f'#!{sys.executable}\nimport os,pathlib,shutil,sys\nargs=sys.argv[1:]\nurl=next(x for x in args if x.startswith("https://"))\nshutil.copyfile(pathlib.Path(os.environ["TOYS_FIXTURE_ASSETS"])/url.rsplit("/",1)[1], args[args.index("-o")+1])\n')
            curl.chmod(0o755)
            environment = dict(os.environ, PATH=str(mock_bin) + os.pathsep + os.environ['PATH'], TOYS_FIXTURE_ASSETS=str(assets))
            destination = work / 'installed'
            command = ['sh', str(ROOT / 'scripts/install.sh'), '--repo', 'example/toys', '--version', 'v0.1.0', '--dir', str(destination)]
            subprocess.run(command, env=environment, check=True, capture_output=True)
            installed = destination / 'supabase-toys'
            self.assertEqual(subprocess.check_output([str(installed)], text=True).strip(), 'fixture-ok')
            before = installed.read_bytes()
            checksums.write_text('0' * 64 + f'  {archive.name}\n')
            result = subprocess.run(command, env=environment, capture_output=True, text=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn('Checksum verification failed', result.stderr)
            self.assertEqual(installed.read_bytes(), before)


if __name__ == '__main__':
    unittest.main()
