"""Opt-in real Firefox/native-host/uninstall smoke test against a fresh companion install.
Runs a disposable headless Firefox profile. Does not control the user's normal browser.
"""
import http.server
import json
import os
import pathlib
import shutil
import signal
import subprocess
import tempfile
import threading
import time

repo = pathlib.Path(__file__).resolve().parents[1]
installed = pathlib.Path.home() / ".local/share/cortier-ddev-manager"
assert installed.exists(), "Install the companion first"
assert not json.loads((installed / "registrations.json").read_text())["profiles"], "This test needs an install with no registered profiles"
received = threading.Event()
report = {}
class Handler(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        report.update(json.loads(self.rfile.read(int(self.headers["Content-Length"]))))
        self.send_response(200)
        self.end_headers()
        received.set()
    def log_message(self, *args):
        pass
server = http.server.HTTPServer(("127.0.0.1", 0), Handler)
threading.Thread(target=server.serve_forever, daemon=True).start()
with tempfile.TemporaryDirectory(prefix="ddev-manager-firefox-") as directory:
    directory = pathlib.Path(directory)
    extension = directory / "extension"
    shutil.copytree(repo / "dist/extension", extension)
    endpoint = f"http://127.0.0.1:{server.server_port}"
    manifest = json.loads((extension / "manifest.json").read_text())
    manifest["permissions"].append("http://127.0.0.1/*")
    (extension / "manifest.json").write_text(json.dumps(manifest))
    with (extension / "background.js").open("a") as f:
        f.write('\nbrowser.tabs.create({url: browser.runtime.getURL("popup.html")});\n')
    html = (extension / "popup.html").read_text().replace("</body>", '<script src="smoke.js"></script></body>')
    (extension / "popup.html").write_text(html)
    (extension / "smoke.js").write_text('''
(async()=>{
  let state;
  for(let i=0;i<60;i++){
    state=await browser.runtime.sendMessage({type:'getState'});
    if(state.connected&&state.cleanupReady&&state.surfaces.length)break;
    await new Promise(r=>setTimeout(r,500));
  }
  await fetch(ENDPOINT,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({connected:state.connected,cleanupReady:state.cleanupReady,count:state.surfaces.length,error:state.error,cleanupError:state.cleanupError})});
  if(state.connected&&state.cleanupReady)await browser.management.uninstallSelf({showConfirmDialog:false});
})().catch(async e=>{await fetch(ENDPOINT,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({error:String(e)})})});
'''.replace("ENDPOINT", json.dumps(endpoint)))
    with (directory / "firefox.log").open("w+") as log:
        proc = subprocess.Popen([str(repo / "node_modules/.bin/web-ext"), "run", "--source-dir", str(extension), "--firefox", "/usr/bin/firefox", "--no-reload", "--no-input", "--verbose", "--arg=-headless"], cwd=repo, stdout=log, stderr=log, start_new_session=True)
        try:
            if not received.wait(50):
                log.seek(0)
                raise AssertionError("Firefox did not report readiness:\n" + log.read()[-5000:])
            print("Real Firefox native connection:", json.dumps(report), flush=True)
            assert report["connected"] and report["cleanupReady"] and report["count"] > 0
            for _ in range(200):
                if not installed.exists():
                    print("Firefox uninstall opened the cleanup page and removed the companion", flush=True)
                    break
                time.sleep(.1)
            else:
                log.seek(0)
                raise AssertionError("Firefox uninstall cleanup did not complete:\n" + log.read()[-5000:])
        finally:
            os.killpg(proc.pid, signal.SIGTERM)
            proc.wait(timeout=10)
server.shutdown()
