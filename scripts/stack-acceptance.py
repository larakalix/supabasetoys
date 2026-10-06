#!/usr/bin/env python3
"""Verify explicit adoption/lifecycle of a new experimental Docker stack; preserve data."""
import json, os, pathlib, subprocess, tempfile
toys = str(pathlib.Path(os.environ['TOYS_BINARY']).resolve())
supabase = str(pathlib.Path(os.environ['SUPABASE_BINARY']).resolve())
root = pathlib.Path(tempfile.mkdtemp(prefix='toys-stack-acceptance-')).resolve()
project = root / 'project'; project.mkdir()
env = {**os.environ, 'SUPABASE_HOME': str(root / 'cli-home'), 'SUPA_TELEMETRY': 'off', 'SUPABASE_EXPERIMENTAL_STACK': '1'}
def upstream(*args):
    p = subprocess.run([supabase,*args,'--workdir',str(project)], env=env, text=True, capture_output=True)
    if p.returncode: raise RuntimeError('Upstream stack command failed; inspect test logs in ' + str(root))
    return p.stdout
def command(*args):
    p = subprocess.run([toys,'--json','--data-dir',str(root/'data'),*args], env=env, text=True, capture_output=True)
    if p.returncode: raise RuntimeError(p.stderr)
    return json.loads(p.stdout)
stack_id = None
try:
    command('settings','--supabase-cli',supabase)
    inventory = command('inventory')
    assert inventory['available'], 'Local Docker unavailable'
    env['DOCKER_HOST'] = inventory['endpoint']; env.pop('DOCKER_CONTEXT',None)
    upstream('init')
    print('Creating an isolated experimental Docker stack…',flush=True)
    upstream('stack','start','--stack','toys-test','--runtime','docker','--eager','--exclude','realtime,storage,functions,analytics,pooler')
    stacks = json.loads(upstream('stack','list','--output-format','json'))['stacks']
    stack_id = next(s['id'] for s in stacks if s['project_root']==str(project))
    registered = command('project','add',str(project),'--stack-id',stack_id)
    initial = command('status',registered['id'],'--resources')
    assert initial['state']=='running', initial['state']
    assert initial['services'], 'Stack labels were not discovered'
    assert initial['endpoints'].get('API_URL'), 'Stack endpoint JSON contract changed'
    command('stop',registered['id'])
    command('start',registered['id'])
    resumed = command('status',registered['id'])
    assert resumed['endpoints']['API_URL']==initial['endpoints']['API_URL']
    print('PASS: experimental Docker stack ownership, labels, JSON endpoints, targeted stop/resume',flush=True)
finally:
    if stack_id:
        upstream('stack','stop','--stack-id',stack_id)
    print(f'Stack test folders and preserved data: {root}',flush=True)
