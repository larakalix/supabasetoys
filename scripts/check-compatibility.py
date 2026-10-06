#!/usr/bin/env python3
"""Check real CLI contracts without launching services. Pass one or more CLI paths."""
import json, os, pathlib, subprocess, sys, tempfile

for executable in sys.argv[1:]:
    executable = str(pathlib.Path(executable).resolve())
    with tempfile.TemporaryDirectory(prefix='toys-contract-') as root:
        env = {**os.environ, 'SUPABASE_HOME': root + '/cli-home', 'SUPA_TELEMETRY': 'off', 'SUPABASE_EXPERIMENTAL_STACK': '0'}
        def run(*args):
            return subprocess.check_output([executable, *args], env=env, stderr=subprocess.DEVNULL, text=True)
        version = run('--version').strip()
        assert version in ('2.119.0', '2.118.0'), version
        for command, flags in [('start', ['--exclude']), ('stop', ['--project-id']), ('status', [] )]:
            help_text = run(command, '--help')
            for flag in flags + ['--workdir', '--output']:
                assert flag in help_text, (version, command, flag)
        run('init', '--workdir', root)
        config = pathlib.Path(root, 'supabase/config.toml').read_text()
        assert 'project_id' in config and 'shadow_port' in config and '[local_smtp]' in config
        env['SUPABASE_EXPERIMENTAL_STACK'] = '1'
        for command in ('start', 'stop', 'status'):
            assert '--stack-id' in run('stack', command, '--help')
        stacks = json.loads(run('stack', 'list', '--output-format', 'json'))
        assert isinstance(stacks['stacks'], list)
        print(f'Supabase CLI {version}: standard + experimental command contracts verified')

