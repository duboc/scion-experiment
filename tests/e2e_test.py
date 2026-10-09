#!/usr/bin/env python3
"""End-to-end verification for the Incident Status Dashboard (DESIGN.md §9).

Builds the Go backend, starts it on a free port with a fresh in-memory store,
and exercises every §7 endpoint (happy paths and error paths), the
declare -> update -> resolve flow, derived service/overall status, security
headers, and the static UI (including a headless-Chromium render if available).

Usage:  python3 /workspace/tests/e2e_test.py -v
Stdlib only.
"""

import json
import os
import shutil
import socket
import subprocess
import tempfile
import time
import unittest
import urllib.error
import urllib.request

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
BACKEND = os.path.join(ROOT, "backend")
FRONTEND = os.path.join(ROOT, "frontend")
JSON_CT = "application/json"


def _free_port():
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


class Server:
    def __init__(self):
        self.tmp = tempfile.mkdtemp(prefix="e2e-incidentdash-")
        self.bin = os.path.join(self.tmp, "incidentdash")
        self.port = _free_port()
        self.base = f"http://127.0.0.1:{self.port}"
        self.proc = None

    def start(self):
        subprocess.run(["go", "build", "-o", self.bin, "."], cwd=BACKEND, check=True)
        self.log = open(os.path.join(self.tmp, "server.log"), "w")
        self.proc = subprocess.Popen(
            [self.bin, "-addr", f"127.0.0.1:{self.port}", "-static", FRONTEND],
            cwd=BACKEND, stdout=self.log, stderr=subprocess.STDOUT)
        deadline = time.time() + 15
        while time.time() < deadline:
            try:
                urllib.request.urlopen(self.base + "/healthz", timeout=1)
                return
            except Exception:
                time.sleep(0.1)
        raise RuntimeError("server did not become healthy")

    def stop(self):
        if self.proc:
            self.proc.terminate()
            try:
                self.proc.wait(timeout=10)
            except subprocess.TimeoutExpired:
                self.proc.kill()
        self.log.close()
        shutil.rmtree(self.tmp, ignore_errors=True)


SERVER = None


def setUpModule():
    global SERVER
    SERVER = Server()
    SERVER.start()


def tearDownModule():
    SERVER.stop()


def req(method, path, body=None, content_type=JSON_CT, raw=None):
    """Returns (status, headers, parsed-json-or-text)."""
    data = raw if raw is not None else (json.dumps(body).encode() if body is not None else None)
    r = urllib.request.Request(SERVER.base + path, data=data, method=method)
    if data is not None and content_type:
        r.add_header("Content-Type", content_type)
    try:
        resp = urllib.request.urlopen(r, timeout=5)
        status, headers, payload = resp.status, resp.headers, resp.read()
    except urllib.error.HTTPError as e:
        status, headers, payload = e.code, e.headers, e.read()
    text = payload.decode()
    if headers.get("Content-Type", "").startswith(JSON_CT) and text:
        return status, headers, json.loads(text)
    return status, headers, text


def declare(**overrides):
    body = {"title": "E2E outage", "service_id": "api", "severity": "sev3",
            "description": "created by e2e"}
    body.update(overrides)
    return req("POST", "/api/v1/incidents", body)


class T01Health(unittest.TestCase):
    def test_healthz(self):
        s, h, b = req("GET", "/healthz")
        self.assertEqual(s, 200)
        self.assertEqual(b, {"status": "ok"})
        self.assertEqual(h.get("X-Content-Type-Options"), "nosniff")


class T02Seed(unittest.TestCase):
    """Runs before any mutation (unittest orders classes alphabetically)."""

    def test_services_sorted_and_seeded(self):
        s, _, b = req("GET", "/api/v1/services")
        self.assertEqual(s, 200)
        ids = [x["id"] for x in b["services"]]
        self.assertEqual(ids, sorted(["api", "web", "db", "auth", "payments"]))
        status = {x["id"]: x["status"] for x in b["services"]}
        self.assertEqual(status["payments"], "partial_outage")  # open sev2 seed
        self.assertEqual(status["web"], "operational")           # resolved seed

    def test_summary_seed(self):
        s, _, b = req("GET", "/api/v1/summary")
        self.assertEqual(s, 200)
        self.assertEqual(b["overall_status"], "partial_outage")
        self.assertEqual(b["open_incidents"], 1)
        self.assertEqual(len(b["services"]), 5)

    def test_seed_incidents(self):
        _, _, b = req("GET", "/api/v1/incidents?status=resolved")
        self.assertTrue(any(i["service_id"] == "web" and i["severity"] == "sev3"
                            and i["resolved_at"] for i in b["incidents"]))
        _, _, b = req("GET", "/api/v1/incidents?status=open")
        self.assertTrue(any(i["service_id"] == "payments" and i["severity"] == "sev2"
                            for i in b["incidents"]))


class T03Lifecycle(unittest.TestCase):
    def test_declare_update_resolve_flow(self):
        s, h, inc = declare(title="  DB down  ", service_id="db", severity="sev1")
        self.assertEqual(s, 201)
        self.assertTrue(h.get("Location", "").endswith("/api/v1/incidents/" + inc["id"]))
        self.assertTrue(inc["id"].startswith("inc-"))
        self.assertEqual(inc["title"], "DB down")  # trimmed
        self.assertEqual(inc["status"], "investigating")
        self.assertIsNone(inc["resolved_at"])
        self.assertEqual(len(inc["updates"]), 1)
        self.assertEqual(inc["updates"][0]["message"], "Incident declared")
        iid = inc["id"]

        _, _, summ = req("GET", "/api/v1/summary")
        self.assertEqual(summ["overall_status"], "major_outage")
        self.assertEqual({x["id"]: x["status"] for x in summ["services"]}["db"], "major_outage")

        s, _, got = req("GET", f"/api/v1/incidents/{iid}")
        self.assertEqual((s, got["id"]), (200, iid))

        s, _, inc = req("POST", f"/api/v1/incidents/{iid}/updates",
                        {"status": "monitoring", "message": "fix deployed"})
        self.assertEqual(s, 200)
        self.assertEqual(inc["status"], "monitoring")
        self.assertEqual([u["status"] for u in inc["updates"]], ["investigating", "monitoring"])

        s, _, inc = req("POST", f"/api/v1/incidents/{iid}/updates",
                        {"status": "resolved", "message": "all clear"})
        self.assertEqual(s, 200)
        self.assertEqual(inc["status"], "resolved")
        self.assertIsNotNone(inc["resolved_at"])

        _, _, svcs = req("GET", "/api/v1/services")
        self.assertEqual({x["id"]: x["status"] for x in svcs["services"]}["db"], "operational")

        s, _, b = req("POST", f"/api/v1/incidents/{iid}/updates",
                      {"status": "investigating", "message": "reopen?"})
        self.assertEqual(s, 409)
        self.assertEqual(b["error"]["code"], "failed_precondition")

    def test_severity_mapping_worst_wins(self):
        for sev in ("sev4", "sev3"):
            self.assertEqual(declare(service_id="auth", severity=sev)[0], 201)
        st = lambda: {x["id"]: x["status"] for x in req("GET", "/api/v1/services")[2]["services"]}["auth"]
        self.assertEqual(st(), "degraded")
        self.assertEqual(declare(service_id="auth", severity="sev2")[0], 201)
        self.assertEqual(st(), "partial_outage")
        self.assertEqual(declare(service_id="auth", severity="sev1")[0], 201)
        self.assertEqual(st(), "major_outage")

    def test_list_order_and_filters(self):
        a = declare(title="first", service_id="web")[2]["id"]
        b = declare(title="second", service_id="web")[2]["id"]
        _, _, lst = req("GET", "/api/v1/incidents?service_id=web")
        ids = [i["id"] for i in lst["incidents"]]
        self.assertTrue(all(i["service_id"] == "web" for i in lst["incidents"]))
        self.assertLess(ids.index(b), ids.index(a))  # newest first
        _, _, op = req("GET", "/api/v1/incidents?status=open")
        self.assertTrue(all(i["status"] != "resolved" for i in op["incidents"]))
        _, _, rs = req("GET", "/api/v1/incidents?status=resolved")
        self.assertTrue(all(i["status"] == "resolved" for i in rs["incidents"]))


class T04Errors(unittest.TestCase):
    def assertErr(self, result, status, code):
        s, h, b = result
        self.assertEqual(s, status, b)
        self.assertTrue(h.get("Content-Type", "").startswith(JSON_CT))
        self.assertEqual(b["error"]["code"], code)
        self.assertTrue(b["error"]["message"])

    def test_validation(self):
        self.assertErr(declare(title="   "), 400, "invalid_argument")
        self.assertErr(declare(title="x" * 121), 400, "invalid_argument")
        self.assertErr(declare(description="x" * 2001), 400, "invalid_argument")
        self.assertErr(declare(service_id="nope"), 400, "invalid_argument")
        self.assertErr(declare(severity="sev9"), 400, "invalid_argument")
        self.assertErr(declare(extra="field"), 400, "invalid_argument")
        self.assertErr(req("POST", "/api/v1/incidents", raw=b"{not json"), 400, "invalid_argument")
        self.assertErr(req("POST", "/api/v1/incidents", raw=b"{}" + b" " * (65 * 1024) + b"{}"),
                       400, "invalid_argument")
        self.assertEqual(declare(title="x" * 120)[0], 201)  # boundary ok

    def test_csrf_content_type(self):
        body = json.dumps({"title": "csrf", "service_id": "api", "severity": "sev4"}).encode()
        for ct in ("text/plain", "application/x-www-form-urlencoded", None):
            self.assertErr(req("POST", "/api/v1/incidents", raw=body, content_type=ct),
                           400, "invalid_argument")
        s, _, _ = req("POST", "/api/v1/incidents", raw=body,
                      content_type="application/json; charset=utf-8")
        self.assertEqual(s, 201)

    def test_update_validation(self):
        iid = declare()[2]["id"]
        p = f"/api/v1/incidents/{iid}/updates"
        self.assertErr(req("POST", p, {"status": "monitoring", "message": ""}), 400, "invalid_argument")
        self.assertErr(req("POST", p, {"status": "monitoring", "message": "x" * 1001}), 400, "invalid_argument")
        self.assertErr(req("POST", p, {"status": "bogus", "message": "m"}), 400, "invalid_argument")
        self.assertErr(req("POST", "/api/v1/incidents/inc-99999/updates",
                           {"status": "monitoring", "message": "m"}), 404, "not_found")

    def test_not_found_and_filters(self):
        self.assertErr(req("GET", "/api/v1/incidents/inc-99999"), 404, "not_found")
        self.assertErr(req("GET", "/api/v1/nope"), 404, "not_found")
        self.assertErr(req("GET", "/api/v1/incidents?status=bogus"), 400, "invalid_argument")
        self.assertErr(req("GET", "/api/v1/incidents?service_id=bogus"), 400, "invalid_argument")

    def test_method_not_allowed(self):
        r = req("DELETE", "/api/v1/incidents")
        self.assertErr(r, 405, "method_not_allowed")
        self.assertTrue(r[1].get("Allow"))
        self.assertErr(req("PUT", "/api/v1/summary", {}), 405, "method_not_allowed")


class T05Frontend(unittest.TestCase):
    def test_static_assets(self):
        s, h, html = req("GET", "/")
        self.assertEqual(s, 200)
        self.assertIn("text/html", h.get("Content-Type", ""))
        self.assertEqual(h.get("X-Content-Type-Options"), "nosniff")
        for hook in ("overall-status", "declare-form", "services",
                     "open-incidents", "resolved-incidents"):
            self.assertIn(f'data-testid="{hook}"', html)
        self.assertIn("Content-Security-Policy", html)
        self.assertNotIn("innerHTML", html)
        for asset in ("/app.js", "/styles.css", "/api.js"):
            self.assertEqual(req("GET", asset)[0], 200, asset)

    def test_no_innerhtml_in_js(self):
        for name in os.listdir(FRONTEND):
            if name.endswith(".js"):
                with open(os.path.join(FRONTEND, name)) as f:
                    self.assertNotIn("innerHTML", f.read(), name)

    @unittest.skipUnless(shutil.which("chromium"), "chromium not installed")
    def test_headless_render(self):
        """Renders the page with JS and checks seed + service cards appear."""
        out = subprocess.run(
            ["chromium", "--headless", "--no-sandbox", "--disable-gpu",
             "--virtual-time-budget=5000", "--dump-dom", SERVER.base + "/"],
            capture_output=True, text=True, timeout=60).stdout
        for sid in ("api", "web", "db", "auth", "payments"):
            self.assertIn(f'data-testid="service-card-{sid}"', out)
        self.assertRegex(out, r'data-testid="incident-inc-\d+"')
        self.assertRegex(out, r'data-testid="update-form-inc-\d+"')


if __name__ == "__main__":
    unittest.main()
