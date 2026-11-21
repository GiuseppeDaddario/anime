"""Release gate: do not ship a client pointing at a missing/incompatible API."""
import json
import sys
from urllib.parse import urlparse
from urllib.request import HTTPRedirectHandler, build_opener

class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, request, file, code, message, headers, new_url):
        return None

url = sys.argv[1].rstrip('/')
parsed = urlparse(url)
if parsed.scheme != 'https' or not parsed.netloc or parsed.path or parsed.query or parsed.fragment or parsed.username:
    raise SystemExit('Set ANIME_API_URL to your deployed HTTPS service URL before releasing')
with build_opener(NoRedirect).open(url + '/v1/healthz', timeout=30) as response:
    result = json.load(response)
if result.get('status') != 'ok' or result.get('protocol') != 1:
    raise SystemExit('The deployed API does not support this client protocol')
print('Deployed API is reachable and supports protocol 1.')
