#!/usr/bin/env python3
import json
from pathlib import Path
import re
import sys
expected = sys.argv[1].removeprefix('v')
versions = [json.loads(Path(path).read_text())['version'] for path in ('package.json', 'apps/desktop/package.json')]
versions.append(json.loads(Path('wails.json').read_text())['info']['productVersion'])
versions.append(re.search(r'const version = "([^"]+)"', Path('cmd/supabase-toys/main.go').read_text())[1])
assert all(version == expected for version in versions), f'Version mismatch: tag {expected}, packages {versions}'
