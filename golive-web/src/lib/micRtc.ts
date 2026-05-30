// Minimal WHIP/WHEP client for SRS WebRTC, used by mic-link.
//
// - Guests PUBLISH their microphone (audio-only) via WHIP.
// - The OBS mic-stage page SUBSCRIBES to a guest's audio via WHEP.
//
// SRS exposes the standard WebRTC HTTP ingest/egress endpoints under /rtc/v1/.
// We post the local SDP offer and apply the returned answer. Media itself
// flows over UDP to the SRS candidate; only signaling goes through HTTPS.

const WHIP_PATH = '/rtc/v1/whip/';
const WHEP_PATH = '/rtc/v1/whep/';

// streamUrl is the SRS-style URL identifying the stream, e.g.
// `webrtc://<host>/live/<streamName>`. SRS uses it to route the session.
function streamUrl(streamName: string): string {
  const host = location.host;
  return `webrtc://${host}/live/${streamName}`;
}

async function signal(path: string, offerSdp: string): Promise<string> {
  const res = await fetch(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/sdp' },
    body: offerSdp,
    // SRS also accepts the api/streamurl as query params for routing.
  });
  if (!res.ok) {
    throw new Error(`mic signaling failed: ${res.status}`);
  }
  return res.text();
}

export interface MicPublishHandle {
  pc: RTCPeerConnection;
  stream: MediaStream;
  close: () => void;
  setMuted: (muted: boolean) => void;
}

// publishMic captures the microphone and publishes it (audio-only) to SRS via
// WHIP under the given stream name.
export async function publishMic(streamName: string): Promise<MicPublishHandle> {
  const stream = await navigator.mediaDevices.getUserMedia({ audio: true, video: false });
  const pc = new RTCPeerConnection({
    iceServers: [{ urls: 'stun:stun.l.google.com:19302' }],
  });
  for (const track of stream.getAudioTracks()) {
    pc.addTrack(track, stream);
  }
  // Audio uplink only.
  pc.addTransceiver('audio', { direction: 'sendonly' });

  const offer = await pc.createOffer();
  await pc.setLocalDescription(offer);
  const answer = await signal(
    `${WHIP_PATH}?streamurl=${encodeURIComponent(streamUrl(streamName))}`,
    offer.sdp ?? '',
  );
  await pc.setRemoteDescription({ type: 'answer', sdp: answer });

  return {
    pc,
    stream,
    setMuted: (muted: boolean) => {
      for (const track of stream.getAudioTracks()) {
        track.enabled = !muted;
      }
    },
    close: () => {
      for (const track of stream.getTracks()) track.stop();
      pc.close();
    },
  };
}

export interface MicPlayHandle {
  pc: RTCPeerConnection;
  stream: MediaStream;
  close: () => void;
}

// playMic subscribes to a published guest stream (audio-only) via WHEP and
// returns a MediaStream the caller can attach to an <audio> element.
export async function playMic(streamName: string): Promise<MicPlayHandle> {
  const pc = new RTCPeerConnection({
    iceServers: [{ urls: 'stun:stun.l.google.com:19302' }],
  });
  const remote = new MediaStream();
  pc.addTransceiver('audio', { direction: 'recvonly' });
  pc.ontrack = (event) => {
    for (const track of event.streams[0]?.getTracks() ?? [event.track]) {
      remote.addTrack(track);
    }
  };

  const offer = await pc.createOffer();
  await pc.setLocalDescription(offer);
  const answer = await signal(
    `${WHEP_PATH}?streamurl=${encodeURIComponent(streamUrl(streamName))}`,
    offer.sdp ?? '',
  );
  await pc.setRemoteDescription({ type: 'answer', sdp: answer });

  return {
    pc,
    stream: remote,
    close: () => pc.close(),
  };
}

// micStreamName builds the per-guest stream name used on both ends. The
// `miclink-` prefix lets room-service's SRS on_publish hook accept it without
// room bookkeeping.
export function micStreamName(roomId: string, userId: string): string {
  return `miclink-${roomId}-${userId}`;
}
