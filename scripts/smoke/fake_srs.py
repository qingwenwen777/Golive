# Minimal fake of the SRS HTTP API: records publisher kicks; the streams
# listing reports an error so the reconciler treats SRS as unreachable.
import json, http.server, sys
KICKS = []
class H(http.server.BaseHTTPRequestHandler):
    def _send(self, code, body):
        data = json.dumps(body).encode()
        self.send_response(code); self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data))); self.end_headers(); self.wfile.write(data)
    def do_DELETE(self):
        if self.path.startswith("/api/v1/clients/"):
            KICKS.append(self.path.rsplit("/", 1)[-1]); self._send(200, {"code": 0})
        else: self._send(404, {"code": 404})
    def do_GET(self):
        if self.path == "/_kicks": self._send(200, KICKS)
        else: self._send(500, {"code": 1})
    def log_message(self, *a): pass
http.server.ThreadingHTTPServer(("127.0.0.1", 1985), H).serve_forever()
