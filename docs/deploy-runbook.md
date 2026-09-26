# Deploy runbook: security fixes (PR #1 and #2)

This covers deploying the security, integrity and service-boundary changes, and
checking that they work. The normal deploy mechanism is unchanged; see
[`deploy-git-bare.md`](deploy-git-bare.md).

## 1. Before you push

1. **Back up the database.** Both PRs migrate the schema on startup (for
   example `users.email` becomes nullable and invented `@gmail.com` addresses
   are cleared). The old code cannot read the migrated `users` table, so
   rolling back past PR #1 means restoring this backup.

   ```bash
   ssh root@154.36.185.85
   cd /srv/golive/app/golive-backend/deploy
   docker compose exec -T mysql sh -c 'mysqldump -uroot -p"$MYSQL_ROOT_PASSWORD" --single-transaction golive' \
     | gzip > /root/golive-$(date +%F-%H%M).sql.gz
   ```

2. **Add the internal-API secret** to `deploy/.env` on the server. Compose
   refuses to start without it, so the deploy hook would fail half-way.

   ```bash
   echo "GOLIVE_INTERNAL_TOKEN=$(openssl rand -hex 32)" >> /srv/golive/app/golive-backend/deploy/.env
   ```

3. **Check the database version.** The outbox needs MySQL 8.0+ (or MariaDB
   10.6+). The Compose file uses `mysql:8.0`.

4. **Pick a quiet time.** Anyone streaming during the deploy is disconnected
   and has to copy the new stream key into OBS (the key format changed to
   `<roomId>?key=…`).

## 2. Deploy

From your machine, with `master` containing both PRs:

```bash
git fetch origin && git checkout master && git pull
git push prod master
```

The hook rebuilds the Go images (now Go 1.26), rebuilds the frontend, and
recreates changed containers. Then on the server:

```bash
cd /srv/golive/app/golive-backend/deploy
docker compose ps                        # everything "running"/"healthy"
docker compose logs --tail=100 user-service room-service gift-service chat-service im-gateway | grep -iE 'error|panic|fatal'
```

Expected one-off log lines: the email-verification migration in user-service,
and room-service's reconciler ending any rooms that were already stuck as live.

## 3. Verify

### Automated, read-only (safe against production)

Needs Python 3 with `requests` and `websockets` (`pip install requests websockets`).

```bash
GOLIVE_API=https://golive.us.ci GOLIVE_ORIGIN=https://golive.us.ci \
  python3 scripts/smoke/smoke.py --public
```

It checks, without logging in or changing anything:

- no live room's public playback URL contains a publish key,
- hot replays and public channel histories hide revenue and top fans,
- `/api/internal/*` is not reachable through the gateway,
- list endpoints cap the page size.

### The SRS API the room reconciler relies on

```bash
docker compose exec room-service wget -qO- http://srs:1985/api/v1/streams/
```

It should print JSON with `"code":0` and a `streams` array. If it doesn't,
the reconciler only ends rooms whose disconnect was recorded (it never
mass-ends rooms), but tell the maintainers.

### Manual checks (5 minutes)

1. **Streaming:** in Creator Studio, go live and copy the new stream key into
   OBS; the stream plays. Open the room as a viewer and check (dev tools →
   network) that the `.flv` URL is `/live/<roomId>.flv` with no `key=`.
   Stop the live from the studio: OBS is disconnected within seconds.
2. **Gifts and coins:** send a small gift; the balance drops by exactly the
   price times the count.
3. **Chat:** send a chat message; it shows your real name and level.
4. **Admins:** invented placeholder addresses were removed and password reset
   now needs a verified email, so admin, demo and admin-created accounts
   can't reset their password by email. Make sure each admin knows their
   password; if one is locked out, another admin can set a new one
   (Admin → Users → open the user → New password).

## 4. Full smoke test (local or staging only)

`scripts/smoke/smoke.py` without `--public` runs 40 checks covering every
Critical and High fix: stream-key handling, SRS hook authentication,
gift/top-up overflow, bets, daily-reward races, appointments, reports,
moderator permissions, bans, chat identity, WebSocket connection caps and the
im-gateway crash. It creates test accounts, reads login captchas from Redis
and calls room-service directly, so **never run it against production**.

```bash
# Services running locally with their app/*/configs/config.yaml, MySQL and Redis on 127.0.0.1.
python3 scripts/smoke/fake_srs.py &                  # stands in for SRS's HTTP API on :1985
SMOKE_MYSQL="mysql -ugolive -pgolive golive" python3 scripts/smoke/seed_users.py
GOLIVE_SRS_FAKE=http://127.0.0.1:1985 python3 scripts/smoke/smoke.py
```

On the pre-fix code (`b19017d`) the same run fails 23 of the 40 checks; on the
fixed code all 40 pass.

## 5. Rollback

- **Only PR #2 misbehaves:** redeploy the merge commit of PR #1
  (`git push -f prod 6b7a627:master`). PR #2's schema changes are additive
  (new columns and indexes), so PR #1's code runs on them.
- **Rolling back past PR #1:** restore the backup from step 1 first, then
  push the older commit. The pre-fix code cannot read NULL emails.
