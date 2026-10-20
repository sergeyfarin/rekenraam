#!/usr/bin/env python3
"""#357: real TLS/client/native-restore probes; emit only redacted evidence.

Run after the #268 fixture's phases, with its private jar/audit.env/ca.crt.
The expected grants below come from ADR 0018's contract, not the runtime map.
Only standard-library Python plus the repository's existing Docker/curl tools.
"""
import argparse
import csv
import hashlib
import http.client
import json
import os
from pathlib import Path
import re
import socket
import sqlite3
import ssl
import subprocess
import time
from http.cookies import SimpleCookie
from io import BytesIO
from zipfile import ZipFile

LEDGER = {
    "/books/current", "/currencies", "/currencies/catalog", "/institutions",
    "/institutions/{institution_id}", "/institutions/{institution_id}/versions",
    "/accounts", "/accounts/{account_id}", "/accounts/{account_id}/versions",
    "/accounts/{account_id}/register", "/categories", "/categories/{category_id}",
    "/tags", "/tags/{tag_id}", "/payees", "/payees/{payee_id}",
    "/transactions", "/transactions/deleted", "/transactions/{transaction_id}",
}
REPORTS = {
    "/ledger/account-balances", "/ledger/category-totals", "/ledger/net-worth",
    "/reports/net-worth", "/reports/spending", "/reports/cashflow",
}
GRANTS = {f"GET /api/v1{path}": "ledger:read" for path in LEDGER}
GRANTS.update({f"GET /api/v1{path}": "reports:read" for path in REPORTS})
REPORT_RANGE = "?start_date=2026-01-01&end_date=2026-12-31&bucket=month"
PAT = re.compile(rb"rr_pat_[A-Za-z0-9_-]{43}")
OWNER_PASSWORD = "proxy-audit-password"
RECOVERED_PASSWORD = "pat-audit-recovered-password"


def check(condition, label):
    if not condition:
        # Never print credential-bearing request/response bodies or headers.
        raise RuntimeError(f"audit assertion failed: {label}")


class LoopbackTLS(http.client.HTTPSConnection):
    def connect(self):
        raw = socket.create_connection(("127.0.0.1", self.port), self.timeout)
        self.sock = self._context.wrap_socket(raw, server_hostname=self.host)


class Client:
    def __init__(self, ca, cookies=None, direct=False):
        self.context = ssl.create_default_context(cafile=str(ca))
        self.cookies = cookies or {}
        self.direct = direct

    def request(self, method, path, data=None, headers=(), token=None, owner=False):
        connection = (http.client.HTTPConnection("127.0.0.1", 16888, timeout=30)
                      if self.direct else LoopbackTLS("rekenraam.test", 8443, timeout=30, context=self.context))
        body = None if data is None else json.dumps(data).encode()
        connection.putrequest(method, path, skip_host=True, skip_accept_encoding=True)
        connection.putheader("Host", "127.0.0.1:16888" if self.direct else "rekenraam.test:8443")
        if body is not None:
            connection.putheader("Content-Type", "application/json")
            connection.putheader("Content-Length", str(len(body)))
        if token is not None:
            connection.putheader("Authorization", f"Bearer {token}")
        if owner:
            connection.putheader("Cookie", "; ".join(f"{name}={value}" for name, value in self.cookies.items()))
        for name, value in headers:
            connection.putheader(name, value)
        connection.endheaders(body)
        response = connection.getresponse()
        status, fields, raw = response.status, response.getheaders(), response.read()
        connection.close()
        for name, value in fields:
            if name.lower() == "set-cookie":
                for key, morsel in SimpleCookie(value).items():
                    self.cookies[key] = morsel.value
        return {"status": status, "headers": {name.lower(): value for name, value in fields}, "raw": raw}

    def json(self, method, path, data=None, expected=200, owner=True, headers=()):
        if method not in ("GET", "HEAD", "OPTIONS"):
            session = self.request("GET", "/api/v1/auth/session", owner=True)
            check(session["status"] == 200, "session for owner mutation")
            csrf = json.loads(session["raw"]).get("csrf_token")
            check(bool(csrf), "owner mutation has a session-bound CSRF token")
            headers = (*headers, ("Origin", "https://rekenraam.test:8443"), ("X-CSRF-Token", csrf))
        response = self.request(method, path, data, headers, owner=owner)
        check(response["status"] == expected, f"owner {method} {path}: status {response['status']}")
        return None if not response["raw"] else json.loads(response["raw"])


class Audit:
    def __init__(self, repo, work, output):
        self.repo, self.work, self.output = repo, work, output
        self.rows, self.commands, self.secrets = [], [], []
        self.owner = Client(work / "ca.crt", self.jar())
        self.client = Client(work / "ca.crt")
        self.direct = Client(work / "ca.crt", direct=True)

    def jar(self):
        values = {}
        for line in (self.work / "jar").read_text().splitlines():
            if line.startswith("#HttpOnly_"):
                line = line[len("#HttpOnly_"):]
            elif line.startswith("#"):
                continue
            fields = line.split("\t")
            if len(fields) == 7:
                values[fields[5]] = fields[6]
        return values

    def compose(self, *args, stdin=None, allowed=(0,)):
        command = ["docker", "compose", "-p", "rr-proxy-audit", "--env-file", str(self.work / "audit.env")]
        for file in ("compose.yaml", "proxy-audit/compose.yaml", "pat-audit/compose.yaml"):
            command += ["-f", str(self.repo / "deploy/docker" / file)]
        result = subprocess.run([*command, *args], input=stdin, capture_output=True)
        self.commands.append({"command": list(args), "exit_code": result.returncode})
        check(result.returncode in allowed, "compose command " + " ".join(args[:3]))
        return result.stdout + result.stderr

    def record(self, label, response, expected, **extra):
        accepted = (expected,) if isinstance(expected, int) else tuple(expected)
        self.rows.append({"case": label, "status": response["status"], "expected_statuses": list(accepted), **extra})
        check(response["status"] in accepted, f"{label}: status {response['status']}")
        return response

    def healthy(self):
        for _ in range(150):
            try:
                if self.client.request("GET", "/healthz")["status"] == 204:
                    return
            except (OSError, http.client.HTTPException):
                pass
            time.sleep(0.2)
        raise RuntimeError("disposable deployment did not become healthy")

    def create(self, name, scopes):
        result = self.owner.json("POST", "/api/v1/auth/tokens", {
            "name": name, "password": OWNER_PASSWORD, "scopes": scopes,
        }, expected=201)
        self.secrets.append(result["token"])
        return result["credential"]["id"], result["token"]

    def seed(self):
        # Allow a failed probe to be re-run against its disposable book without
        # mistaking earlier synthetic credentials for recovery survivors.
        for old in self.owner.json("GET", "/api/v1/auth/tokens")["tokens"]:
            if old["revoked_at"] is None:
                self.owner.json("DELETE", f"/api/v1/auth/tokens/{old['id']}", expected=204)
        suffix = str(time.time_ns())
        currencies = self.owner.json("GET", "/api/v1/currencies")["currencies"]
        commodity = currencies[0]["id"]
        account = self.owner.json("POST", "/api/v1/accounts", {
            "name": f"PAT audit checking {suffix}", "account_class": "asset", "account_kind": "checking",
            "default_commodity_id": commodity, "allows_postings": True, "opened_on": "2026-01-01",
        }, expected=201)
        category = self.owner.json("POST", "/api/v1/categories", {
            "name": f"PAT audit expense {suffix}", "category_type": "expense",
        }, expected=201)
        self.transaction = self.owner.json("POST", "/api/v1/transactions", {
            "transaction_date": "2026-01-02", "status": "posted", "description": "Synthetic PAT audit 25.00",
            "journal_entries": [{"entry_date": "2026-01-02", "postings": [
                {"account_id": account["id"], "commodity_id": commodity, "quantity_value": "-2500", "quantity_scale": 2},
                {"account_id": category["id"], "commodity_id": commodity, "quantity_value": "2500", "quantity_scale": 2},
            ]}],
        }, expected=201)["id"]
        self.expected_postings = sorted([(account["id"], "-2500", 2), (category["id"], "2500", 2)])
        self.financial("seed")

    def financial(self, phase):
        result = self.owner.json("GET", f"/api/v1/transactions/{self.transaction}")
        actual = sorted((p["account_id"], p["quantity_value"], p["quantity_scale"])
                        for entry in result["journal_entries"] for p in entry["postings"])
        check(result["status"] == "posted" and actual == self.expected_postings, f"exact -25.00/+25.00 at {phase}")
        self.rows.append({"case": f"financial-{phase}", "expected_coefficients": ["-2500", "2500"], "scale": 2, "pass": True})

    def routes(self, tokens):
        source = (self.repo / "backend/internal/api/health.go").read_text()
        registered = re.findall(r'mux\.HandleFunc\("([^"\n]+)"', source)
        check(len(registered) == source.count("mux.HandleFunc("), "every route registration is statically enumerated")
        check(set(GRANTS).issubset(registered), "all independently expected grants are registered")
        self.registry_count = len(registered)
        session = self.owner.json("GET", "/api/v1/auth/session")
        check(bool(session.get("csrf_token")), "fallback control has valid owner CSRF")
        browser_headers = [("Origin", "https://rekenraam.test:8443"), ("X-CSRF-Token", session["csrf_token"])]
        for index, pattern in enumerate(registered):
            if index % 100 == 0:
                print(f"route patterns: {index}/{len(registered)}", flush=True)
            method, path = pattern.split(" ", 1)
            concrete = re.sub(r"\{[^}]+\}", "1", path)
            if path in ("/api/v1/reports/net-worth", "/api/v1/reports/cashflow"):
                concrete += REPORT_RANGE
            elif path == "/api/v1/reports/spending":
                concrete += "?start_date=2026-01-01&end_date=2026-12-31&group_by=category"
            methods = [method, "HEAD"] if method == "GET" else [method]
            for verb in methods:
                scope = GRANTS.get(pattern)
                for grant, token in tokens.items():
                    admitted = bool(scope) and (grant == scope or grant == "both")
                    response = self.client.request(verb, concrete, token=token)
                    self.record(f"route:{grant}:{verb} {path}", response,
                                (200, 404) if admitted else 403,
                                pattern=pattern, grant=grant, admitted=admitted)
                    if not admitted:
                        self.scope_denial(response, verb)
                    if verb == "HEAD":
                        check(response["raw"] == b"", "real HEAD response has no body")
                    if not admitted:
                        # Even the owner cookie cannot broaden a Bearer grant.
                        mixed = self.owner.request(verb, concrete, data={} if verb not in ("GET", "HEAD") else None,
                                                   headers=browser_headers, token=token, owner=True)
                        self.record(f"cookie-fallback:{grant}:{verb} {path}", mixed, 403)
                        self.scope_denial(mixed, verb)
        self.financial("all-route-refusals")

    def scope_denial(self, response, method):
        if method != "HEAD":
            error = json.loads(response["raw"])["error"]
            check(error["code"] == "FORBIDDEN" and error["message"] == "API credential does not grant this operation",
                  "scope refusal is the Bearer boundary, not CSRF or a business guard")

    def paths_and_headers(self, token):
        variants = [
            ("/api/v1/%61ccounts", (200,)),
            ("/api/v1/accounts/%31", (200,)),
            ("/api/v1/accounts%2f1", (403,)),
            ("/api/v1/accounts/../auth/tokens", (403,)),
            ("/api/v1/accounts//../auth/tokens", (403,)),
            ("/api/v1/%61uth/tokens", (403,)),
            ("/api/v1/auth%2ftokens", (403,)),
            ("/api/v1/accounts/%2e%2e/auth/tokens", (403,)),
            ("/api/v1/accounts/1%2f..%2f..%2fauth%2ftokens", (400,)),
            ("/api/v1/accounts%2f../auth/tokens", (403,)),
            ("/api/v1/accounts/../accounts", (307,)),
            ("/api/v1//accounts", (307,)),
            ("/api/v1/accounts/", (403,)),
            ("/api/v1/accounts;ignored=/api/v1/auth/tokens", (403,)),
            ("/api/v1/ACCOUNTS", (403,)),
            ("/api/v1/accounts%00", (400, 403)),
            ("/api/v1/not-added-yet", (403,)),
            ("/app/settings/security", (403,)),
        ]
        for path, statuses in variants:
            for method in ("GET", "HEAD"):
                response = self.client.request(method, path, token=token)
                self.record(f"path:{method}:{path}", response, statuses)
                if response["status"] == 307:
                    check(response["headers"].get("location") == "/api/v1/accounts", "canonical redirect stays on the exact granted read")
        headers = [
            [("Authorization", "")], [("Authorization", "Bearer")], [("Authorization", "Basic invalid")],
            [("Authorization", "Bearer invalid")], [("Authorization", f"Bearer {token} trailing")],
            [("Authorization", f"Bearer {token}, invalid")],
            [("Authorization", f"Bearer {token}"), ("authorization", f"Bearer {token}")],
            [("authorization", "Bearer invalid"), ("Authorization", f"Bearer {token}")],
        ]
        for index, values in enumerate(headers):
            for label, client in (("proxy", self.owner), ("direct", self.direct)):
                client.cookies = self.owner.cookies.copy()
                response = client.request("GET", "/api/v1/accounts", headers=values, owner=True)
                self.record(f"header:{label}:{index}", response, 401)
        self.record("case-insensitive-bearer-with-tab", self.client.request("GET", "/api/v1/accounts", headers=[("Authorization", f"bEaReR\t{token}")]), 200)
        response = self.client.request("GET", "/api/v1/accounts", headers=[("Cookie", f"__Host-rekenraam_session={token}")])
        self.record("token-is-not-a-session-cookie", response, 401)
        response = self.client.request("GET", "/api/v1/accounts", data={"token": token})
        self.record("json-token-does-not-authenticate", response, 401)
        # Do not put a usable credential in a query string even in a probe:
        # proxy URI logging is outside the supported credential transport.
        response = self.client.request("GET", "/api/v1/accounts?access_token=obviously-invalid-audit-value")
        self.record("url-token-does-not-authenticate", response, 401)
        for method in ("OPTIONS", "TRACE", "PATCH", "POST", "PUT", "DELETE"):
            self.record(f"method:{method}:accounts", self.client.request(method, "/api/v1/accounts", token=token), 403)

    def attribution(self, token):
        forged = [("X-Forwarded-For", "203.0.113.66"), ("X-Forwarded-Host", "evil.example"),
                  ("X-Forwarded-Proto", "http"), ("Forwarded", "for=203.0.113.67;proto=http;host=evil.example"),
                  ("X-Real-IP", "203.0.113.68")]
        for label, client in (("trusted-proxy", self.client), ("untrusted-direct-peer", self.direct)):
            response = self.record(label, client.request("GET", "/api/v1/accounts", headers=forged, token=token), 200)
            request_id = response["headers"]["x-request-id"]
            events = self.owner.json("GET", "/api/v1/auth/events?limit=200")["events"]
            matching = [event for event in events if event.get("request_id") == request_id]
            check(len(matching) == 1, "one independently correlated authentication event")
            event = matching[0]
            check(event["event_type"] == "token_authenticated" and event["origin"] == "personal_access_token", label)
            check(not event.get("client_ip", "").startswith("203.0.113."), "forged addresses cannot control attribution")
            self.rows.append({"case": f"attribution:{label}", "client_ip": event["client_ip"], "request_id": request_id, "pass": True})
            if label == "trusted-proxy":
                check("strict-transport-security" in response["headers"], "proxy HTTPS emits HSTS")
            else:
                check("strict-transport-security" not in response["headers"], "untrusted forged HTTPS cannot produce HSTS")

    def redirects(self, token):
        for path, trusted, expected in (("same-origin", False, 200), ("cross-origin", False, 200), ("cross-origin", True, 418)):
            # Pass the synthetic secret through stdin, never argv, disk or output.
            config = f'''silent\nshow-error\nlocation\ncacert = "{self.work / 'ca.crt'}"\nresolve = "rekenraam.test:8443:127.0.0.1"\nresolve = "collector.rekenraam.test:8443:127.0.0.1"\nheader = "Authorization: Bearer {token}"\nurl = "https://rekenraam.test:8443/pat-audit/{path}"\noutput = "/dev/null"\nwrite-out = "%{{http_code}}"\n'''
            if trusted:
                config += "location-trusted\n"
            result = subprocess.run(["curl", "--config", "-"], input=config.encode(), capture_output=True)
            check(result.returncode == 0 and result.stdout.decode() == str(expected), f"curl redirect {path}, trusted={trusted}")
            self.rows.append({"case": f"redirect:{path}:unsafe-forwarding={trusted}", "status": expected, "pass": True})

    def backup(self, phase):
        print(f"backup: {phase}", flush=True)
        run = self.owner.json("POST", "/api/v1/maintenance/backups", expected=202)
        for _ in range(300):
            status = self.owner.json("GET", "/api/v1/maintenance/backups")
            matching = [row for row in status["runs"] if row["id"] == run["id"]]
            if matching and matching[0]["status"] in ("completed", "failed"):
                saved = matching[0]
                check(saved["status"] == "completed" and saved["verified"], f"native backup {phase}")
                target = saved["target_path"]
                self.compose("exec", "-T", "rekenraam", "/app/rekenraam", "verify-backup", "--from", target)
                destination = self.work / f"{phase}.sqlite"
                self.compose("cp", f"rekenraam:{target}", str(destination))
                self.inspect_database(destination, phase)
                print(f"backup verified: {phase}", flush=True)
                return target
            time.sleep(0.2)
        raise RuntimeError("native backup did not finish")

    def inspect_database(self, file, phase):
        raw = file.read_bytes()
        check(PAT.search(raw) is None, f"no plaintext PAT pattern in {phase}")
        connection = sqlite3.connect(file.as_uri() + "?mode=ro", uri=True)
        rows = connection.execute("SELECT token_hash, revoked_at, expires_at FROM personal_access_tokens ORDER BY id").fetchall()
        connection.close()
        hashes = {row[0] for row in rows}
        for secret in self.secrets:
            digest = hashlib.sha256(("pat:" + secret).encode()).hexdigest()
            check(secret.encode() not in raw and digest in hashes, f"hash-only credential in {phase}")
        self.rows.append({"case": f"storage:{phase}", "token_rows": len(rows), "plaintext_absent": True, "hashes_verified": len(self.secrets)})

    def restore(self, target, phase):
        print(f"restore: {phase}", flush=True)
        self.compose("stop", "rekenraam")
        self.compose("run", "--rm", "--no-deps", "-T", "rekenraam", "restore", "--from", target)
        self.compose("up", "-d", "--no-deps", "rekenraam")
        self.healthy()
        self.financial(phase)
        print(f"restore verified: {phase}", flush=True)

    def exports(self, phase):
        count = 0
        for endpoint in ("ledger.csv", "bundle.zip", "qif"):
            response = self.owner.request("GET", f"/api/v1/exports/{endpoint}", owner=True)
            check(response["status"] == 200, f"owner {endpoint} export")
            chunks = [response["raw"]]
            if response["raw"].startswith(b"PK"):
                with ZipFile(BytesIO(response["raw"])) as archive:
                    check(not any("personal-access-token" in name or "personal_access_token" in name for name in archive.namelist()), "credential store is excluded from ledger exports")
                    chunks += [archive.read(name) for name in archive.namelist()]
            count += len(chunks)
            for chunk in chunks:
                check(PAT.search(chunk) is None, f"no plaintext pattern in {endpoint}")
                for secret in self.secrets:
                    check(hashlib.sha256(("pat:" + secret).encode()).hexdigest().encode() not in chunk, "no credential digest in portable export")
        self.rows.append({"case": f"exports:{phase}", "formats": 3, "inspected_members": count, "credentials_absent": True})

    def logs(self):
        raw = self.compose("logs", "--no-color", "rekenraam", "proxy")
        check(PAT.search(raw) is None, "app and access logs have no plaintext PAT")
        for secret in self.secrets:
            check(hashlib.sha256(("pat:" + secret).encode()).hexdigest().encode() not in raw, "logs have no token digest")
        check(b"Authorization" in raw and b"REDACTED" in raw, "access log really observes and redacts credential headers")
        # Negative control: the detector must catch an actual known credential
        # inserted into a copy in memory. Never persist the unredacted copy.
        leaky = raw + json.dumps({"Authorization": self.secrets[0]}).encode()
        check(PAT.search(leaky) is not None, "leaky-log negative control")
        self.rows.append({"case": "logs", "plaintext_absent": True, "digest_absent": True, "redaction_observed": True, "leaky_control_detected": True})

    def run(self):
        self.healthy()
        self.seed()
        ledger_id, ledger = self.create("Independent ledger client", ["ledger:read"])
        report_id, report = self.create("Independent reports client", ["reports:read"])
        both_id, both = self.create("Independent combined client", ["ledger:read", "reports:read"])
        self.routes({"ledger:read": ledger, "reports:read": report, "both": both})
        print("route matrix passed; paths, headers, attribution and redirects", flush=True)
        self.paths_and_headers(ledger)
        self.attribution(ledger)
        self.redirects(ledger)
        self.exports("before-restore")
        print("native backups and restores", flush=True)
        before = self.backup("before-revocation")
        self.owner.json("DELETE", f"/api/v1/auth/tokens/{ledger_id}", expected=204)
        self.record("revoked-before-restore", self.client.request("GET", "/api/v1/accounts", token=ledger), 401)
        after = self.backup("after-revocation")
        self.restore(before, "restored-before-revocation")
        self.record("old-backup-restores-then-valid-token", self.client.request("GET", "/api/v1/accounts", token=ledger), 200)
        self.restore(after, "restored-after-revocation")
        self.record("later-backup-preserves-revocation", self.client.request("GET", "/api/v1/accounts", token=ledger), 401)
        self.record("unrevoked-token-survives-native-restore-without-key", self.client.request("GET", "/api/v1/reports/cashflow" + REPORT_RANGE, token=report), 200)
        self.exports("after-restore")
        print("CLI owner recovery", flush=True)
        self.compose("stop", "rekenraam")
        self.compose("run", "--rm", "--no-deps", "-T", "rekenraam", "recover-owner", "--password-stdin", "--backup-path", "/app/data/pat-audit-recovery.sqlite", stdin=(RECOVERED_PASSWORD + "\n").encode())
        self.compose("up", "-d", "--no-deps", "rekenraam")
        self.healthy()
        for name, token in (("ledger", ledger), ("report", report), ("both", both)):
            self.record(f"recovery-revokes:{name}", self.client.request("GET", "/api/v1/accounts", token=token), 401)
        self.record("recovery-revokes-carried-session", self.owner.request("GET", "/api/v1/auth/tokens", owner=True), 401)
        response = self.owner.request("POST", "/api/v1/auth/login", {"username": "owner", "password": RECOVERED_PASSWORD}, [("Origin", "https://rekenraam.test:8443")])
        self.record("recovered-owner-signs-in", response, 200)
        check(not json.loads(response["raw"]).get("mfa_required"), "recovery clears unreadable MFA")
        self.financial("owner-recovered")
        events = self.owner.json("GET", "/api/v1/auth/events?limit=200")["events"]
        recoveries = [event for event in events if event["event_type"] == "owner_access_recovered"]
        check(len(recoveries) == 1 and recoveries[0]["metadata"]["tokens_revoked"] == 2, "CLI recovery atomically records remaining token revocations")
        self.logs()
        cookies = [{"name": name, "value": value, "domain": "rekenraam.test", "path": "/", "secure": True, "httpOnly": True, "sameSite": "Strict"}
                   for name, value in self.owner.cookies.items() if "session" in name or "device" in name]
        (self.work / "browser-owner.json").write_text(json.dumps({"cookies": cookies, "password": RECOVERED_PASSWORD}))
        os.chmod(self.work / "browser-owner.json", 0o600)
        evidence = {"registered_patterns": self.registry_count, "independent_grants": len(GRANTS), "cases": self.rows, "commands": self.commands}
        self.output.write_text(json.dumps(evidence, indent=2) + "\n")
        with self.output.with_name("routes.csv").open("w", newline="") as file:
            writer = csv.writer(file)
            writer.writerow(["case", "expected_statuses", "actual_status"])
            for row in self.rows:
                if row["case"].startswith(("route:", "cookie-fallback:")):
                    writer.writerow([row["case"], "/".join(map(str, row["expected_statuses"])), row["status"]])
        summary = {**evidence, "cases": [row for row in self.rows if not row["case"].startswith(("route:", "cookie-fallback:"))],
                   "route_case_count": sum(row["case"].startswith(("route:", "cookie-fallback:")) for row in self.rows), "status": "passed"}
        self.output.with_name("summary.json").write_text(json.dumps(summary, indent=2) + "\n")
        print(json.dumps({"registered_patterns": self.registry_count, "independent_grants": len(GRANTS), "case_count": len(self.rows), "status": "passed"}))


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--repo", type=Path, default=Path(__file__).resolve().parents[3])
    parser.add_argument("--workdir", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    arguments = parser.parse_args()
    Audit(arguments.repo.resolve(), arguments.workdir.resolve(), arguments.output.resolve()).run()
