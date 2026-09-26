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

export type MicSetupReason =
  | 'insecure_context'
  | 'permission_denied'
  | 'no_device'
  | 'getusermedia_failed'
  | 'signaling_failed';

// MicSetupError carries a precise machine-readable reason so the UI can show
// an accurate message instead of always blaming microphone permission.
export class MicSetupError extends Error {
  reason: MicSetupReason;
  constructor(reason: MicSetupReason, message: string) {
    super(message);
    this.name = 'MicSetupError';
    this.reason = reason;
  }
}

// All mic-link streams live under SRS app "live". SRS parses app+stream from
// the query string (proven against SRS 5.0.213). SRS forwards the whole query
// string to the on_publish hook, so the guest's publish token rides along as
// `key` for room-service to verify.
function signalUrl(path: string, streamName: string, publishToken?: string): string {
  const params = new URLSearchParams({ app: 'live', stream: streamName });
  if (publishToken) params.set('key', publishToken);
  return `${path}?${params.toString()}`;
}

async function signal(path: string, offerSdp: string): Promise<string> {
  const res = await fetch(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/sdp' },
    body: offerSdp,
  });
  const body = await res.text();
  if (!res.ok) {
    throw new Error(`mic signaling failed: ${res.status} ${body.slice(0, 120)}`);
  }
  return body;
}

export interface MicPublishHandle {
  pc: RTCPeerConnection;
  stream: MediaStream;
  close: () => void;
  setMuted: (muted: boolean) => void;
}

// publishMic captures the microphone and publishes it (audio-only) to SRS via
// WHIP under the given stream name, authorized by the publish token the
// mic-link API hands the on-air guest. Exactly one sendonly audio m-line is
// offered — SRS only negotiates BUNDLE, so a duplicate transceiver breaks it.
export async function publishMic(
  streamName: string,
  publishToken: string,
): Promise<MicPublishHandle> {
  // getUserMedia only exists in a secure context (https or localhost). If it is
  // missing the browser blocked it for an insecure origin — surface a clear,
  // non-permission error so the UI does not mislead the user.
  if (!navigator.mediaDevices || typeof navigator.mediaDevices.getUserMedia !== 'function') {
    throw new MicSetupError('insecure_context', 'getUserMedia unavailable (insecure context?)');
  }

  let stream: MediaStream;
  try {
    stream = await navigator.mediaDevices.getUserMedia({ audio: true, video: false });
  } catch (err) {
    const name = err instanceof DOMException ? err.name : 'Unknown';
    if (name === 'NotAllowedError' || name === 'SecurityError') {
      throw new MicSetupError('permission_denied', `getUserMedia ${name}`);
    }
    if (name === 'NotFoundError' || name === 'DevicesNotFoundError') {
      throw new MicSetupError('no_device', `getUserMedia ${name}`);
    }
    throw new MicSetupError('getusermedia_failed', `getUserMedia ${name}`);
  }

  const [track] = stream.getAudioTracks();
  if (!track) {
    for (const t of stream.getTracks()) t.stop();
    throw new MicSetupError('no_device', 'no audio track from getUserMedia');
  }

  const pc = new RTCPeerConnection({
    iceServers: [{ urls: 'stun:stun.l.google.com:19302' }],
  });
  pc.addTransceiver(track, { direction: 'sendonly', streams: [stream] });

  try {
    const offer = await pc.createOffer();
    await pc.setLocalDescription(offer);
    const answer = await signal(signalUrl(WHIP_PATH, streamName, publishToken), offer.sdp ?? '');
    await pc.setRemoteDescription({ type: 'answer', sdp: answer });
  } catch (err) {
    for (const t of stream.getTracks()) t.stop();
    pc.close();
    throw new MicSetupError(
      'signaling_failed',
      err instanceof Error ? err.message : 'signaling failed',
    );
  }

  return {
    pc,
    stream,
    setMuted: (muted: boolean) => {
      for (const t of stream.getAudioTracks()) {
        t.enabled = !muted;
      }
    },
    close: () => {
      for (const t of stream.getTracks()) t.stop();
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
  const answer = await signal(signalUrl(WHEP_PATH, streamName), offer.sdp ?? '');
  await pc.setRemoteDescription({ type: 'answer', sdp: answer });

  return {
    pc,
    stream: remote,
    close: () => pc.close(),
  };
}

// micStreamName builds the per-guest stream name used on both ends. The
// `miclink-` prefix tells room-service's SRS on_publish hook to check the
// guest's publish token instead of room bookkeeping. Must match
// miclink.StreamName in golive-backend/pkg/miclink.
export function micStreamName(roomId: string, userId: string): string {
  return `miclink-${roomId}-${userId}`;
}
