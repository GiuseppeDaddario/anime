"""Build the native-only release archive; never bundle Python, keys or a browser."""
import argparse
import gzip
import io
import os
from pathlib import Path
import re
import subprocess
import tarfile
from urllib.parse import urlparse


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--version', required=True)
    parser.add_argument('--api-url', required=True)
    parser.add_argument('--arch', choices=['arm64', 'amd64'], required=True)
    parser.add_argument('--output', type=Path, default=Path('dist'))
    args = parser.parse_args()
    if not re.fullmatch(r'v?\d+\.\d+\.\d+(?:-[a-zA-Z0-9.-]+)?', args.version):
        parser.error('Use a semantic release version, for example v0.1.0')
    parsed = urlparse(args.api_url)
    if parsed.scheme != 'https' or not parsed.netloc or parsed.path not in ('', '/') or parsed.query or parsed.fragment or parsed.username:
        parser.error('A deployed HTTPS API URL is required before releasing')
    if not re.fullmatch(r'https://[a-zA-Z0-9.:-]+/?', args.api_url):
        parser.error('Invalid API URL')
    args.output.mkdir(parents=True, exist_ok=True)
    binary = args.output.resolve() / ('anime-' + args.arch)
    root = Path(__file__).resolve().parents[1]
    env = dict(os.environ, CGO_ENABLED='0', GOOS='darwin', GOARCH=args.arch)
    subprocess.run(['go', 'build', '-trimpath', '-buildvcs=false', '-ldflags',
                    f'-s -w -X main.version={args.version} -X main.defaultAPIURL={args.api_url.rstrip("/")}',
                    '-o', str(binary), '.'], cwd=root, env=env, check=True)
    archive = args.output / f'anime_{args.version}_darwin_{args.arch}.tar.gz'
    # Stable metadata also makes checksums comparable across repeated builds.
    with archive.open('wb') as file:
        with gzip.GzipFile(fileobj=file, mode='wb', mtime=0, filename='') as compressed:
            with tarfile.open(fileobj=compressed, mode='w') as tar:
                data = binary.read_bytes()
                member = tarfile.TarInfo('anime')
                member.size, member.mode, member.mtime = len(data), 0o755, 0
                tar.addfile(member, io.BytesIO(data))
    binary.unlink()
    print(archive)

if __name__ == '__main__': main()
