#!/usr/bin/env python3
"""Interactively create .env; secrets are read from stdin, never process arguments."""
import getpass
import hashlib
from pathlib import Path
import secrets
import subprocess

root = Path(__file__).resolve().parents[1]
target = root / '.env'
if target.exists():
    raise SystemExit('.env already exists; edit it directly rather than overwriting credentials')
server = root / 'bin/server'
if not server.exists():
    raise SystemExit('Run go build -o bin/server ./cmd/server first')
origin = input('Public origin [http://localhost:8080]: ').strip() or 'http://localhost:8080'
user = input('Admin username [admin]: ').strip() or 'admin'
password = getpass.getpass('Admin password: ')
if len(password) < 12:
    raise SystemExit('Use at least 12 characters')
if password != getpass.getpass('Repeat password: '):
    raise SystemExit('Passwords do not match')
token = getpass.getpass('CLI token (blank to generate one): ') or secrets.token_urlsafe(32)
password_hash = subprocess.check_output([server, 'hash-password'], input=password.encode()).decode().strip()
values = {'FILESHARE_PORT': '8080', 'FILESHARE_PUBLIC_URL': origin, 'FILESHARE_ADMIN_USER': user,
          'FILESHARE_PASSWORD_HASH': password_hash, 'FILESHARE_TOKEN_HASH': hashlib.sha256(token.encode()).hexdigest(),
          'FILESHARE_SESSION_KEY': secrets.token_hex(32), 'FILESHARE_MAX_UPLOAD_BYTES': '104857600'}
# Single quotes preserve bcrypt dollar signs for Docker Compose and shell sourcing.
with target.open('x') as f:
    target.chmod(0o600)
    for key, value in values.items():
        if "'" in value or '\n' in value or '\r' in value:
            raise SystemExit('Invalid configuration input')
        f.write(f"{key}='{value}'\n")
print('Created .env (0600). Save this CLI token in your secret manager; it is shown once:')
print(token)
