#!/usr/bin/env python3
"""Fetch checksum-pinned Linux release tools; never accept changed downloads."""
import hashlib
from pathlib import Path
import urllib.request

assets = {
    'appimagetool': ('https://github.com/AppImage/appimagetool/releases/download/1.9.1/appimagetool-x86_64.AppImage',
                     'ed4ce84f0d9caff66f50bcca6ff6f35aae54ce8135408b3fa33abfc3cb384eb0'),
    'runtime': ('https://github.com/AppImage/type2-runtime/releases/download/continuous/runtime-x86_64',
                '156f4bdbde9c52d01814600013e0a273f0118dc2de98975f3c8c63427ec79074'),
}
root = Path('desktop-tools')
root.mkdir(exist_ok=True)
for name, (url, expected) in assets.items():
    with urllib.request.urlopen(url, timeout=60) as response:
        data = response.read()
    if hashlib.sha256(data).hexdigest() != expected:
        raise RuntimeError(f'{name} checksum changed; inspect upstream and explicitly repin before releasing')
    (root / name).write_bytes(data)
