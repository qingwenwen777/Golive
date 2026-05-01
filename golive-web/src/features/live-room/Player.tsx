import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import mpegts from 'mpegts.js';
import {
  Play,
  Pause,
  Volume2,
  VolumeX,
  Maximize,
  Minimize,
  PictureInPicture,
  MessagesSquare,
  MessageSquareOff,
} from 'lucide-react';
import { cn } from '@/lib/cn';
import { useDanmuStore } from '@/stores/useDanmuStore';
import { DanmuLayer } from './DanmuLayer';
import type { Stream } from '@/types/stream';
import type { Bullet } from '@/stores/useRealtimeStore';

const DEFAULT_VIDEO_SRC =
  'https://commondatastorage.googleapis.com/gtv-videos-bucket/sample/BigBuckBunny.mp4';

export interface PlayerProps {
  stream: Stream;
  videoSrc?: string;
  extraViewers?: number;
  viewerCount?: number;
  bullets?: Bullet[];
  onBulletEnd?: (id: string) => void;
}

function buildFlvUrl(stream: Stream): string {
  const flvBase = ((import.meta.env.VITE_FLV_BASE as string | undefined) ?? '').replace(/\/$/, '');
  const source = stream.playbackUrl || '';
  const key =
    stream.streamKey ||
    source.match(/\/live\/([^/?#]+)\.flv(?:[?#].*)?$/)?.[1] ||
    source.match(/\/([^/?#]+)\.flv(?:[?#].*)?$/)?.[1] ||
    '';

  if (flvBase && key) {
    return `${flvBase}/${key}.flv`;
  }
  if (source) return source;
  if (key) return `/live/${key}.flv`;
  return '';
}

export function Player({
  stream,
  videoSrc,
  extraViewers = 0,
  viewerCount,
  bullets = [],
  onBulletEnd,
}: PlayerProps) {
  const { t } = useTranslation('pages');
  const containerRef = useRef<HTMLDivElement | null>(null);
  const videoRef = useRef<HTMLVideoElement | null>(null);

  const [playing, setPlaying] = useState(true);
  const [muted, setMuted] = useState(true);
  const [volume, setVolume] = useState(1);
  const [fullscreen, setFullscreen] = useState(false);
  const [pip, setPip] = useState(false);

  const danmuOn = useDanmuStore((s) => s.on);
  const toggleDanmu = useDanmuStore((s) => s.toggle);

  const flvUrl = buildFlvUrl(stream);
  const isLiveFlv = !!flvUrl && stream.isLive !== false;
  const fallbackSrc = videoSrc ?? DEFAULT_VIDEO_SRC;

  const [flvError, setFlvError] = useState<string | null>(null);
  const [retryNonce, setRetryNonce] = useState(0);
  const endedRef = useRef(false);
  const showPlaybackEnded = useCallback(() => {
    if (endedRef.current) return;
    endedRef.current = true;
    setFlvError('Stream ended.');
  }, []);

  useEffect(() => {
    endedRef.current = false;
  }, [flvUrl]);

  useEffect(() => {
    if (!isLiveFlv || !flvUrl || !videoRef.current) return;
    if (!mpegts.isSupported()) {
      setFlvError('Your browser does not support HTTP-FLV playback.');
      return;
    }
    setFlvError(null);

    const player = mpegts.createPlayer(
      { type: 'flv', url: flvUrl, isLive: true, hasAudio: true, hasVideo: true },
      { isLive: true, enableStashBuffer: false, liveBufferLatencyChasing: true },
    );
    const onErr = (errType: string, errDetail: string) => {
      const detail = `${errType} ${errDetail}`.toLowerCase();
      if (detail.includes('eof') || detail.includes('ended') || detail.includes('loading_complete')) {
        showPlaybackEnded();
        return;
      }
      setFlvError(`Stream error: ${errType}${errDetail ? ' / ' + errDetail : ''}`);
    };
    const onEnd = () => showPlaybackEnded();
    player.on(mpegts.Events.ERROR, onErr);
    player.on(mpegts.Events.LOADING_COMPLETE, onEnd);

    player.attachMediaElement(videoRef.current);
    try {
      player.load();
      void player.play()?.catch?.(() => {
        // Autoplay blocked — surface as recoverable, user can click play.
      });
    } catch (e) {
      setFlvError(e instanceof Error ? e.message : 'Failed to start stream.');
    }

    return () => {
      try {
        player.off(mpegts.Events.ERROR, onErr);
        player.off(mpegts.Events.LOADING_COMPLETE, onEnd);
      } catch {
        // ignore
      }
      try {
        player.pause();
      } catch {
        /* noop */
      }
      try {
        player.unload();
        player.detachMediaElement();
      } catch {
        /* noop */
      }
      try {
        player.destroy();
      } catch {
        /* noop */
      }
    };
  }, [isLiveFlv, flvUrl, retryNonce, showPlaybackEnded]);

  const togglePlay = useCallback(() => {
    const v = videoRef.current;
    if (!v) return;
    if (v.paused) void v.play();
    else v.pause();
  }, []);

  const toggleMute = useCallback(() => {
    const v = videoRef.current;
    if (!v) return;
    v.muted = !v.muted;
  }, []);

  const changeVolume = useCallback((value: number) => {
    const v = videoRef.current;
    if (!v) return;
    const clamped = Math.min(1, Math.max(0, value));
    v.volume = clamped;
    if (clamped > 0 && v.muted) v.muted = false;
  }, []);

  const toggleFullscreen = useCallback(() => {
    const el = containerRef.current;
    if (!el) return;
    const doc = document as Document & {
      webkitFullscreenElement?: Element | null;
      webkitExitFullscreen?: () => Promise<void>;
    };
    const node = el as HTMLDivElement & {
      webkitRequestFullscreen?: () => Promise<void>;
    };
    const active = document.fullscreenElement ?? doc.webkitFullscreenElement ?? null;
    if (!active) {
      (node.requestFullscreen?.() ?? node.webkitRequestFullscreen?.())?.catch(() => undefined);
    } else {
      (document.exitFullscreen?.() ?? doc.webkitExitFullscreen?.())?.catch(() => undefined);
    }
  }, []);

  const togglePip = useCallback(() => {
    const v = videoRef.current;
    if (!v) return;
    const doc = document as Document & { pictureInPictureElement?: Element | null };
    if (doc.pictureInPictureElement) {
      void document.exitPictureInPicture();
    } else if (typeof v.requestPictureInPicture === 'function') {
      void v.requestPictureInPicture().catch(() => undefined);
    }
  }, []);

  useEffect(() => {
    const v = videoRef.current;
    if (!v) return;
    const onPlay = () => setPlaying(true);
    const onPause = () => setPlaying(false);
    const onEnded = () => {
      if (isLiveFlv) showPlaybackEnded();
    };
    const onVol = () => {
      setMuted(v.muted);
      setVolume(v.volume);
    };
    const onEnterPip = () => setPip(true);
    const onLeavePip = () => setPip(false);
    v.addEventListener('play', onPlay);
    v.addEventListener('pause', onPause);
    v.addEventListener('ended', onEnded);
    v.addEventListener('volumechange', onVol);
    v.addEventListener('enterpictureinpicture', onEnterPip);
    v.addEventListener('leavepictureinpicture', onLeavePip);
    return () => {
      v.removeEventListener('play', onPlay);
      v.removeEventListener('pause', onPause);
      v.removeEventListener('ended', onEnded);
      v.removeEventListener('volumechange', onVol);
      v.removeEventListener('enterpictureinpicture', onEnterPip);
      v.removeEventListener('leavepictureinpicture', onLeavePip);
    };
  }, [isLiveFlv, showPlaybackEnded]);

  useEffect(() => {
    const onFsChange = () => {
      const doc = document as Document & { webkitFullscreenElement?: Element | null };
      setFullscreen(Boolean(document.fullscreenElement ?? doc.webkitFullscreenElement));
    };
    document.addEventListener('fullscreenchange', onFsChange);
    document.addEventListener('webkitfullscreenchange', onFsChange);
    return () => {
      document.removeEventListener('fullscreenchange', onFsChange);
      document.removeEventListener('webkitfullscreenchange', onFsChange);
    };
  }, []);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const active = document.activeElement;
      if (active && (active.tagName === 'INPUT' || active.tagName === 'TEXTAREA')) return;
      const inRoom = containerRef.current && containerRef.current.isConnected;
      if (!inRoom) return;

      switch (e.key) {
        case ' ':
        case 'Spacebar':
          e.preventDefault();
          togglePlay();
          break;
        case 'f':
        case 'F':
          e.preventDefault();
          toggleFullscreen();
          break;
        case 'm':
        case 'M':
          e.preventDefault();
          toggleMute();
          break;
        case 'd':
        case 'D':
          e.preventDefault();
          toggleDanmu();
          break;
        case 'ArrowUp':
          e.preventDefault();
          changeVolume(volume + 0.05);
          break;
        case 'ArrowDown':
          e.preventDefault();
          changeVolume(volume - 0.05);
          break;
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [togglePlay, toggleFullscreen, toggleMute, toggleDanmu, changeVolume, volume]);

  const jumpToLive = () => {
    const v = videoRef.current;
    if (!v) return;
    try {
      v.currentTime = v.duration || 0;
      void v.play();
    } catch {
      /* noop */
    }
  };

  const viewers = viewerCount ?? stream.viewers + extraViewers;

  return (
    <div ref={containerRef} className="gl-player group">
      <video
        ref={videoRef}
        className="gl-video"
        src={isLiveFlv ? undefined : fallbackSrc}
        autoPlay
        muted={muted}
        playsInline
        loop={!isLiveFlv}
        controls={false}
      />

      <div className="gl-player-top">
        <span className="gl-player-title">{stream.title}</span>
        <span className="gl-player-viewers">
          <span className="gl-live-dot-red" aria-hidden="true" />
          {viewers.toLocaleString()}
        </span>
      </div>

      {isLiveFlv && flvError && (
        <div
          role="alert"
          className="absolute inset-0 z-10 flex flex-col items-center justify-center gap-3 bg-black/80 text-center text-sm text-white"
        >
          <div className="px-6">{flvError}</div>
          <button
            type="button"
            onClick={() => {
              setFlvError(null);
              setRetryNonce((n) => n + 1);
            }}
            className="rounded-full bg-white/10 px-4 py-1.5 text-xs font-semibold hover:bg-white/20"
          >
            {t('player.retry', { defaultValue: 'Retry' })}
          </button>
        </div>
      )}

      <DanmuLayer bullets={bullets} onBulletEnd={onBulletEnd} />

      <div className={cn('gl-player-ctl', 'is-on')}>
        <div className="gl-progress" aria-label={t('player.liveProgress')}>
          <div className="gl-progress-fill" style={{ width: '100%' }} />
        </div>
        <div className="gl-ctl-row">
          <button
            className="gl-pbtn"
            onClick={togglePlay}
            aria-label={playing ? t('player.pause') : t('player.play')}
          >
            {playing ? <Pause size={20} /> : <Play size={20} />}
          </button>
          <div className="gl-vol-wrap">
            <button
              className="gl-pbtn"
              onClick={toggleMute}
              aria-label={muted ? t('player.unmute') : t('player.mute')}
              aria-pressed={muted}
            >
              {muted || volume === 0 ? <VolumeX size={20} /> : <Volume2 size={20} />}
            </button>
            <div className="gl-vol">
              <input
                type="range"
                min={0}
                max={1}
                step={0.05}
                value={muted ? 0 : volume}
                onChange={(e) => changeVolume(Number(e.target.value))}
                aria-label={t('player.volume')}
              />
            </div>
          </div>
          <button className="gl-ctl-live" onClick={jumpToLive} aria-label={t('player.jumpLive')}>
            <span className="gl-live-dot-red" aria-hidden="true" />
            <span className="gl-live-word">{t('player.live')}</span>
          </button>
          <div className="gl-ctl-spacer" />
          <button
            className="gl-pbtn"
            onClick={toggleDanmu}
            aria-label={t('player.toggleDanmu')}
            aria-pressed={danmuOn}
            title={danmuOn ? t('player.danmuOn') : t('player.danmuOff')}
          >
            {danmuOn ? <MessagesSquare size={20} /> : <MessageSquareOff size={20} />}
          </button>
          <button
            className="gl-pbtn"
            onClick={togglePip}
            aria-label={t('player.pip')}
            aria-pressed={pip}
          >
            <PictureInPicture size={20} />
          </button>
          <button
            className="gl-pbtn"
            onClick={toggleFullscreen}
            aria-label={fullscreen ? t('player.exitFullscreen') : t('player.fullscreen')}
            aria-pressed={fullscreen}
          >
            {fullscreen ? <Minimize size={20} /> : <Maximize size={20} />}
          </button>
        </div>
      </div>
    </div>
  );
}
