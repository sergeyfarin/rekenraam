#!/usr/bin/env python3
"""Disposable Compose install/upgrade/backup-restore drill; requires built images."""
import argparse
import csv
import http.cookiejar
import io
import json
import os
import pathlib
import secrets
import sqlite3
import subprocess
import tempfile
import time
import urllib.error
import urllib.request

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--old-image', required=True)
parser.add_argument('--new-image', required=True)
parser.add_argument('--old-schema', type=int, required=True)
parser.add_argument('--new-schema', type=int, required=True)
parser.add_argument('--port', type=int, default=16889)
args = parser.parse_args()
os.umask(0o077)
ROOT = pathlib.Path(__file__).resolve().parents[1]
WORK = pathlib.Path(tempfile.mkdtemp(prefix='rekenraam-deployment-audit-'))
PROJECT = 'rr-audit-' + secrets.token_hex(4)
BASE = f'http://127.0.0.1:{args.port}'
OLD, NEW = args.old_image, args.new_image
print('Evidence directory:', WORK, flush=True)
os.environ['SETUP_TOKEN'] = secrets.token_urlsafe(32)
PASSWORD = secrets.token_urlsafe(24)
jar = http.cookiejar.CookieJar()
opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))

def docker(*args, ok=True):
    result = subprocess.run(['docker', *args], capture_output=True, text=True)
    if ok and result.returncode:
        raise RuntimeError(result.stderr)
    return result

def compose(*args, ok=True):
    return docker('compose', '-p', PROJECT, '-f', str(ROOT/'deploy/docker/compose.yaml'),
                  '-f', str(WORK/'override.yaml'), *args, ok=ok)

def select(image):
    # JSON quoting is also valid YAML for the image scalar.
    (WORK/'override.yaml').write_text('services:\n  rekenraam:\n    image: '+json.dumps(image)+f'\n    ports: !override\n      - "127.0.0.1:{args.port}:16888"\n')

def api(path, body=None, method=None, token=None, expected=200):
    headers = {'Origin': BASE}
    # Browsers trust HTTP loopback for Secure cookies; stdlib does not. This
    # synthetic client explicitly sends its cookie jar only to BASE's loopback.
    if list(jar):
        headers['Cookie'] = '; '.join(c.name+'='+c.value for c in jar)
    if body is not None:
        headers['Content-Type'] = 'application/json'
    if token:
        headers['X-CSRF-Token'] = token
    if path == '/api/v1/setup/owner':
        headers['X-Setup-Token'] = os.environ['SETUP_TOKEN']
    request = urllib.request.Request(BASE+path, data=None if body is None else json.dumps(body).encode(),
                                     headers=headers, method=method)
    try:
        response = opener.open(request, timeout=30)
    except urllib.error.HTTPError as error:
        response = error
    data = response.read()
    assert response.status == expected, (path, response.status, data)
    if 'json' in response.headers.get('Content-Type', ''):
        return json.loads(data)
    return data

def mutate(path, body=None, expected=200):
    token = api('/api/v1/auth/session')['csrf_token']
    return api(path, body, method='POST', token=token, expected=expected)

def start(image):
    select(image)
    compose('up', '-d', '--no-build', '--wait', '--wait-timeout', '60')
    print('healthy:', image, flush=True)

def backup(label):
    run = mutate('/api/v1/maintenance/backups', expected=202)
    deadline = time.monotonic()+95
    while time.monotonic() < deadline:
        status = api('/api/v1/maintenance/backups')
        found = next((r for r in status['runs'] if r['id'] == run['id']), None)
        if found and found['status'] == 'completed':
            assert found['verified']
            result = compose('exec', '-T', 'rekenraam', '/app/rekenraam', 'verify-backup', '--from', found['target_path'])
            (WORK/(label+'-verify.txt')).write_text(result.stdout)
            print(label, result.stdout.splitlines()[:4], flush=True)
            return found['target_path']
        time.sleep(1)
    raise RuntimeError('backup did not complete: '+str(found))

def check(label):
    result = mutate('/api/v1/maintenance/self-check')
    assert result['status'] == 'passed', result
    (WORK/(label+'-self-check.json')).write_text(json.dumps(result, indent=2))
    print(label, 'self-check passed', flush=True)

def ledger(label):
    text = api('/api/v1/exports/ledger.csv').decode('utf-8-sig')
    rows = list(csv.DictReader(io.StringIO(text)))
    assert len(rows) == 2, rows
    assert sorted(r['quantity'] for r in rows) == ['-25.00', '25.00'], rows
    assert {r['description'] for r in rows} == {'Audit279 transfer'}, rows
    (WORK/(label+'-ledger.csv')).write_text(text)
    print(label, 'exact two-row ledger: -25.00 / +25.00', flush=True)
    # Head adds posting_version_id (#322). Preserve every pre-existing CSV
    # field; the additive identity column is not a changed financial figure.
    if 'posting_version_id' in rows[0]:
        assert sorted(r['posting_version_id'] for r in rows) == ['1', '2']
    return [{k: v for k, v in row.items() if k != 'posting_version_id'} for row in rows]

select(OLD)
try:
    for image in [OLD, NEW]:
        print('image:', image, docker('image', 'inspect', '--format', '{{.Id}}', image).stdout.strip(), flush=True)
    start(NEW)
    assert api('/api/v1/setup/status')['install_state'] == 'fresh'
    assert b'<html' in api('/')
    assert b'<html' in api('/app/ledger/transactions')
    print('fresh current Compose install: healthy, setup ready, embedded shell served', flush=True)
    compose('down', '-v')
    start(OLD)
    assert api('/api/v1/setup/status')['install_state'] == 'fresh'
    assert b'<html' in api('/')
    assert b'<html' in api('/app/ledger/transactions')
    api('/api/v1/audit279-missing', expected=404)
    api('/missing-audit279.js', expected=404)
    api('/api/v1/setup/owner', {'username': 'audit279', 'password': PASSWORD}, expected=201)
    mutate('/api/v1/setup/book', {'code': 'audit279', 'name': 'Audit279 book'}, expected=201)
    mutate('/api/v1/setup/currencies', {'default_currency_code': 'USD'}, expected=201)
    mutate('/api/v1/setup/system-accounts', expected=201)
    mutate('/api/v1/setup/categories', expected=201)
    currency = next(c for c in api('/api/v1/currencies')['currencies'] if c['code'] == 'USD')['id']
    accounts = [mutate('/api/v1/accounts', {'name': name, 'account_class': 'asset', 'account_kind': 'checking',
                'default_commodity_id': currency, 'allows_postings': True, 'opened_on': '2026-01-01'}, expected=201)
                for name in ['Audit279 checking', 'Audit279 savings']]
    mutate('/api/v1/transactions', {'transaction_date': '2026-01-02', 'description': 'Audit279 transfer',
           'journal_entries': [{'entry_date': '2026-01-02', 'postings': [
           {'account_id': account['id'], 'quantity_value': value, 'quantity_scale': 2, 'commodity_id': currency}
           for account, value in zip(accounts, [-2500, 2500])]}]}, expected=201)
    before = ledger('old')
    check('old')
    old_backup = backup('old')
    assert f'schema version:  {args.old_schema} ' in (WORK/'old-verify.txt').read_text()
    cid = compose('ps', '-q', 'rekenraam').stdout.strip()
    inspect = json.loads(docker('inspect', cid).stdout)[0]
    assert inspect['Config']['User'] == '10001:10001'
    assert inspect['HostConfig']['ReadonlyRootfs']
    assert inspect['HostConfig']['CapDrop'] == ['ALL']
    assert 'no-new-privileges:true' in inspect['HostConfig']['SecurityOpt']
    print('container security: non-root, read-only, caps dropped, no-new-privileges', flush=True)
    print(compose('exec', '-T', 'rekenraam', 'stat', '-c', '%a %u:%g %n', '/app/data',
                  '/app/data/rekenraam.sqlite', '/app/data/rekenraam.sqlite-wal', '/app/data/rekenraam.sqlite-shm').stdout, flush=True)
    compose('restart', 'rekenraam')
    time.sleep(2)
    assert ledger('restart') == before
    compose('stop', 'rekenraam')
    start(NEW)
    assert b'<html' in api('/')
    assert ledger('head') == before
    check('head')
    head_backup = backup('head')
    assert f'schema version:  {args.new_schema} ' in (WORK/'head-verify.txt').read_text()
    compose('stop', 'rekenraam')
    cid = compose('ps', '-aq', 'rekenraam').stdout.strip()
    docker('cp', cid+':/app/data', str(WORK/'post-upgrade'))
    database = sqlite3.connect(WORK/'post-upgrade/rekenraam.sqlite')
    assert database.execute('SELECT max(version_id) FROM goose_db_version WHERE is_applied=1').fetchone()[0] == args.new_schema
    assert database.execute('PRAGMA integrity_check').fetchall() == [('ok',)]
    assert database.execute('PRAGMA foreign_key_check').fetchall() == []
    database.close()
    select(OLD)
    newer = compose('run', '--rm', '--no-deps', 'rekenraam', 'restore', '--from', head_backup, ok=False)
    assert newer.returncode != 0, newer.stdout
    assert 'newer' in (newer.stdout+newer.stderr).lower()
    (WORK/'newer-refusal.txt').write_text(newer.stdout+newer.stderr)
    print('old binary refuses newer-schema backup', flush=True)
    restore = compose('run', '--rm', '--no-deps', 'rekenraam', 'restore', '--from', old_backup)
    (WORK/'restore.txt').write_text(restore.stdout)
    start(OLD)
    assert ledger('rollback') == before
    check('rollback')
    print(f'PASS: fresh install, seed, verified backup, restart, {args.old_schema}->{args.new_schema} upgrade, newer refusal, restore rollback', flush=True)
finally:
    (WORK/'container.log').write_text(compose('logs', '--no-color', ok=False).stdout)
    compose('down', '-v', ok=False)
