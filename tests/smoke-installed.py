"""Opt-in destructive-to-this-install smoke check; requires no registered profiles.
Run after installing a fresh companion. It removes that companion, never DDEV projects.
"""
import json
import os
import pathlib
import secrets
import struct
import subprocess
import sys
import time
import urllib.request
from urllib.parse import urlsplit

if sys.platform != "linux":
    raise SystemExit("This installed-service smoke check runs on Linux only")
root = pathlib.Path.home() / ".local/share/cortier-ddev-manager"
manifest = pathlib.Path.home() / ".mozilla/native-messaging-hosts/com.cortier.ddev_manager.json"
assert not json.loads((root / "registrations.json").read_text())["profiles"], "Use a fresh install with no registered Firefox profiles"

def native(method, **params):
    request = json.dumps(dict(version=1, id=secrets.token_hex(8), method=method, **params)).encode()
    output = subprocess.check_output([str(root / "ddev-manager"), str(manifest), "ddev-manager@cortier.com"], input=struct.pack("=I", len(request)) + request)
    size = struct.unpack("=I", output[:4])[0]
    result = json.loads(output[4:4+size])
    assert "error" not in result, result.get("error")
    return result["result"]

projects = native("discover")
assert len(projects) > 0
print("Native protocol discovery:", len(projects), "projects")
service_checks = json.loads(os.environ.get("DDEV_MANAGER_SMOKE_SERVICES", "{}"))
for surface, expected in service_checks.items():
    links = native("services", surfaceId=surface)
    assert all(name in [link["id"] for link in links] for name in expected), links
    print(surface + " services:", ", ".join(link["name"] for link in links))

profiles = [dict(profileId=secrets.token_hex(32), proof=secrets.token_hex(32)) for _ in range(2)]
urls = [native("register", **profile)["url"] for profile in profiles]
subprocess.check_call(["systemctl", "--user", "restart", "com.cortier.ddev-manager.service"])
for _ in range(50):
    try:
        assert native("register", **profiles[0])["url"] == urls[0]
        break
    except AssertionError:
        time.sleep(.1)
else:
    raise AssertionError("Registration did not survive restart")
for i, url in enumerate(urls):
    parts = urlsplit(url)
    origin = f"{parts.scheme}://{parts.netloc}"
    with urllib.request.urlopen(origin + parts.path) as response:
        assert response.status == 200
    assert len(json.loads((root / "registrations.json").read_text())["profiles"]) == 2-i, "GET changed registrations"
    profile_id, token = parts.fragment.split(":")
    request = urllib.request.Request(origin + parts.path, data=json.dumps(dict(id=profile_id, token=token)).encode(), headers={"Origin": origin, "Content-Type": "application/json"})
    with urllib.request.urlopen(request) as response:
        assert response.status == 200
    if i == 0:
        assert (root / "ddev-manager").exists()
        print("First profile removal retained companion")
for _ in range(100):
    if not root.exists() and not manifest.exists():
        print("Last profile removal cleaned binary, data, socket, and native registration")
        break
    time.sleep(.1)
else:
    raise AssertionError("Automatic cleanup did not complete")
assert not (pathlib.Path.home() / ".config/systemd/user/com.cortier.ddev-manager.service").exists()
print("Linux startup registration removed")
