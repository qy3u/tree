"""Re-fetch the pinned Fluent Emoji Flat subset, verifying asset hashes."""
import concurrent.futures
import hashlib
import json
from pathlib import Path
import urllib.parse
import urllib.request

folder = Path(__file__).resolve().parents[1] / 'assets' / 'page-icons'
source = json.loads((folder / 'SOURCE.json').read_text())
items = json.loads((folder / 'catalog.json').read_text())
base = 'https://raw.githubusercontent.com/microsoft/fluentui-emoji/' + source['revision'] + '/'

def download(item):
    data = urllib.request.urlopen(base + urllib.parse.quote(item['source']), timeout=30).read()
    if hashlib.sha256(data).hexdigest() != item['sha256']:
        raise ValueError('Hash mismatch: ' + item['id'])
    (folder / (item['id'] + '.svg')).write_bytes(data)

with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
    list(pool.map(download, items))
(folder / 'LICENSE').write_bytes(urllib.request.urlopen(base + 'LICENSE', timeout=30).read())
print(f"Downloaded {len(items)} verified icons at {source['revision']}")
