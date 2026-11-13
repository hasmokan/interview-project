#!/usr/bin/env python3
"""Real-binary CLI acceptance and Skill command-workflow replay; no production data."""
import argparse
from contextlib import contextmanager
from datetime import datetime
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import platform
import re
import secrets
import socket
import sqlite3
import subprocess
import tempfile
import threading
import time
import urllib.error
import urllib.request
from zoneinfo import ZoneInfo

from acceptance import fixtures

ROOT = Path(__file__).resolve().parents[1]


def digest(data):
    return hashlib.sha256(data).hexdigest()


def http_request(origin, path, method='GET', headers=None, body=None):
    request = urllib.request.Request(origin + path, data=body, headers=headers or {}, method=method)
    try:
        response = urllib.request.urlopen(request, timeout=5)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        return response.status, dict(response.headers), response.read()


@contextmanager
def fault_server(respond):
    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass

        def handle_request(self):
            body = b''
            if self.headers.get('Transfer-Encoding', '').lower() == 'chunked':
                chunks = []
                while True:
                    size = int(self.rfile.readline().strip().split(b';')[0], 16)
                    if size == 0:
                        self.rfile.readline()
                        break
                    chunks.append(self.rfile.read(size))
                    self.rfile.read(2)
                body = b''.join(chunks)
            elif self.headers.get('Content-Length'):
                body = self.rfile.read(int(self.headers['Content-Length']))
            status, headers, result = respond(self.command, self.path, dict(self.headers), body)
            self.send_response(status)
            for key, value in headers.items():
                self.send_header(key, value)
            self.send_header('Content-Length', str(len(result)))
            self.end_headers()
            try:
                self.wfile.write(result)
            except (BrokenPipeError, ConnectionResetError):
                pass

        do_GET = do_POST = do_DELETE = handle_request

    server = ThreadingHTTPServer(('127.0.0.1', 0), Handler)
    thread = threading.Thread(target=lambda: server.serve_forever(poll_interval=.05), daemon=True)
    thread.start()
    try:
        yield f'http://127.0.0.1:{server.server_port}'
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=2)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--bin-dir', default='bin')
    parser.add_argument('--output', default='work/agent-cli/results.json')
    args = parser.parse_args()
    binary_dir = (ROOT / args.bin_dir).resolve()
    cli = binary_dir / 'fileshare'
    server = binary_dir / 'server'
    output = (ROOT / args.output).resolve()
    output.parent.mkdir(parents=True, exist_ok=True)
    now = lambda: datetime.now(ZoneInfo('Asia/Shanghai')).isoformat(timespec='seconds')
    results = {
        'started_at': now(),
        'tested_commit': subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip(),
        'environment': {'platform': platform.platform(), 'python': platform.python_version(),
                        'go': subprocess.check_output(['go', 'version'], text=True).strip(), 'upload_limit_bytes': 1048576},
        'binary_sha256': {'fileshare': digest(cli.read_bytes()), 'server': digest(server.read_bytes())},
        'skill_sha256': digest((ROOT / 'skills/fileshare/SKILL.md').read_bytes()),
        'scope': 'Real CLI processes against an isolated real service; Skill command workflows replayed from its instructions. Independent model selection and clarification behavior are not evaluated.',
        'cases': [],
    }
    active = None
    state = {}
    with tempfile.TemporaryDirectory(prefix='fileshare-agent-cli-') as temporary:
        temporary = Path(temporary)
        fixtures(temporary)
        token, password, session_key = secrets.token_urlsafe(32), secrets.token_urlsafe(20), secrets.token_hex(32)
        secret_values = [token, password, session_key]

        def clean(value):
            text = json.dumps(value, ensure_ascii=False)
            for secret in secret_values:
                text = text.replace(secret, '<redacted>')
            text = re.sub(r'/s/[A-Za-z0-9_-]{43}', '/s/<redacted>', text)
            text = text.replace(str(temporary), '<sandbox>').replace(str(ROOT), '<project>')
            return json.loads(text)

        def case(identifier, title):
            def execute(function):
                nonlocal active
                active = {'id': identifier, 'name': title, 'commands': [], 'observations': {}}
                started = time.monotonic()
                try:
                    function()
                    active['status'] = 'PASS'
                except Exception as error:
                    active['status'] = 'FAIL'
                    active['error'] = str(error) or type(error).__name__
                active['duration_ms'] = round((time.monotonic() - started) * 1000)
                results['cases'].append(clean(active))
                print(f"{active['status']} {identifier}: {title}", flush=True)
                active = None
                return function
            return execute

        with socket.socket() as sock:
            sock.bind(('127.0.0.1', 0))
            port = sock.getsockname()[1]
        origin = f'http://127.0.0.1:{port}'
        env = dict(os.environ, FILESHARE_SERVER=origin, FILESHARE_TOKEN=token,
                   FILESHARE_CONFIG=str(temporary / 'cli.json'))
        password_hash = subprocess.check_output([server, 'hash-password'], input=password.encode()).decode().strip()
        config = dict(os.environ, FILESHARE_ADDR=f'127.0.0.1:{port}', FILESHARE_PUBLIC_URL=origin,
                      FILESHARE_DATA_DIR=str(temporary / 'data'), FILESHARE_WEB_DIR=str(ROOT / 'web/dist/client'),
                      FILESHARE_PASSWORD_HASH=password_hash, FILESHARE_TOKEN_HASH=digest(token.encode()),
                      FILESHARE_SESSION_KEY=session_key, FILESHARE_MAX_UPLOAD_BYTES='1048576')
        log = (temporary / 'server.log').open('w+')
        process = subprocess.Popen([server], env=config, cwd=ROOT, stdout=log, stderr=log)

        def run(*arguments, ok=True, custom_env=None):
            command = [str(cli), *map(str, arguments)]
            completed = subprocess.run(command, env=custom_env or env, text=True, capture_output=True, timeout=10)
            active['commands'].append({'argv': ['fileshare', *map(str, arguments)],
                                       'exit_code': completed.returncode, 'stdout': completed.stdout, 'stderr': completed.stderr})
            assert all(secret not in completed.stdout + completed.stderr for secret in secret_values), 'CLI exposed a credential'
            if ok:
                assert completed.returncode == 0, completed.stderr
                assert completed.stderr == '', 'success wrote unexpected stderr'
                return json.loads(completed.stdout)
            assert completed.returncode != 0, 'expected nonzero exit'
            assert not completed.stdout, 'failure emitted success output'
            assert completed.stderr, 'failure missing stderr'
            return completed.stderr

        def query(*arguments):
            return run(*arguments, '--json')

        def count():
            return query('list', '--page-size', '100')['total']

        def content(file, expected=200):
            url = file['share_url']
            path = '/api/public/shares/' + url.rsplit('/', 1)[1] + '/content'
            status, headers, body = http_request(origin, path)
            active['observations'].setdefault('anonymous_content', []).append({'status': status, 'body_sha256': digest(body)})
            assert status == expected, (status, expected)
            return body

        try:
            for attempt in range(100):
                try:
                    if http_request(origin, '/healthz')[0] == 200:
                        break
                except OSError:
                    pass
                time.sleep(.1)
            else:
                raise RuntimeError('isolated server did not start')

            @case('C01', 'Missing configuration produces a clear nonzero error')
            def _():
                e = dict(env, FILESHARE_SERVER='', FILESHARE_TOKEN='')
                assert 'configure' in run('list', '--json', ok=False, custom_env=e)

            @case('C02', 'Persist CLI configuration with permissions 0600 and no token echo')
            def _():
                result = query('config', 'set', '--server', origin, '--token', token)
                assert result['configured'] is True
                assert (temporary / 'cli.json').stat().st_mode & 0o777 == 0o600
                e = dict(env, FILESHARE_SERVER='', FILESHARE_TOKEN='')
                assert run('list', '--json', custom_env=e)['total'] == 0
                active['observations']['config_mode'] = '0600'

            @case('C03', 'Environment server/token override stored configuration')
            def _():
                path = temporary / 'wrong.json'
                path.write_text(json.dumps({'server': 'http://127.0.0.1:1', 'token': 'deliberately-invalid'}))
                assert run('list', '--json', custom_env=dict(env, FILESHARE_CONFIG=str(path)))['total'] == 0

            @case('C04', 'Upload from a path containing Chinese characters and spaces; private by default')
            def _():
                folder = temporary / '验收 文件夹'
                folder.mkdir()
                path = folder / '验收 报告.pdf'
                path.write_bytes((temporary / 'report.pdf').read_bytes())
                f = query('upload', path)
                assert f['name'] == path.name and f['share_status'] == 'unshared' and f['share_url'] is None
                assert f['size_bytes'] == path.stat().st_size
                state['private'] = f
                assert query('get', f['file_id'])['share_url'] is None
                assert http_request(origin, '/files/' + f['file_id'])[0] == 404

            @case('C05', 'Skill upload-and-share command returns an accessible original PDF')
            def _():
                f = query('upload', temporary / 'report.pdf', '--share')
                assert f['share_status'] == 'shared' and f['share_url'].startswith(origin + '/s/')
                assert content(f) == (temporary / 'report.pdf').read_bytes()
                active['observations']['original_sha256'] = digest((temporary / 'report.pdf').read_bytes())
                state['shared'] = f

            @case('C06', 'Search/list/get return the original ID/link without reuploading')
            def _():
                before = count()
                p = query('list', '--search', 'report.pdf')
                assert any(f['file_id'] == state['shared']['file_id'] for f in p['items'])
                assert query('get', state['shared']['file_id'])['share_url'] == state['shared']['share_url']
                assert count() == before

            @case('C07', 'Repeated share is idempotent')
            def _():
                f = state['shared']
                assert query('share', f['file_id'])['share_url'] == f['share_url']
                assert query('share', f['file_id'])['share_url'] == f['share_url']

            @case('C08', 'Revoke keeps the file, rejects old content, and supports repeated revoke')
            def _():
                f = state['shared']
                before = count()
                for _ in range(2):
                    revoked = query('revoke', f['file_id'])
                    assert revoked['share_status'] == 'unshared' and revoked['share_url'] is None
                assert query('get', f['file_id'])['file_id'] == f['file_id']
                assert count() == before
                content(f, 404)
                path = '/api/public/shares/' + f['share_url'].rsplit('/', 1)[1]
                for method, suffix in [('GET', ''), ('HEAD', '/content'), ('GET', '/content')]:
                    assert http_request(origin, path + suffix, method, {'Range': 'bytes=0-4'})[0] == 404

            @case('C09', 'Reshare creates a new link while the old link stays invalid')
            def _():
                old = state['shared']
                fresh = query('share', old['file_id'])
                assert fresh['share_url'] != old['share_url']
                content(old, 404)
                assert content(fresh) == (temporary / 'report.pdf').read_bytes()
                state['shared'] = fresh
                active['observations']['link_changed'] = True

            @case('C10', 'Same-name candidates expose distinct IDs with no query side effects')
            def _():
                f = query('upload', temporary / 'report.pdf')
                before = count()
                candidates = query('list', '--search', 'report.pdf')['items']
                assert len(candidates) == 2 and len({x['file_id'] for x in candidates}) == 2
                assert count() == before
                assert query('get', state['shared']['file_id'])['share_status'] == 'shared'
                assert query('get', f['file_id'])['share_status'] == 'unshared'
                active['observations']['candidates'] = [{'file_id': c['file_id'], 'share_status': c['share_status']} for c in candidates]
                state['duplicate'] = f

            @case('C11', 'Pagination can retrieve all candidates without missing IDs')
            def _():
                first = query('list', '--search', 'report.pdf', '--page-size', '1', '--page', '1')
                second = query('list', '--search', 'report.pdf', '--page-size', '1', '--page', '2')
                empty = query('list', '--search', 'report.pdf', '--page-size', '1', '--page', '3')
                assert first['total'] == second['total'] == 2
                assert first['items'][0]['file_id'] != second['items'][0]['file_id']
                assert empty['items'] == []

            @case('C12', 'Explicit delete invalidates its link and leaves the same-name sibling intact')
            def _():
                f = state['shared']
                assert query('delete', f['file_id'])['deleted'] is True
                assert query('delete', f['file_id'])['deleted'] is True
                content(f, 404)
                assert '404' in run('get', f['file_id'], '--json', ok=False)
                assert '404' in run('share', f['file_id'], '--json', ok=False)
                assert query('get', state['duplicate']['file_id'])['share_status'] == 'unshared'
                with sqlite3.connect(temporary / 'data/fileshare.db') as db:
                    key, = db.execute('SELECT storage_key FROM files WHERE id=?', (f['file_id'],)).fetchone()
                assert not (temporary / 'data/files' / key).exists()

            @case('C13', 'Missing local path fails without uploading')
            def _():
                before = count()
                assert 'cannot open local file' in run('upload', temporary / 'missing.pdf', '--share', '--json', ok=False)
                assert count() == before

            @case('C14', 'Directory paths fail without uploading')
            def _():
                before = count()
                assert 'regular file' in run('upload', temporary, '--json', ok=False)
                assert count() == before

            @case('C15', 'Invalid token is rejected for every management command')
            def _():
                e = dict(env, FILESHARE_TOKEN='invalid-token')
                before = count()
                fid = state['private']['file_id']
                commands = [('list',), ('get', fid), ('share', fid), ('revoke', fid), ('delete', fid), ('upload', temporary / 'report.pdf')]
                for command in commands:
                    assert '401' in run(*command, '--json', ok=False, custom_env=e)
                assert count() == before and query('get', fid)['share_status'] == 'unshared'

            @case('C16', 'Exact upload limit succeeds; one byte over fails with 413 and no extra record')
            def _():
                path = temporary / 'boundary.bin'
                path.write_bytes(b'x' * 1048576)
                assert query('upload', path)['size_bytes'] == 1048576
                before = count()
                path.write_bytes(b'x' * 1048577)
                assert '413' in run('upload', path, '--share', '--json', ok=False)
                assert count() == before
                active['observations'].update({'limit_bytes': 1048576, 'rejected_bytes': 1048577})

            @case('C17', 'An unreachable service fails clearly without a fake result')
            def _():
                with socket.socket() as closed:
                    closed.bind(('127.0.0.1', 0))
                    e = dict(env, FILESHARE_SERVER=f'http://127.0.0.1:{closed.getsockname()[1]}')
                    assert 'request failed' in run('list', '--timeout', '1s', '--json', ok=False, custom_env=e)

            @case('C18', 'Unknown file ID returns a nonzero 404 error')
            def _():
                for command in ['get', 'share', 'revoke', 'delete']:
                    assert '404' in run(command, 'missing-file-id', '--json', ok=False)

            @case('C19', 'Missing arguments, unknown flags and invalid timeout fail clearly')
            def _():
                for command in [('get', '--json'), ('upload', '--unknown', '--json'), ('list', '--timeout', '0s', '--json'), ('list', '--page', '0', '--json')]:
                    run(*command, ok=False)

            @case('C20', 'JSON flags work before or after the file ID')
            def _():
                fid = state['private']['file_id']
                assert run('get', '--json', fid) == run('get', fid, '--json')

            @case('C21', 'Upload-success/share-failure reports the ID; retry shares it without reuploading')
            def _():
                failed_once = False
                def proxy(method, path, headers, body):
                    nonlocal failed_once
                    if method == 'POST' and path.endswith('/share') and not failed_once:
                        failed_once = True
                        return 500, {'Content-Type': 'application/json'}, b'{"code":"injected_failure","message":"controlled share failure"}'
                    status, response_headers, response_body = http_request(origin, path, method,
                        {'Authorization': headers.get('Authorization', ''), 'Content-Type': headers.get('Content-Type', '')}, body if method == 'POST' else None)
                    return status, {'Content-Type': response_headers.get('Content-Type', 'application/json')}, response_body
                before = count()
                with fault_server(proxy) as proxy_origin:
                    e = dict(env, FILESHARE_SERVER=proxy_origin)
                    error = run('upload', temporary / 'report.pdf', '--share', '--json', ok=False, custom_env=e)
                    match = re.search(r'file_id=([a-f0-9-]{36})', error)
                    assert match and 'retry fileshare share' in error
                    fid = match.group(1)
                    assert query('get', fid)['share_status'] == 'unshared' and count() == before + 1
                    shared = run('share', fid, '--json', custom_env=e)
                    assert content(shared) == (temporary / 'report.pdf').read_bytes()
                    assert count() == before + 1
                    active['observations']['recovered_file_id'] = fid

            @case('C22', 'Configured request timeout produces a bounded failure')
            def _():
                def slow(*args):
                    time.sleep(.3)
                    return 200, {'Content-Type': 'application/json'}, b'{"items":[],"total":0}'
                with fault_server(slow) as slow_origin:
                    started = time.monotonic()
                    assert 'request failed' in run('list', '--timeout', '50ms', '--json', ok=False, custom_env=dict(env, FILESHARE_SERVER=slow_origin))
                    assert time.monotonic() - started < 2

            @case('C23', 'Malformed API JSON fails without a success payload')
            def _():
                with fault_server(lambda *args: (200, {'Content-Type': 'text/plain'}, b'not json')) as bad_origin:
                    assert 'invalid API response' in run('list', '--json', ok=False, custom_env=dict(env, FILESHARE_SERVER=bad_origin))

            @case('C24', 'HTTP redirects are not followed with a management token')
            def _():
                target_hits = []
                def target(*args):
                    target_hits.append(True)
                    return 200, {'Content-Type': 'application/json'}, b'{}'
                with fault_server(target) as target_origin:
                    with fault_server(lambda *args: (302, {'Location': target_origin, 'Content-Type': 'application/json'}, b'{"code":"redirect","message":"redirect blocked"}')) as redirect_origin:
                        run('list', '--json', ok=False, custom_env=dict(env, FILESHARE_SERVER=redirect_origin))
                assert target_hits == []

            @case('C25', 'Search treats percent and underscore as literal filename characters')
            def _():
                for name in ['100%_报告.txt', '100ab报告.txt']:
                    path = temporary / name
                    path.write_text('literal filename search\n')
                    query('upload', path)
                found = query('list', '--search', '%_')
                assert found['total'] == 1 and found['items'][0]['name'] == '100%_报告.txt'

            @case('C26', 'No credential appears in captured output or server logs; only logical foreign keys')
            def _():
                log.flush()
                text = (temporary / 'server.log').read_text()
                commands = json.dumps(results['cases'], ensure_ascii=False)
                assert all(secret not in text + commands for secret in secret_values)
                with sqlite3.connect(temporary / 'data/fileshare.db') as db:
                    assert db.execute('PRAGMA foreign_key_list(shares)').fetchall() == []
                active['observations']['foreign_key_count'] = 0
        finally:
            process.terminate()
            process.wait(timeout=15)
            log.close()
    results['finished_at'] = now()
    results['passed'] = sum(case['status'] == 'PASS' for case in results['cases'])
    results['failed'] = sum(case['status'] == 'FAIL' for case in results['cases'])
    output.write_text(json.dumps(results, ensure_ascii=False, indent=2) + '\n')
    print(f"Result: {results['passed']} passed, {results['failed']} failed. Evidence: {output.relative_to(ROOT)}")
    return int(results['failed'] > 0)


if __name__ == '__main__':
    raise SystemExit(main())
