#!/usr/bin/env python3
"""Run PRD acceptance with the real server/CLI; optionally use Docker and browser."""
import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import secrets
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parents[1]


def fixtures(path):
    # Minimal valid, one-page PDF, with an accurate xref table.
    objects = [b'<< /Type /Catalog /Pages 2 0 R >>',
               b'<< /Type /Pages /Kids [3 0 R] /Count 1 >>',
               b'<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 200] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>',
               b'<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>']
    stream = b'BT /F1 18 Tf 30 120 Td (Sharebox report) Tj ET'
    objects.append(b'<< /Length ' + str(len(stream)).encode() + b' >>\nstream\n' + stream + b'\nendstream')
    pdf = b'%PDF-1.4\n'; offsets = [0]
    for i, obj in enumerate(objects, 1):
        offsets.append(len(pdf)); pdf += f'{i} 0 obj\n'.encode() + obj + b'\nendobj\n'
    xref = len(pdf)
    pdf += f'xref\n0 {len(offsets)}\n0000000000 65535 f \n'.encode()
    for offset in offsets[1:]:
        pdf += f'{offset:010d} 00000 n \n'.encode()
    pdf += f'trailer\n<< /Size {len(offsets)} /Root 1 0 R >>\nstartxref\n{xref}\n%%EOF\n'.encode()
    (path / 'report.pdf').write_bytes(pdf)
    (path / 'image.png').write_bytes(base64.b64decode('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII='))
    (path / 'notes.txt').write_text('A small file to share.\n')


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--docker', action='store_true')
    parser.add_argument('--compose', action='store_true', help='Run the supplied Compose service with the built image')
    parser.add_argument('--image', default='interview-project:local')
    parser.add_argument('--browser', action='store_true')
    args = parser.parse_args()
    if args.compose: args.docker = True
    checks = []
    def passed(name):
        checks.append(name); print('PASS:', name, flush=True)
    with tempfile.TemporaryDirectory(prefix='fileshare-acceptance-') as temp:
        temp = Path(temp); fixtures(temp)
        with socket.socket() as sock:
            sock.bind(('127.0.0.1', 0)); port = sock.getsockname()[1]
        base = f'http://127.0.0.1:{port}'
        token, password = secrets.token_urlsafe(32), secrets.token_urlsafe(20)
        server = ROOT / 'bin/server'; cli = ROOT / 'bin/fileshare'
        password_hash = subprocess.check_output([server, 'hash-password'], input=password.encode()).decode().strip()
        cfg = {'FILESHARE_ADDR': ':8080' if args.docker else f'127.0.0.1:{port}', 'FILESHARE_PUBLIC_URL': base,
               'FILESHARE_DATA_DIR': '/data' if args.docker else str(temp / 'data'),
               'FILESHARE_WEB_DIR': '/app/web' if args.docker else str(ROOT / 'web/dist/client'),
               'FILESHARE_ADMIN_USER': 'admin', 'FILESHARE_PASSWORD_HASH': password_hash,
               'FILESHARE_TOKEN_HASH': hashlib.sha256(token.encode()).hexdigest(), 'FILESHARE_SESSION_KEY': secrets.token_hex(32)}
        cli_env = dict(os.environ, FILESHARE_CONFIG=str(temp / 'cli.json'), FILESHARE_SERVER=base, FILESHARE_TOKEN=token)
        proc = None; name = 'fileshare-acceptance-' + secrets.token_hex(5); volume = name + '-data'
        log = (temp / 'server.log').open('w+')
        compose = ['docker', 'compose', '-p', name, '--env-file', str(temp / 'compose.env'), '-f', str(ROOT / 'compose.yaml'), '-f', str(temp / 'override.yaml')]
        def start():
            nonlocal proc
            proc = subprocess.Popen([server], cwd=ROOT, env=dict(os.environ, **cfg), stdout=log, stderr=log)
        def wait_ready():
            for _ in range(120):
                try:
                    with urllib.request.urlopen(base + '/healthz', timeout=1) as res:
                        if res.status == 200: return
                except (OSError, urllib.error.URLError): pass
                time.sleep(.25)
            raise RuntimeError('server did not start')
        def command(*argv, success=True, env=None):
            result = subprocess.run([cli, *argv, '--json'], env=env or cli_env, text=True, capture_output=True)
            if not success:
                assert result.returncode != 0 and not result.stdout and result.stderr
                return result
            assert result.returncode == 0, result.stderr
            return json.loads(result.stdout)
        def request(path, expected=200, method='GET', headers=None):
            req = urllib.request.Request(base + path, method=method, headers=headers or {})
            try: res = urllib.request.urlopen(req, timeout=10)
            except urllib.error.HTTPError as error: res = error
            with res:
                data = res.read(); assert res.status == expected, (method, path, res.status, data)
                return data, res.headers
        try:
            if args.compose:
                envfile = temp / 'compose.env'
                envfile.write_text(''.join(f"{k}='{v}'\n" for k, v in dict(cfg, FILESHARE_PORT=str(port)).items())); envfile.chmod(0o600)
                (temp / 'override.yaml').write_text('services:\n  fileshare:\n    image: ' + args.image + '\n')
                subprocess.run(compose + ['up', '--no-build', '-d'], check=True, stdout=subprocess.DEVNULL)
            elif args.docker:
                envfile = temp / 'server.env'; envfile.write_text(''.join(f'{k}={v}\n' for k, v in cfg.items())); envfile.chmod(0o600)
                subprocess.run(['docker', 'run', '-d', '--name', name, '--env-file', str(envfile), '-p', f'127.0.0.1:{port}:8080', '-v', volume + ':/data', args.image], check=True, stdout=subprocess.DEVNULL)
            else: start()
            wait_ready()
            request('/'); request('/login')
            passed('Production SPA shell served by Go')
            config_env = dict(cli_env); config_env.pop('FILESHARE_SERVER'); config_env.pop('FILESHARE_TOKEN')
            command('config', 'set', '--server', base, '--token', token, env=config_env)
            assert (temp / 'cli.json').stat().st_mode & 0o777 == 0o600
            f = command('upload', str(temp / 'report.pdf'), env=config_env)
            assert f['share_status'] == 'unshared' and f['share_url'] is None
            fid = f['file_id']; request('/files/' + fid, 404)
            assert command('get', fid)['file_id'] == fid
            listing = command('list', '--search', 'report'); assert any(i['file_id'] == fid for i in listing['items'])
            passed('PRD 1: upload is private and visible in file list')
            f = command('share', fid); old = f['share_url'].split('/s/')[1]
            assert command('share', fid)['share_url'] == f['share_url']
            data, headers = request('/api/public/shares/' + old + '/content')
            assert data == (temp / 'report.pdf').read_bytes() and headers['Cache-Control'] == 'no-store'
            assert headers['Content-Disposition'].startswith('inline')
            request('/api/public/shares/' + old + '/content', method='HEAD')
            data, _ = request('/api/public/shares/' + old + '/content', 206, headers={'Range': 'bytes=0-4'}); assert data == b'%PDF-'
            _, headers = request('/api/public/shares/' + old + '/content?download=1'); assert headers['Content-Disposition'].startswith('attachment')
            passed('PRD 2: anonymous PDF preview, Range and download')
            command('revoke', fid); command('revoke', fid)
            for method in ['GET', 'HEAD']:
                request('/api/public/shares/' + old + '/content', 404, method, {'Range': 'bytes=0-4'})
            request('/api/public/shares/' + old, 404)
            passed('PRD 3: revoked link denies metadata, GET, HEAD and Range')
            f = command('share', fid); fresh = f['share_url'].split('/s/')[1]; assert fresh != old
            request('/api/public/shares/' + fresh + '/content'); request('/api/public/shares/' + old + '/content', 404)
            passed('PRD 4: reshare creates new link; old link stays invalid')
            passed('PRD 5: real Go CLI config/upload/list/get/share/revoke commands')
            agent_file = command('upload', str(temp / 'report.pdf'), '--share'); assert agent_file['share_status'] == 'shared'
            request('/api/public/shares/' + agent_file['share_url'].split('/s/')[1] + '/content')
            assert command('list', '--search', 'report.pdf')['total'] >= 2
            command('revoke', agent_file['file_id']); request('/api/public/shares/' + agent_file['share_url'].split('/s/')[1] + '/content', 404)
            passed('PRD 6: Skill upload-and-share, query, revoke workflows using real CLI')
            if args.compose: subprocess.run(compose + ['restart', 'fileshare'], check=True, stdout=subprocess.DEVNULL)
            elif args.docker: subprocess.run(['docker', 'restart', name], check=True, stdout=subprocess.DEVNULL)
            else: proc.terminate(); proc.wait(timeout=20); start()
            wait_ready()
            assert command('get', fid)['share_url'] == f['share_url']
            data, _ = request('/api/public/shares/' + fresh + '/content'); assert data == (temp / 'report.pdf').read_bytes()
            request('/api/public/shares/' + old + '/content', 404)
            assert command('get', agent_file['file_id'])['share_status'] == 'unshared'
            passed('PRD 7: restart retains bytes, valid links and revoked state' + (' (Docker Volume)' if args.docker else ' (local process)'))
            for method, path in [('GET', '/api/session'), ('POST', '/api/logout'), ('POST', '/api/files'), ('GET', '/api/files'), ('GET', '/api/files/' + fid), ('POST', '/api/files/' + fid + '/share'), ('DELETE', '/api/files/' + fid + '/share'), ('DELETE', '/api/files/' + fid)]:
                request(path, 401, method); request(path, 401, method, {'Authorization': 'Bearer invalid'})
            command('get', fid, success=False, env=dict(cli_env, FILESHARE_TOKEN='invalid'))
            command('upload', str(temp / 'does-not-exist.pdf'), success=False)
            passed('PRD 8: every management route rejects unauthenticated/invalid-token requests')
            command('delete', fid); command('delete', fid)
            request('/api/public/shares/' + fresh + '/content', 404)
            command('get', fid, success=False); command('share', fid, success=False)
            assert all(i['file_id'] != fid for i in command('list')['items'])
            passed('PRD 9: deletion removes file from management and invalidates public access')
            if args.browser:
                browser_env = dict(os.environ, E2E_BASE_URL=base, E2E_USERNAME='admin', E2E_PASSWORD=password, E2E_FIXTURES=str(temp))
                subprocess.run(['npm', 'run', 'test:e2e'], cwd=ROOT / 'web', env=browser_env, check=True)
                passed('Browser: upload, anonymous PDF/image/attachment, copy, revoke, reshare, delete, search and mobile layout')
        finally:
            if args.compose:
                subprocess.run(compose + ['down', '-v'], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            elif args.docker:
                subprocess.run(['docker', 'rm', '-f', name], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
                subprocess.run(['docker', 'volume', 'rm', volume], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            elif proc:
                proc.terminate(); proc.wait(timeout=20)
            log.close()
    evidence = ROOT / 'work' / ('acceptance-docker.json' if args.docker else 'acceptance-local.json')
    evidence.parent.mkdir(exist_ok=True); evidence.write_text(json.dumps({'checks': checks, 'browser': args.browser, 'compose': args.compose}, ensure_ascii=False, indent=2) + '\n')
    print(f'All {len(checks)} checks passed.')

if __name__ == '__main__':
    main()
