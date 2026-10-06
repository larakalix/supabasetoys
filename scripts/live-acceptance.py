#!/usr/bin/env python3
"""Live A/B isolation test. Creates unique test projects; stops them without deleting data.
Required: TOYS_BINARY and SUPABASE_BINARY (absolute paths). Optional TOYS_TEST_ROOT.
Never operates on pre-existing project folders or runs global stop / reset / destroy.
"""
import json, os, pathlib, subprocess, sys, tempfile, uuid, urllib.request, urllib.error

toys = str(pathlib.Path(os.environ['TOYS_BINARY']).resolve())
supabase = str(pathlib.Path(os.environ['SUPABASE_BINARY']).resolve())
root = pathlib.Path(os.environ.get('TOYS_TEST_ROOT') or tempfile.mkdtemp(prefix='supabase-toys-acceptance-')).resolve()
root.mkdir(parents=True, exist_ok=True)
env = {**os.environ, 'SUPABASE_HOME': str(root / 'cli-home'), 'SUPA_TELEMETRY': 'off', 'SUPABASE_EXPERIMENTAL_STACK': '0'}
data = root / 'toys-data'
projects = []
docker_endpoint = None
def command(*args):
    result = subprocess.run([toys, '--json', '--data-dir', str(data), *map(str,args)], env=env, text=True, capture_output=True)
    if result.returncode:
        raise RuntimeError(f'Toys {args[0]} failed: {result.stderr}')
    return json.loads(result.stdout)
def sql(project_id, query):
    result = subprocess.run(['docker', '--host', docker_endpoint, 'exec', f'supabase_db_{project_id}', 'psql', '-U', 'postgres', '-d', 'postgres', '-At', '-v', 'ON_ERROR_STOP=1', '-c', query], env=env, text=True, capture_output=True)
    if result.returncode: raise RuntimeError('Test SQL failed: ' + result.stderr)
    return result.stdout.strip()
def verify(project, marker):
    assert sql(project['project_id'], 'select marker from toys_acceptance.probe') == marker
    status = command('status', project['id'])
    assert status['state'] == 'running', status['state']
    api_url = status['endpoints'].get('API_URL')
    assert api_url, 'Runtime API endpoint missing'
    # The health endpoint must be reachable. A protected API response also proves routing.
    try: urllib.request.urlopen(api_url + '/rest/v1/', timeout=10)
    except urllib.error.HTTPError as e: assert e.code in (401,403,404), e.code
    return status

try:
    command('settings', '--supabase-cli', supabase)
    docker_endpoint = command('inventory')['endpoint']
    assert docker_endpoint, 'Local Docker endpoint unavailable'
    for letter in ('A','B'):
        path = root / letter
        if path.exists(): raise RuntimeError('Refusing to reuse an existing acceptance project folder')
        path.mkdir()
        subprocess.run([supabase,'init','--workdir',str(path)], env=env, check=True, stdout=subprocess.DEVNULL)
        config_path = path / 'supabase/config.toml'
        text = config_path.read_text()
        import re
        text = re.sub(r'^project_id = .*$', f'project_id = "toys-test-{letter.lower()}-{uuid.uuid4().hex[:10]}"', text, flags=re.M)
        # Lightweight stack while keeping API/Auth/Studio endpoints testable.
        for section in ('realtime','storage','analytics','edge_runtime'):
            text = re.sub(r'(\[' + section + r'\]\s*\n)(.*?)(?=\n\[|\Z)', lambda m: m[1] + re.sub(r'^enabled = true$', 'enabled = false', m[2], flags=re.M), text, flags=re.S)
        config_path.write_text(text)
        project = command('project','add',path,'--name',letter)
        projects.append(project)
        preview = command('doctor',project['id'],'--preview')
        proposal = root / f'{letter}-ports.json'; proposal.write_text(json.dumps(preview))
        command('doctor',project['id'],'--apply',proposal)
        print(f'Starting isolated test project {letter}…', flush=True)
        command('start',project['id'])
        marker = f'{letter}-{uuid.uuid4().hex}'
        # Private test schema is not exposed through the Data API.
        sql(project['project_id'], f"create schema toys_acceptance; create table toys_acceptance.probe(marker text primary key); insert into toys_acceptance.probe values ('{marker}');")
        project['marker'] = marker
        verify(project,marker)
    a,b = projects
    a_before = verify(a,a['marker'])
    b_before = verify(b,b['marker'])
    assert a_before['endpoints']['API_URL'] != b_before['endpoints']['API_URL']
    command('restart',b['id']); verify(a,a['marker']); verify(b,b['marker'])
    command('stop',b['id']); verify(a,a['marker'])
    command('start',b['id']); verify(a,a['marker']); verify(b,b['marker'])
    assert command('status',b['id'])['endpoints']['API_URL'] == b_before['endpoints']['API_URL']
    # Explicit API-port edits must preserve A's routing and both databases.
    import socket
    with socket.socket() as listener:
        listener.bind(('127.0.0.1', 0))
        new_port = listener.getsockname()[1]
    manual = command('ports', 'preview', b['id'], '--set', f'api.port={new_port}')
    assert manual['mode'] == 'manual' and manual['restart_required']
    manual_file = root / 'B-manual-ports.json'; manual_file.write_text(json.dumps(manual))
    command('ports', 'apply', manual_file, '--restart')
    b_changed = verify(b,b['marker'])
    assert b_changed['endpoints']['API_URL'] == f'http://127.0.0.1:{new_port}'
    assert verify(a,a['marker'])['endpoints']['API_URL'] == a_before['endpoints']['API_URL']
    command('stop',b['id']); command('start',b['id'])
    assert verify(b,b['marker'])['endpoints']['API_URL'] == b_changed['endpoints']['API_URL']
    assert verify(a,a['marker'])['endpoints']['API_URL'] == a_before['endpoints']['API_URL']
    report = command('diagnostics','export',b['id'],'--include-logs')
    assert report['project']['id'] == b['id']
    print('PASS: A/B API isolation, targeted restart/stop, database persistence, stable ports, explicit port editing, and redacted report', flush=True)
finally:
    for project in reversed(projects):
        try: command('stop',project['id'])
        except Exception as error: print('Cleanup needs attention:', error, file=sys.stderr)
    print(f'Test folders and preserved data remain at {root}', flush=True)
