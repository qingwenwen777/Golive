"""End-to-end smoke test for the security and correctness fixes.

Talks to a GoLive deployment through the api-gateway, like the web client.

Modes:
  --public   Anonymous, read-only checks only. Safe to run against production.
  (default)  Full run. Needs test accounts (seed_users.py), Redis access to read
             login captchas, and direct access to room-service for the SRS hook
             checks. Use it on a local or staging stack, never production.

Environment:
  GOLIVE_API       gateway base URL                 (default http://127.0.0.1:8080)
  GOLIVE_WS        im-gateway WebSocket URL         (default ws://127.0.0.1:8081/ws)
  GOLIVE_ORIGIN    an allowed browser origin        (default http://localhost:5173)
  GOLIVE_ROOMSVC   room-service base URL (SRS hooks) (default http://127.0.0.1:8091)
  GOLIVE_USERSVC   user-service base URL (internal-API auth check) (default http://127.0.0.1:8090)
  GOLIVE_SRS_FAKE  fake SRS control URL (fake_srs.py); publisher-kick check skipped when unset
  SMOKE_REDIS_CLI  command that runs redis-cli       (default "redis-cli")
  USERS_JSON       accounts file written by seed_users.py (default users.json)
"""
import asyncio, json, os, shlex, subprocess, sys, threading, uuid
import requests

API = os.environ.get("GOLIVE_API", "http://127.0.0.1:8080")
WS = os.environ.get("GOLIVE_WS", "ws://127.0.0.1:8081/ws")
ORIGIN = os.environ.get("GOLIVE_ORIGIN", "http://localhost:5173")
ROOMSVC = os.environ.get("GOLIVE_ROOMSVC", "http://127.0.0.1:8091")
SRS_FAKE = os.environ.get("GOLIVE_SRS_FAKE", "")
USERSVC = os.environ.get("GOLIVE_USERSVC", "http://127.0.0.1:8090")
REDIS_CLI = shlex.split(os.environ.get("SMOKE_REDIS_CLI", "redis-cli"))
USERS = {}


def captcha_answer(captcha_id):
    return subprocess.check_output(REDIS_CLI + ["GET", f"captcha:{captcha_id}"], text=True).strip()

results = []
TOKENS = []


def check(name, ok, detail=""):
    results.append((name, bool(ok), detail))
    print(f"[{'PASS' if ok else 'FAIL'}] {name}" + ("" if ok else f"\n        {detail}"))


class Client:
    def __init__(self, name):
        self.name = name
        self.s = requests.Session()
        # One client IP per user so the per-IP auth rate limit doesn't bite.
        self.s.headers.update({"Origin": ORIGIN, "X-Real-IP": f"10.9.0.{len(name) * 7 % 250 + 1}"})
        self.token = self.refresh = None
        self.id = USERS[name]["id"]

    def csrf(self):
        r = self.s.get(f"{API}/api/csrf-token")
        self.s.headers["X-CSRF-Token"] = r.json()["token"]

    def login(self):
        self.csrf()
        cap = self.s.get(f"{API}/api/auth/captcha").json()
        code = captcha_answer(cap["id"])
        r = self.s.post(f"{API}/api/auth/login", json={
            "username": USERS[self.name]["username"], "password": USERS[self.name]["password"],
            "captchaId": cap["id"], "captchaCode": code})
        if r.status_code == 200:
            body = r.json()
            self.token, self.refresh = body.get("token"), body.get("refreshToken")
            self.s.headers["Authorization"] = f"Bearer {self.token}"
        return r

    def req(self, method, path, **kw):
        return self.s.request(method, f"{API}{path}", **kw)


def srs_hook(action, stream, param="", client_id="", app="live"):
    r = requests.post(f"{ROOMSVC}/srs/{action}", json={
        "action": action, "client_id": client_id, "app": app, "stream": stream, "param": param})
    return r.json().get("code")


def public_checks():
    """Anonymous, read-only checks: safe against production."""
    h = {"Origin": ORIGIN}
    r = requests.get(f"{API}/api/rooms?size=100000", headers=h)
    check("room list page size is capped at 100", r.ok and r.json().get("size", 0) <= 100, f"{r.status_code} {r.text[:200]}")
    rooms = r.json().get("items", []) if r.ok else []
    leaked = [i.get("id") for i in rooms if "key=" in i.get("playbackUrl", "") or "/lk_" in i.get("playbackUrl", "")]
    check("no live room's public playback URL contains a publish key",
          r.ok and not leaked and "streamKey" not in r.text, f"leaking rooms: {leaked}")
    r = requests.get(f"{API}/api/rooms/replays/hot", headers=h)
    check("hot replays expose no revenue", r.ok and "revenueCoin" not in r.text, f"{r.status_code} {r.text[:200]}")
    channels = {i.get("channelId") for i in rooms + (r.json().get("items", []) if r.ok else []) if i.get("channelId")}
    for ch in sorted(channels)[:5]:
        hr = requests.get(f"{API}/api/rooms/channels/{ch}/history", headers=h)
        check(f"public history of {ch} hides revenue and top fans",
              hr.ok and "revenueCoin" not in hr.text and "topFan" not in hr.text, f"{hr.status_code} {hr.text[:200]}")
    r = requests.get(f"{API}/api/internal/users/x/permission", headers=h)
    check("internal APIs are not reachable through the gateway", r.status_code == 404, f"{r.status_code}")


def finish():
    failed = [n for n, ok, _ in results if not ok]
    print(f"\n{len(results) - len(failed)}/{len(results)} checks passed")
    sys.exit(1 if failed else 0)


def main():
    if "--public" in sys.argv:
        public_checks()
        finish()
    USERS.update(json.load(open(os.environ.get("USERS_JSON", "users.json"))))
    c = {n: Client(n) for n in USERS}
    for n, cl in c.items():
        r = cl.login()
        if r.status_code != 200:
            print(f"login {n} failed: {r.status_code} {r.text}")
            sys.exit(2)
    admin, creator, viewer, attacker, mod, victim = (c[k] for k in
        ("admin", "creator", "viewer", "attacker", "mod", "victim"))

    # ---------------- Critical 1: stream key never reaches viewers ----------------
    r = creator.req("POST", "/api/rooms/live", json={"title": "Smoke stream", "category": "Gaming",
                                                     "channelName": "Smoke Creator"})
    room = r.json() if r.ok else {}
    room_id, obs_key = room.get("id", ""), room.get("streamKey", "")
    secret = obs_key.split("key=", 1)[1] if "key=" in obs_key else ""
    check("go live returns an OBS key of the form <roomId>?key=<secret>",
          r.ok and obs_key.startswith(room_id + "?key=") and secret, f"{r.status_code} {r.text[:300]}")
    check("SRS publish without a key is rejected", srs_hook("on_publish", room_id) != 0)
    check("SRS publish with a wrong key is rejected", srs_hook("on_publish", room_id, "?key=lk_guess") != 0)
    check("SRS publish with the leaked-style bare key as stream name is rejected",
          srs_hook("on_publish", secret) != 0)
    check("SRS publish with the correct key is accepted",
          srs_hook("on_publish", room_id, "?key=" + secret, client_id="cid-smoke-1") == 0)
    r = requests.get(f"{API}/api/rooms", headers={"Origin": ORIGIN})
    items = r.json().get("items", []) if r.ok else []
    ours = [i for i in items if i.get("id") == room_id]
    pub = json.dumps(r.json()) if r.ok else ""
    check("public room list shows the room with playback URL /live/<roomId>.flv",
          ours and ours[0].get("playbackUrl", "").endswith(f"/{room_id}.flv"), json.dumps(ours)[:300])
    check("public room list never contains the publish secret", secret and secret not in pub)
    check("miclink-* stream without a token is rejected", srs_hook("on_publish", f"miclink-{room_id}-x") != 0)

    # ---------------- Critical 2: gift count overflow ----------------
    def balance(cl):
        return cl.req("GET", "/api/users/me").json().get("coinBalance")
    before = balance(viewer)
    r = viewer.req("POST", "/api/gifts/send", headers={"X-Request-Id": str(uuid.uuid4())},
                   json={"roomId": room_id, "giftId": "flower", "count": 1844674407270955161})
    check("gift with overflowing count is rejected (400)", r.status_code == 400, f"{r.status_code} {r.text[:200]}")
    check("balance unchanged after rejected overflow gift", balance(viewer) == before, f"{before} -> {balance(viewer)}")
    r = viewer.req("POST", "/api/gifts/send", headers={"X-Request-Id": str(uuid.uuid4())},
                   json={"roomId": room_id, "giftId": "flower", "count": 2})
    after = balance(viewer)
    check("a normal gift still works and debits price*count", r.ok and after == before - 20,
          f"{r.status_code} {r.text[:200]} balance {before}->{after}")

    # ---------------- Critical 3: Stripe top-up overflow ----------------
    r = viewer.req("POST", "/api/users/me/coins/topup", json={"amount": 4611686018427387914})
    check("top-up above the cap is rejected (400)", r.status_code == 400, f"{r.status_code} {r.text[:200]}")

    # ---------------- High: bets ----------------
    r = creator.req("POST", "/api/bets", json={"roomId": room_id, "amount": 10, "question": "Win?"})
    bet = r.json() if r.ok else {}
    bet_id = bet.get("id") or bet.get("round", {}).get("id", "")
    check("host can open a bet", r.ok and bet_id, f"{r.status_code} {r.text[:200]}")
    r = creator.req("POST", f"/api/bets/{bet_id}/wagers", json={"roomId": room_id, "option": "lose"})
    check("host cannot wager on their own round (403)", r.status_code == 403, f"{r.status_code} {r.text[:200]}")
    r = creator.req("POST", f"/api/bets/{bet_id}/settle", json={"option": "lose"})
    check("host cannot settle before betting closes (409)", r.status_code == 409, f"{r.status_code} {r.text[:200]}")

    # ---------------- High: daily reward claimed once under parallel requests ----------------
    task = "daily-login-lottery"
    start = balance(attacker)
    codes = []
    def claim():
        codes.append(attacker.req("POST", f"/api/users/me/coins/daily-tasks/{task}/claim").status_code)
    ts = [threading.Thread(target=claim) for _ in range(20)]
    [t.start() for t in ts]; [t.join() for t in ts]
    txs = attacker.req("GET", "/api/users/me/coins/transactions").json().get("items", [])
    daily = [t for t in txs if t.get("type") == "daily_task"]
    check("20 parallel daily-reward claims credit exactly once", len(daily) == 1,
          f"ledger rows={len(daily)} codes={sorted(set(codes))} balance {start}->{balance(attacker)}")

    # ---------------- High: appointments need creator approval ----------------
    r = viewer.req("POST", "/api/rooms/appointments", json={
        "title": "Sneaky stream", "category": "Gaming", "scheduledAt": "2030-01-01T00:00:00Z"})
    check("non-approved user cannot create an appointment (403)", r.status_code == 403, f"{r.status_code} {r.text[:200]}")

    # ---------------- High: reports can't be aimed at someone else ----------------
    r = creator.req("POST", "/api/rooms/posts", json={"content": "smoke post", "visibility": "public"})
    post = r.json() if r.ok else {}
    post_id = post.get("id") or post.get("post", {}).get("id", "")
    r = attacker.req("POST", "/api/rooms/reports", json={
        "targetType": "post", "targetId": post_id, "reason": "spam",
        "targetUserId": victim.id, "targetOwnerId": victim.id, "targetUrl": "https://evil.example/phish"})
    rep = r.json() if r.ok else {}
    rep_body = json.dumps(rep)
    check("report stores the real author, not the reporter-supplied user",
          r.ok and victim.id not in rep_body and "evil.example" not in rep_body,
          f"{r.status_code} {rep_body[:300]}")

    # ---------------- High: moderator role is not admin ----------------
    r = mod.req("GET", "/api/rooms/admin/system-settings")
    check("moderator cannot read system settings (403)", r.status_code == 403, f"{r.status_code} {r.text[:200]}")
    r = mod.req("GET", "/api/rooms/admin/reports")
    check("moderator can still review reports", r.ok, f"{r.status_code} {r.text[:200]}")

    # ---------------- High: revenue is owner-only ----------------
    r = requests.get(f"{API}/api/rooms/replays/hot", headers={"Origin": ORIGIN})
    check("hot replays expose no revenue", r.ok and "revenueCoin" not in r.text, f"{r.status_code} {r.text[:200]}")

    # ---------------- High: live chat identity is server-assigned ----------------
    TOKENS[:] = [cl.token for cl in (creator, attacker, mod, admin, victim)]
    chat = asyncio.run(chat_checks(viewer, creator, room_id))
    for name, ok, detail in chat:
        check(name, ok, detail)

    # ---------------- High: bans ----------------
    old_refresh = victim.refresh
    r = admin.req("PATCH", f"/api/admin/users/{victim.id}/ban", json={"banned": True, "reason": "smoke test"})
    check("admin can ban a user", r.ok, f"{r.status_code} {r.text[:200]}")
    r = victim.req("POST", "/api/auth/refresh", json={"refreshToken": old_refresh})
    check("ban revokes the user's existing refresh token", r.status_code in (401, 403), f"{r.status_code} {r.text[:200]}")
    r = victim.req("PATCH", "/api/users/me/profile", json={"displayName": "still here"})
    check("banned user cannot edit their profile (403 user_banned)",
          r.status_code == 403 and "user_banned" in r.text, f"{r.status_code} {r.text[:200]}")
    r = victim.login()
    check("banned user can still sign in (to appeal)", r.status_code == 200, f"{r.status_code} {r.text[:200]}")

    # ---------------- Stop live kicks the SRS publisher ----------------
    kicks_before = requests.get(f"{SRS_FAKE}/_kicks").json() if SRS_FAKE else []
    r = creator.req("DELETE", "/api/rooms/live")
    check("creator can stop the live", r.ok, f"{r.status_code} {r.text[:200]}")
    if SRS_FAKE:
        kicks = requests.get(f"{SRS_FAKE}/_kicks").json()
        new_kicks = kicks[len(kicks_before):]
        check("stopping the live disconnects the publisher from SRS", "cid-smoke-1" in new_kicks, f"new kicks={new_kicks}")

    # The stream has now ended with gift revenue, so there is something to leak.
    r = requests.get(f"{API}/api/rooms/channels/ch-{creator.id}/history", headers={"Origin": ORIGIN})
    ended = room_id in r.text
    check("ended stream appears in the public channel history", r.ok and ended, f"{r.status_code} {r.text[:200]}")
    check("public channel history hides the ended stream's revenue and top fans",
          r.ok and ended and "revenueCoin" not in r.text and "topFan" not in r.text, f"{r.status_code} {r.text[:300]}")
    r = creator.req("GET", f"/api/rooms/channels/ch-{creator.id}/history")
    check("the channel owner still sees their revenue", r.ok and "revenueCoin" in r.text, f"{r.status_code} {r.text[:200]}")

    # ---------------- Medium / boundaries ----------------
    r = requests.get(f"{API}/api/rooms?size=100000", headers={"Origin": ORIGIN})
    check("room list page size is capped at 100", r.ok and r.json().get("size", 0) <= 100, f"{r.status_code} {r.text[:200]}")
    r = viewer.req("POST", "/api/rooms/ch-no-such-channel-xyz/follow")
    check("following a channel that doesn't exist is refused (404)", r.status_code == 404, f"{r.status_code} {r.text[:200]}")
    r = requests.get(f"{API}/api/internal/users/{victim.id}/permission", headers={"Origin": ORIGIN})
    check("internal APIs are not reachable through the gateway", r.status_code == 404, f"{r.status_code}")
    r = requests.post(f"{USERSVC}/internal/users/{victim.id}/restriction",
                      json={"action": "unban"})
    check("internal APIs reject calls without the shared token", r.status_code in (401, 403), f"{r.status_code} {r.text[:200]}")

    finish()


async def chat_checks(sender, other, room_id):
    import websockets
    out = []
    url = f"{WS}?roomId={room_id}"

    def connect(token, target=url):
        # Like the web client: the access token rides in Sec-WebSocket-Protocol
        # next to golive.v1, never in the URL (URLs end up in access logs).
        return websockets.connect(target, subprotocols=["golive.v1", f"auth.{token}"],
                                  additional_headers={"Origin": ORIGIN})

    async with connect(other.token) as watch, connect(sender.token) as ws:
        out.append(("im-gateway answers with golive.v1, never echoing the token", ws.subprotocol == "golive.v1",
                    f"subprotocol={(ws.subprotocol or '')[:16]!r}"))
        await asyncio.sleep(0.5)
        await ws.send(json.dumps({"type": "chat", "text": "hello from smoke", "clientId": "smoke-client-1",
                                  "username": "Smoke Creator", "userLevel": 99, "fanBadge": {"name": "fake"}}))
        got = None
        try:
            async with asyncio.timeout(5):
                while True:
                    msg = json.loads(await watch.recv())
                    if msg.get("type") == "chat" and "hello from smoke" in json.dumps(msg):
                        got = msg
                        break
        except TimeoutError:
            pass
        detail = json.dumps(got)[:300]
        out.append(("chat message is broadcast", got is not None, detail))
        if got:
            blob = json.dumps(got)
            out.append(("chat ignores a client-supplied name", "Smoke Creator" not in blob, detail))
            out.append(("chat ignores a client-supplied level 99", '"userLevel": 99' not in blob, detail))
            out.append(("chat message id is server-assigned, not the client id",
                        got.get("id") and got.get("id") != "smoke-client-1", detail))
    # A token in the URL is refused even next to a valid one in the header.
    status = None
    try:
        async with connect(sender.token, target=f"{url}&token=smoke-in-url"):
            pass
    except websockets.exceptions.InvalidStatus as e:
        status = e.response.status_code
    out.append(("a token in the WebSocket URL is refused (400)", status == 400, f"status={status}"))
    # Per-user connection cap: the 9th socket for one user is refused.
    same_user, capped = [], None
    try:
        for _ in range(9):
            same_user.append(await connect(other.token))
    except websockets.exceptions.InvalidStatus as e:
        capped = e.response.status_code
    for c in same_user:
        await c.close()
    out.append(("one user can't open unlimited sockets (429 after the cap)", capped == 429, f"status={capped} opened={len(same_user)}"))
    # Many connections that vanish mid-broadcast used to crash the gateway.
    conns = []
    for tok in TOKENS:
        for _ in range(6):
            conns.append(await connect(tok))
    async with connect(sender.token) as ws:
        for i in range(20):
            await ws.send(json.dumps({"type": "chat", "text": f"burst {i}", "clientId": f"b{i}"}))
            if i == 5:
                for c in conns:
                    c.transport.abort()
            await asyncio.sleep(0.05)
        await asyncio.sleep(1)
    r = requests.get(WS.replace("ws://", "http://").replace("/ws", "/healthz"))
    out.append(("im-gateway survives abrupt disconnects during broadcast", r.status_code == 200, f"{r.status_code}"))
    return out


if __name__ == "__main__":
    main()
