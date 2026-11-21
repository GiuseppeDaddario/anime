"""Generate a real formula from built assets and their actual SHA-256 checksums."""
import argparse
import hashlib
import json
from pathlib import Path
import re
from urllib.parse import urlparse


def generate(repository, version, directory, base=None):
    if not re.fullmatch(r'[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+', repository):
        raise ValueError('Expected OWNER/REPOSITORY')
    if not re.fullmatch(r'v?\d+\.\d+\.\d+(?:-[a-zA-Z0-9.-]+)?', version):
        raise ValueError('Invalid release version')
    base = base or f'https://github.com/{repository}/releases/download/{version}'
    if urlparse(base).scheme not in ('https', 'file') or any(character in base for character in '\n\r"#'):
        raise ValueError('Invalid asset base URL')
    assets = {}
    for arch in ('arm64', 'amd64'):
        name = f'anime_{version}_darwin_{arch}.tar.gz'
        assets[arch] = (f'{base}/{name}', hashlib.sha256((directory / name).read_bytes()).hexdigest())
    quote = json.dumps
    return f'''class Anime < Formula
  desc "Terminal anime catalogue, playback and Telegram history"
  homepage "https://github.com/{repository}"
  license "MIT"
  version {quote(version.removeprefix('v'))}
  depends_on :macos
  depends_on "mpv"

  on_arm do
    url {quote(assets['arm64'][0])}
    sha256 "{assets['arm64'][1]}"
  end
  on_intel do
    url {quote(assets['amd64'][0])}
    sha256 "{assets['amd64'][1]}"
  end

  def install
    bin.install "anime"
  end

  test do
    assert_match "anime", shell_output("#{{bin}}/anime --version")
    assert_match "--continue", shell_output("#{{bin}}/anime --help")
    assert_predicate Formula["mpv"].opt_bin/"mpv", :executable?
    system "#{{bin}}/anime", "--player", "--no-config", "--vo=null", "--ao=null",
           "--frames=1", "av://lavfi:testsrc=size=64x64:rate=1"
  end
end
'''


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--repository', required=True)
    parser.add_argument('--version', required=True)
    parser.add_argument('--assets', type=Path, default=Path('dist'))
    parser.add_argument('--asset-base')
    parser.add_argument('--output', type=Path, default=Path('dist/anime.rb'))
    args = parser.parse_args()
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(generate(args.repository, args.version, args.assets, args.asset_base))
    print(args.output)

if __name__ == '__main__': main()
