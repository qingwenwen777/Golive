# Deploy runbook: security fixes (PR #1 and #2), review follow-ups and operations hardening

This covers deploying the security, integrity and service-boundary changes,
the fixes from their follow-up review, and the operations hardening, and
checking that they work. None of it has been deployed yet, so one deploy
brings all of it. The deploy mechanism is described in
[`deploy-git-bare.md`](deploy-git-bare.md); its hook changed with the
operations hardening (step 1.4).

## What changes for creators, viewers and admins

| Change | What people notice |
| ---- | ---- |
| New OBS stream key | The key is now `<roomId>_<24 hex>?key=lk_…`. The part before `?` is the stream's private play name, so a fan-club-only live can't be watched by guessing its room id. Old keys are refused; creators copy the new key from Creator Studio. |
| OBS reconnects | A publisher that drops can reconnect to the same room within the grace period (each publish session is recorded to its own file and the files are joined for the replay). |
| Replays | Failed replay uploads are retried (at startup, then after 15m, 30m, 1h … up to 7 attempts) instead of their recordings being deleted. See [Replay recordings](#replay-recordings-after-this-deploy). |
| Chat | The WebSocket token is sent in a header instead of the URL. Tabs opened before the deploy must be reloaded to get chat back. Names and levels follow profile changes, and the client keeps reconnecting instead of giving up. |
| Bans | Banning a user also ends their running live. Banned admins and moderators lose their staff powers, and a banned user's session ends when their 2-hour access token expires. |
| Reports | Reports filed before the deploy are re-checked against the reported content before a moderator sees or acts on them; reports whose content is gone can only be dismissed. |
| Accounts | A display name can't be another user's username (and the reverse). Email codes: at most 5 per address and purpose per hour. |
| Gifts | Joining a fan club you're already in is refused without charging. Super-chat text is limited per tier on the server too. |

## What the operations hardening changes on the server

| Change | What it means for the operator |
| ---- | ---- |
| Go services run as UID/GID 10001 | A one-shot `init-permissions` container runs before them on every `docker compose up` and exits. It makes the upload and recording volumes (`deploy_avatar_uploads`, `deploy_cover_uploads`, `deploy_post_uploads`, `deploy_replay_records`) owned by 10001:10001, and gives group 10001 read access to `deploy/secrets` (the owner stays; for example `root:10001`, files `0640`, directories `0750`). No manual permission changes are needed. |
| Redis password | New required variable `GOLIVE_REDIS_PASSWORD`. Compose refuses to start without it. |
| Health checks | Every Go service has a `/healthz` check. api-gateway, im-gateway and nginx start only once what they depend on is healthy, so `docker compose up` waits for it (up to 5 minutes for a service that runs schema migrations). |
| Removed containers | ZooKeeper, etcd and MinIO (nothing used them). The deploy removes their containers; their volumes stay until you delete them (step 2). |
| Kafka | Optional and off by default: behind the Compose profile `kafka` (apache/kafka 3.9.2, KRaft, no ZooKeeper). chat-service no longer starts a Kafka consumer (`kafka.enabled: false`, like gift-service and im-gateway). |
| Pinned images | `redis:7.4.11-alpine`, `ossrs/srs:v5.0.213` (what `ossrs/srs:5` served until 2026-09-24), `nginx:1.27.5-alpine`; the Go service image moves to alpine 3.22. MySQL stays on `mysql:8.0` and is not restarted. |
| HTTP servers | All services have read/write/idle timeouts. pprof listens only on `127.0.0.1` inside each container. |
| Container hardening | `no-new-privileges` on every container except MySQL. The Go and observability containers run without Linux capabilities. |
| Deploy hook | Builds the images first, then runs `docker compose up` with a 20-minute limit, and removes the old Kafka container when the `kafka` profile is off. |

This deploy recreates the Go services, Redis, SRS, nginx, Jaeger, the OTel
Collector, Prometheus and Grafana. MySQL keeps running.

## 1. Before you push

1. **Back up the database, before every deploy.** The services change the
   schema themselves when they start, with no versioned migrations and no way
   to undo a change except restoring a backup (see
   [Schema changes at startup](#schema-changes-at-startup)). For example, PR #1
   makes `users.email` nullable and clears invented `@gmail.com` addresses;
   the old code cannot read the migrated `users` table, so rolling back past
   PR #1 means restoring this backup.

   ```bash
   ssh root@154.36.185.85
   cd /srv/golive/app/golive-backend/deploy
   docker compose exec -T mysql sh -c 'mysqldump -uroot -p"$MYSQL_ROOT_PASSWORD" --single-transaction golive' \
     | gzip > /root/golive-$(date +%F-%H%M).sql.gz
   ```

   **Snapshot Redis too.** Follows and likes are stored only in Redis, and this
   deploy restarts it:

   ```bash
   docker compose exec -T redis redis-cli SAVE            # prints OK
   docker cp golive-redis:/data/dump.rdb /root/golive-redis-$(date +%F-%H%M).rdb
   ```

   After this deploy the same commands keep working: `redis-cli` inside the
   container authenticates with the password on its own.

2. **Add the new secrets** to `deploy/.env` on the server. Compose refuses to
   start without them, so the deploy hook would fail half-way.

   ```bash
   cd /srv/golive/app/golive-backend/deploy
   grep -q '^GOLIVE_INTERNAL_TOKEN=' .env || echo "GOLIVE_INTERNAL_TOKEN=$(openssl rand -hex 32)" >> .env
   grep -q '^GOLIVE_REDIS_PASSWORD=' .env || echo "GOLIVE_REDIS_PASSWORD=$(openssl rand -hex 32)" >> .env
   ```

   Keep the password hex (as generated above): Compose expands `$` in `.env`.
   `GOLIVE_MINIO_ROOT_USER` and `GOLIVE_MINIO_ROOT_PASSWORD` are no longer
   used; leave them in place until you are sure you will not roll back past
   this release, because the older Compose file requires them.

3. **Check the database version.** The outbox needs MySQL 8.0+ (or MariaDB
   10.6+). The Compose file uses `mysql:8.0`.

4. **Update the deploy hook from your local checkout.** The hook that runs is
   the copy installed on the server, not the one in the commit you push, so
   install the new one first:

   ```bash
   scp scripts/post-receive.golive.example root@154.36.185.85:/srv/git/golive.git/hooks/post-receive
   ssh root@154.36.185.85 chmod +x /srv/git/golive.git/hooks/post-receive
   ```

   The deploy also works with the old hook; then remove the old Kafka
   container by hand afterwards (step 2), and note that the old hook's
   `docker compose up` has no time limit.

5. **Pick a quiet time.** Anyone streaming during the deploy is taken off
   air and has to copy the new stream key from Creator Studio into OBS (see
   the table above); OBS set up with an old key is refused. This deploy
   restarts SRS, which disconnects running lives. Even without that restart,
   room-service ends each old-format live within about two minutes and
   disconnects its OBS, so SRS closes the recording and the replay uploads
   from it. The deploy also restarts Redis and nginx, so viewers reconnect,
   and the site is unreachable for a minute or two while the services pass
   their health checks (longer if a schema migration takes long).

## 2. Deploy

From your machine, with `master` containing everything above:

```bash
git fetch origin && git checkout master && git pull
git push prod master
```

The hook rebuilds the Go images (now Go 1.26), rebuilds the frontend, and
recreates changed containers: `init-permissions` runs and exits first, then
each service starts once the services it depends on are healthy. Then on the
server:

```bash
cd /srv/golive/app/golive-backend/deploy
docker compose ps -a                     # services "Up … (healthy)"; init-permissions "Exited (0)"
docker compose logs init-permissions     # "… ready for UID/GID 10001"
docker compose logs --tail=100 user-service room-service gift-service chat-service im-gateway | grep -iE 'error|panic|fatal'
```

Expected one-off log lines:

- user-service: the email-verification migration, and `indexed legacy refresh
  tokens` (the Redis key `refresh-user-index:backfilled` marks it done; if an
  old user-service kept issuing tokens after the new one started, delete that
  key and restart user-service);
- room-service: the reconciler ending rooms that were already stuck as live
  and old-format lives (step 1.5), and retries of failed replay uploads
  (`replay upload failed; retry scheduled`, or `… giving up, recording kept
  for a manual retry`);
- gift-service: `restored super chats hidden by the old moderation to
  success`, if there are any. Moderation used to mark a paid super chat as
  failed, which left it out of creator revenue; it now stays paid and hidden;
- chat-service: `super_chat_orders.moderated_at is missing …` only if it
  starts before gift-service has migrated; super-chat history works in the
  meantime.

chat-service logs `kafka consumer disabled` on every start. Until chat-service
and gift-service both run the new build, *Delete content* on reported chat
messages and super chats fails and the report stays open; retry it after the
rollout.

If the hook stops with `dependency failed to start: container golive-… is
unhealthy`, or gives up after 20 minutes, read that service's logs
(`docker compose logs --tail=200 <service>`): a service that cannot read
`deploy/secrets`, reach Redis with the password, or finish its migrations
never becomes healthy, and what depends on it is not started.

**Once, after the first deploy of the operations hardening:**

- If you did not update the hook (step 1.4), remove the old Kafka broker:
  `docker rm -f golive-kafka`. Its service now sits behind the disabled
  `kafka` profile, so `--remove-orphans` leaves it running; the ZooKeeper,
  etcd and MinIO containers are removed automatically.
- Optionally delete the volumes nothing uses any more. MinIO's data volume is
  `deploy_minio_data`; the code never used MinIO, but check before deleting:

  ```bash
  docker run --rm -v deploy_minio_data:/data alpine:3.22 du -sh /data
  docker volume rm deploy_minio_data
  ```

  The old ZooKeeper, Kafka and etcd images kept their data in anonymous
  volumes; `docker volume ls -f dangling=true` lists unused volumes. Review
  the list before removing anything.

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

### Operations hardening

```bash
cd /srv/golive/app/golive-backend/deploy
docker compose exec user-service id              # uid=10001(golive) gid=10001(golive)
docker compose exec redis redis-cli ping         # PONG (authenticates via REDISCLI_AUTH)
docker compose exec redis env -u REDISCLI_AUTH redis-cli ping    # NOAUTH Authentication required.
docker compose exec api-gateway wget -qO- http://127.0.0.1:6060/debug/pprof/ | head -3
docker compose exec room-service wget -qO- -T 3 http://api-gateway:6060/debug/pprof/; echo "exit=$?"   # fails: pprof is loopback-only
ls -ln secrets                                   # group 10001, group-readable
docker ps -a --filter name=golive-kafka          # empty unless the kafka profile is on
```

### Manual checks (10 minutes)

1. **Streaming:** in Creator Studio, go live and copy the new stream key into
   OBS; the stream plays. Open the room as a viewer and check (dev tools →
   network) that the `.flv` URL is `/live/<roomId>_<24 hex>.flv` with no
   `key=`. Stop the live from the studio: OBS is disconnected within seconds.
2. **Fan-club-only lives:** go live with "Fan club only". Logged out, and as
   a signed-in non-member, `GET /api/rooms/<roomId>` has no `playbackUrl`,
   and `curl -sI https://golive.us.ci/live/<roomId>.flv` returns 404. The
   owner's player uses `/live/<roomId>_<24 hex>.flv` and it plays.
3. **Reconnect:** with "upload replay after end" on, go live and wait a
   minute. In OBS click Stop Streaming, then Start Streaming within 20
   seconds: the same room plays again, and
   `docker compose logs --since 10m srs | grep -i "can't append"` prints
   nothing. End the live: room-service logs `joined recording segments for
   replay upload` and the replay becomes Ready with both parts.
4. **Gifts and coins:** send a small gift; the balance drops by exactly the
   price times the count.
5. **Chat:** send a chat message; it shows your real name and level. A tab
   opened before the deploy needs a reload first.
6. **Bans:** ban a test account while it is live (Admin → Users): the live
   ends within seconds and OBS is disconnected. Ban a test moderator and sign
   in as them: `/api/rooms/admin/reports` answers 403 `user_banned`. Unban
   both afterwards.
7. **Reports:** Admin → Content shows pre-deploy reports with the reported
   content's real author and text. Reports tagged *Unverified* (content gone)
   can only be dismissed; dismiss them.
8. **Admins:** invented placeholder addresses were removed and password reset
   now needs a verified email, so admin, demo and admin-created accounts
   can't reset their password by email, and neither can invited or Google
   accounts whose email an admin changed. Make sure each admin knows their
   password; if one is locked out, another admin can set a new one
   (Admin → Users → open the user → New password).
9. **Uploads:** change your avatar, a room cover and post an image with a
   post; each one shows up (the services now write the volumes as UID 10001).
10. **Follows:** your followed channels are still listed (Redis restarted).
11. **Replays:** with Bunny configured, end a short live; the replay appears
    and room-service deletes the recordings SRS wrote
    (`docker compose exec room-service ls /tmp/golive/records`).

### Replay recordings after this deploy

- On its first start, room-service retries every failed replay upload whose
  room still has replay upload on and whose recording is on disk, 20 per
  pass. Creator Studio shows Pending upload → Uploading → Ready or Failed.
- A failed attempt is retried after 15m, 30m, 1h, 2h, 4h and 8h (7 attempts
  in all; a restart retries right away). The reason reads "(attempt N of 7,
  retrying automatically)" or "(gave up after 7 attempts)". A recording is
  deleted 7 days after its last attempt. Deleted replays and rooms with
  replay upload off are never retried; their recordings go once 24h old.
- Settings (`deploy/configs/room-service.yaml`, `replay:`; env
  `ROOMSVC_REPLAY_<NAME>`): `upload_attempts: 7`, `upload_retry_delay: 15m`,
  `failed_recording_retention: 168h`.
- Recordings take roughly 1–2 GB per live hour, and joining a reconnected
  live's files briefly doubles that. Watch the `replay_records` volume
  (`docker system df -v`) after the deploy.
- List held failures, and retry one that gave up (while its recording is
  still kept; it is picked up within a minute with fresh attempts):

  ```sql
  SELECT id, replay_attempts, replay_failed_at, replay_retry_at, replay_error FROM rooms
  WHERE status='ended' AND replay_status='failed' AND replay_upload_enabled=1 ORDER BY replay_failed_at;

  UPDATE rooms SET replay_attempts=0, replay_retry_at=NULL
  WHERE id='<room id>' AND status='ended' AND replay_status='failed' AND replay_upload_enabled=1;
  ```

## 4. Full smoke test (local or staging only)

`scripts/smoke/smoke.py` without `--public` runs 45 checks covering every
Critical and High fix: stream-key and play-name handling (including
fan-club-only lives), SRS hook authentication, gift/top-up overflow, bets,
daily-reward races, appointments, reports, moderator permissions, bans, chat
identity, the WebSocket token and connection caps, and the im-gateway crash.
It creates test accounts, reads login captchas from Redis and calls
room-service directly, so **never run it against production**.

```bash
# Services running locally with their app/*/configs/config.yaml, MySQL and Redis on 127.0.0.1.
python3 scripts/smoke/fake_srs.py &                  # stands in for SRS's HTTP API on :1985
SMOKE_MYSQL="mysql -ugolive -pgolive golive" python3 scripts/smoke/seed_users.py
GOLIVE_SRS_FAKE=http://127.0.0.1:1985 python3 scripts/smoke/smoke.py
```

When the script had 40 checks, the code before PR #1 (`b19017d`) failed 23 of
them. The current code passes all 45 (and `--public` passes its 4).

## 5. Rollback

- **The review follow-ups or the operations hardening misbehave:** redeploy
  the merge commit of PR #2 (`git push -f prod bf58781:master`).
  - Their schema changes are additive (`rooms.replay_attempts`,
    `replay_failed_at`, `replay_retry_at`), so PR #2's code runs on them.
  - Creators copy their stream key into OBS again (the format goes back to
    `<roomId>?key=…`).
  - PR #2's cleanup deletes recordings of failed uploads once they are 24h
    old, and doesn't know the new per-session file names. Copy any
    recordings you want to keep out of `/tmp/golive/records` first.
  - The older Compose file runs the services as root again (root can use the
    10001-owned volumes and the group-readable secrets, so nothing needs
    undoing), restarts Redis without a password, and recreates ZooKeeper,
    Kafka, etcd and MinIO, so `deploy/.env` must still contain
    `GOLIVE_MINIO_ROOT_USER` and `GOLIVE_MINIO_ROOT_PASSWORD`. The new hook
    works with the older Compose file.
  - To roll back only the operations hardening, revert its merge commit on a
    branch (`git revert -m 1 79b8ef5`) and push that instead.
- **Only PR #2 misbehaves:** redeploy the merge commit of PR #1
  (`git push -f prod 6b7a627:master`). PR #2's schema changes are additive
  (new columns and indexes), so PR #1's code runs on them. PR #1's
  chat-service shows every paid super chat in public history, so super chats
  hidden by moderation since PR #2 (and the older ones this release converts,
  see step 2) show up again.
- **Rolling back past PR #1:** restore the backup from step 1 first, then
  push the older commit. The pre-fix code cannot read NULL emails.

## 6. Known risks and follow-ups

### Schema changes at startup

Each service migrates the shared database when it starts, before it opens
its HTTP port: GORM `AutoMigrate` for the tables it owns (it adds tables,
columns and indexes and can change column types, but never drops anything),
plus one-off data fixes in code (for example user-service's email
verification migration and room-service's legacy channel-id fix).

- Nothing records which schema version the database is at, there is no dry
  run, and there are no down migrations: rolling back the code does not roll
  back the schema. Restoring the backup from step 1.1 is the only undo.
- A migration that fails part-way can leave the schema partly changed; the
  service exits and tries again on every restart.
- A long migration (an index on a big table) delays the service's health
  check. After the 5-minute start period the deploy stops with "unhealthy"
  while the migration may still be running; wait for it to finish, then run
  `docker compose up -d` again.
- Some shared tables are migrated by more than one service (see Data ownership
  in `golive-backend/README.md`), and those services start at the same time.

So: back up before every deploy (step 1.1), deploy at a quiet time, and check
the logs for migration errors (step 2).

**Follow-up: versioned migrations.** Check SQL migrations into the repository
(for example with golang-migrate or Atlas), apply them from a one-shot Compose
job that runs before the services (like `init-permissions`), record the
applied version in the database, review down migrations with each change, and
remove `AutoMigrate` from service startup.

### Other follow-ups

- MySQL 8.0 reached end of life in April 2026; plan the move to 8.4 LTS with a
  tested backup and restore.
- nginx 1.27 no longer receives updates; move to 1.28 (stable).
- SRS v5.0.225 (`v5.0-r4`) is available; try it on staging before changing
  the pin.
- Kafka 4.x needs a newer franz-go client, so the optional profile stays on 3.9.
- Follows and likes exist only in Redis, persisted by RDB snapshots (a crash
  can lose up to an hour of changes). Consider AOF (`appendonly yes`) or
  moving them to MySQL.
- MySQL, Redis, SRS and nginx still start as root with Docker's default
  capabilities, and the Go services could run with read-only root
  filesystems.
- nginx's `worker_connections 2048` allows about 1024 proxied WebSockets per
  worker process. Raise it (with `worker_rlimit_nofile`) before a big event,
  and consider a per-IP `limit_conn` on `/ws`.

### Known gaps in the application

- Mic-link guests' audio streams (`miclink-<roomId>-<userId>`) have guessable
  names, so a non-member can play a fan-club-only room's guest audio over
  WHEP (never the host's audio or video). Fixing it needs a per-guest secret
  across gift-service, `pkg/miclink`, the web client and the OBS stage page.
- The cap of 5 email codes per address per hour also lets someone who knows
  a user's username and email block that user's password reset for an hour.
  Per-IP limits at the gateway would close this. Sign-up also uses up the
  email code before it checks the username and invite.
- Joined replay files are tested with synthetic FLV only. Check one from a
  real reconnect with `ffprobe` on staging, ideally with an encoder settings
  change between the two sessions.
- The chat word filter can join a short entry across one punctuation mark
  ("s/b" matches "sb") and still matches four-letter entries inside longer
  words; it needs an allow list.
- Room moderators' mute/unmute and fan-group admins' member changes don't
  check whether the acting user is banned.
- A publisher still streaming into a room that was ended while SRS's API was
  unreachable is not kicked later.
- A fan-club member whom the creator blocked still gets the playback URL from
  the library lists (room detail hides the room from them).
- A room's channel label is chosen by whoever goes live, so it can copy
  another creator's name (links no longer follow it).
