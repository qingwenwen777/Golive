import { useEffect, useRef, useState } from 'react';
import { useParams } from 'react-router-dom';
import { http } from '@/lib/axios';
import { playMic, micStreamName, type MicPlayHandle } from '@/lib/micRtc';
import type { MicLinkView } from '@/types/micLink';

// MicStagePage is the headless audio stage the streamer adds as an OBS Browser
// Source. It polls the on-air roster and keeps a WHEP audio subscription per
// guest, playing their voice through hidden <audio> elements. OBS captures
// that audio and mixes it into the outgoing RTMP, so all viewers hear guests
// through the normal FLV/HLS playback — no viewer-side change needed.
//
// It deliberately has no chrome: a transparent page with a tiny status badge
// (visible to the streamer in OBS while configuring, harmless on the stream).
export default function MicStagePage() {
  const { roomId = '' } = useParams();
  const [guests, setGuests] = useState<{ userId: string; name: string }[]>([]);
  const handlesRef = useRef<Map<string, MicPlayHandle>>(new Map());
  const audioRef = useRef<Map<string, HTMLAudioElement>>(new Map());

  // Poll the roster every few seconds.
  useEffect(() => {
    if (!roomId) return;
    let active = true;
    const poll = async () => {
      try {
        const { data } = await http.get<MicLinkView>('/mic-link/latest', { params: { roomId } });
        if (!active) return;
        setGuests(data.roster.map((g) => ({ userId: g.userId, name: g.name })));
      } catch {
        /* transient; keep last roster */
      }
    };
    void poll();
    const timer = window.setInterval(poll, 3_000);
    return () => {
      active = false;
      window.clearInterval(timer);
    };
  }, [roomId]);

  // Reconcile WHEP subscriptions against the current roster.
  useEffect(() => {
    const handles = handlesRef.current;
    const wanted = new Set(guests.map((g) => g.userId));

    // Drop guests who left.
    for (const [userId, handle] of handles) {
      if (!wanted.has(userId)) {
        handle.close();
        handles.delete(userId);
        const el = audioRef.current.get(userId);
        if (el) {
          el.srcObject = null;
          audioRef.current.delete(userId);
        }
      }
    }

    // Add new guests.
    for (const guest of guests) {
      if (handles.has(guest.userId)) continue;
      void playMic(micStreamName(roomId, guest.userId))
        .then((handle) => {
          handles.set(guest.userId, handle);
          const el = audioRef.current.get(guest.userId);
          if (el) {
            el.srcObject = handle.stream;
            void el.play().catch(() => undefined);
          }
        })
        .catch(() => undefined);
    }
  }, [guests, roomId]);

  useEffect(() => {
    const handles = handlesRef.current;
    return () => {
      for (const handle of handles.values()) handle.close();
      handles.clear();
    };
  }, []);

  return (
    <div className="gl-mic-stage">
      <div className="gl-mic-stage-badge">
        <span className="gl-mic-stage-dot" />
        Mic-link stage · {guests.length} on air
      </div>
      {guests.map((guest) => (
        <audio
          key={guest.userId}
          autoPlay
          ref={(el) => {
            if (!el) return;
            audioRef.current.set(guest.userId, el);
            // If the WHEP handle resolved before this element mounted, attach
            // its stream now so playback starts.
            const handle = handlesRef.current.get(guest.userId);
            if (handle && el.srcObject !== handle.stream) {
              el.srcObject = handle.stream;
              void el.play().catch(() => undefined);
            }
          }}
        />
      ))}
    </div>
  );
}
