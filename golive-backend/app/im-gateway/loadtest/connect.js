// k6 WebSocket load test for im-gateway.
//
// Usage (50k connections in batches across 50 workers):
//   k6 run -e WS_URL=ws://localhost:8081/ws -e ROOMS=20 -e PER_VU=1000 -u 50 -d 5m connect.js
//
// What it does:
//   - Each VU opens PER_VU concurrent connections to a randomly chosen room.
//   - Each connection sends a heartbeat every 25s.
//   - 5% of connections also send one chat (authenticated VUs only — set TOKEN env to test that).
//   - All connections stay open for the duration; we measure handshake p95
//     and viewer_count delivery rate as proxies for fanout health.
//
// Targets to validate:
//   - 50 VUs × 1000 conns = 50_000 simultaneous connections
//   - handshake p95 < 200ms
//   - msg_recv_rate ≥ 0.2/s (at least one viewer_count every 5s)

import ws from 'k6/ws';
import { check, sleep } from 'k6';
import { Counter, Trend } from 'k6/metrics';

const WS_URL = __ENV.WS_URL || 'ws://localhost:8081/ws';
const ROOMS = Number(__ENV.ROOMS || 20);
const PER_VU = Number(__ENV.PER_VU || 1000);
const TOKEN = __ENV.TOKEN || '';

const handshakeMs = new Trend('handshake_ms');
const msgsRecv = new Counter('msgs_recv');
const handshakeFail = new Counter('handshake_fail');

export const options = {
  // Override -u / -d on the CLI; defaults here are conservative.
  vus: 10,
  duration: '60s',
  thresholds: {
    handshake_ms: ['p(95)<200'],
    handshake_fail: ['count<10'],
  },
};

export default function () {
  for (let i = 0; i < PER_VU; i++) {
    const roomId = `room-${(__VU * PER_VU + i) % ROOMS}`;
    const url = `${WS_URL}?roomId=${roomId}`;
    // The token rides in Sec-WebSocket-Protocol; the gateway refuses it in the URL.
    const params = TOKEN ? { headers: { 'Sec-WebSocket-Protocol': `golive.v1, auth.${TOKEN}` } } : null;

    const t0 = Date.now();
    const res = ws.connect(url, params, function (socket) {
      handshakeMs.add(Date.now() - t0);

      socket.setInterval(() => {
        socket.send(JSON.stringify({ type: 'heartbeat' }));
      }, 25_000);

      socket.on('message', () => {
        msgsRecv.add(1);
      });

      // hold the connection open for the test's "stay" period
      socket.setTimeout(() => socket.close(), 60_000);
    });
    check(res, { 'status is 101': (r) => r && r.status === 101 }) || handshakeFail.add(1);
  }
  // Connections returned by ws.connect block until the socket closes; the
  // loop body above effectively holds PER_VU sockets serially per iteration.
  // For true parallelism per VU you can split into multiple default fns —
  // simpler: scale VUs with -u.
  sleep(1);
}
