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
  Type,
} from 'lucide-react';
import { cn } from '@/lib/cn';
import { type DanmuFontSize, useDanmuStore } from '@/stores/useDanmuStore';
import { usePlayerPreferenceStore } from '@/stores/usePlayerPreferenceStore';
import { DanmuLayer } from './DanmuLayer';
import { playerShortcutFor } from './playerShortcuts';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import type { Stream } from '@/types/stream';
import type { Bullet } from '@/stores/useRealtimeStore';
import { formatNumber } from '@/lib/format';

const LIVE_STASH_INITIAL_SIZE = 384 * 1024;
const LIVE_RECOVERY_DELAY_MS = 2500;
const LIVE_RECONNECT_DELAY_MS = 1800;
const LIVE_STUCK_RELOAD_MS = 7000;
const LIVE_BUFFER_CHECK_MS = 2000;
const LIVE_RECOVERY_BACKOFF_SECONDS = 1;
const LIVE_MAX_BUFFER_LATENCY_SECONDS = 8;
const LIVE_TARGET_LATENCY_SECONDS = 2;

const DANMU_FONT_OPTIONS: Array<{ value: DanmuFontSize; label: string }> = [
  { value: 'sm', label: 'Small' },
  { value: 'md', label: 'Medium' },
  { value: 'lg', label: 'Large' },
];

type WebKitVideoElement = HTMLVideoElement & {
  webkitDisplayingFullscreen?: boolean;
  webkitEnterFullscreen?: () => void;
  webkitExitFullscreen?: () => void;
};

type ScreenWakeLockSentinel = EventTarget & {
  released: boolean;
  release: () => Promise<void>;
};

type ScreenWakeLockNavigator = Navigator & {
  wakeLock?: {
    request: (type: 'screen') => Promise<ScreenWakeLockSentinel>;
  };
};

function bufferedRangeAtLiveEdge(video: HTMLVideoElement): { start: number; end: number } | null {
  const { buffered } = video;
  if (buffered.length === 0) return null;
  const last = buffered.length - 1;
  return { start: buffered.start(last), end: buffered.end(last) };
}

function seekNearLiveEdge(video: HTMLVideoElement, secondsBehindLive: number): boolean {
  const range = bufferedRangeAtLiveEdge(video);
  if (!range) return false;
  const nextTime = Math.max(range.start, range.end - secondsBehindLive);
  if (!Number.isFinite(nextTime) || Math.abs(video.currentTime - nextTime) < 0.25) return false;
  video.currentTime = nextTime;
  return true;
}

function bufferedAhead(video: HTMLVideoElement): number {
  const range = bufferedRangeAtLiveEdge(video);
  if (!range) return 0;
  return Math.max(0, range.end - video.currentTime);
}

export interface PlayerProps {
  stream: Stream;
  videoSrc?: string;
  extraViewers?: number;
  viewerCount?: number;
  bullets?: Bullet[];
  onBulletEnd?: (id: string) => void;
  liveEnding?: boolean;
}

// The playback name comes only from playbackUrl: the stream's play name,
// which is not the room id. The owner's streamKey is the OBS publish
// credential ("<playName>?key=<secret>"), not a playable stream name.
function streamPlaybackKey(stream: Stream): string {
  const source = stream.playbackUrl || '';
  return (
    source.match(/\/live\/([^/?#]+)\.flv(?:[?#].*)?$/)?.[1] ||
    source.match(/\/([^/?#]+)\.flv(?:[?#].*)?$/)?.[1] ||
    ''
  );
}

function buildFlvUrl(stream: Stream): string {
  const flvBase = ((import.meta.env.VITE_FLV_BASE as string | undefined) ?? '').replace(/\/$/, '');
  const source = stream.playbackUrl || '';
  const key = streamPlaybackKey(stream);

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
  liveEnding = false,
}: PlayerProps) {
  const { t } = useTranslation('pages');
  const containerRef = useRef<HTMLDivElement | null>(null);
  const videoRef = useRef<HTMLVideoElement | null>(null);

  const [playing, setPlaying] = useState(true);
  const muted = usePlayerPreferenceStore((s) => s.muted);
  const volume = usePlayerPreferenceStore((s) => s.volume);
  const setPlayerAudio = usePlayerPreferenceStore((s) => s.setAudio);
  const [fullscreen, setFullscreen] = useState(false);
  const [pip, setPip] = useState(false);
  const [controlsVisible, setControlsVisible] = useState(true);
  const hideControlsTimerRef = useRef<number | null>(null);
  const stallTimerRef = useRef<number | null>(null);
  const reconnectTimerRef = useRef<number | null>(null);
  const wakeLockRef = useRef<ScreenWakeLockSentinel | null>(null);
  const lastPlaybackRef = useRef({ time: 0, changedAt: Date.now() });

  const danmuOn = useDanmuStore((s) => s.on);
  const toggleDanmu = useDanmuStore((s) => s.toggle);
  const danmuFontSize = useDanmuStore((s) => s.fontSize);
  const setDanmuFontSize = useDanmuStore((s) => s.setFontSize);

  const flvUrl = buildFlvUrl(stream);
  const isLiveFlv = !!flvUrl && stream.isLive !== false;
  // Nothing to play: say so over the cover rather than playing a demo video.
  const noSignal = !isLiveFlv && !videoSrc && !liveEnding;

  const [flvError, setFlvError] = useState<string | null>(null);
  const [buffering, setBuffering] = useState(false);
  const [retryNonce, setRetryNonce] = useState(0);
  const endedRef = useRef(false);
  const showPlaybackEnded = useCallback(() => {
    if (endedRef.current || liveEnding) return;
    endedRef.current = true;
    setBuffering(false);
    setFlvError(t('player.streamEnded', { defaultValue: 'Stream ended.' }));
  }, [liveEnding, t]);

  const showControlsTemporarily = useCallback(() => {
    setControlsVisible(true);
    if (hideControlsTimerRef.current) {
      window.clearTimeout(hideControlsTimerRef.current);
    }
    hideControlsTimerRef.current = window.setTimeout(() => {
      setControlsVisible(false);
      hideControlsTimerRef.current = null;
    }, 2600);
  }, []);

  const clearStallTimer = useCallback(() => {
    if (!stallTimerRef.current) return;
    window.clearTimeout(stallTimerRef.current);
    stallTimerRef.current = null;
  }, []);

  const clearReconnectTimer = useCallback(() => {
    if (!reconnectTimerRef.current) return;
    window.clearTimeout(reconnectTimerRef.current);
    reconnectTimerRef.current = null;
  }, []);

  const scheduleLiveReconnect = useCallback(() => {
    if (!isLiveFlv || stream.isLive === false || endedRef.current || liveEnding) return false;
    setFlvError(null);
    setBuffering(true);
    showControlsTemporarily();

    clearReconnectTimer();
    reconnectTimerRef.current = window.setTimeout(() => {
      reconnectTimerRef.current = null;
      setRetryNonce((n) => n + 1);
    }, LIVE_RECONNECT_DELAY_MS);
    return true;
  }, [clearReconnectTimer, isLiveFlv, liveEnding, showControlsTemporarily, stream.isLive]);

  const recoverLivePlayback = useCallback(
    (secondsBehindLive = LIVE_RECOVERY_BACKOFF_SECONDS) => {
      const v = videoRef.current;
      if (!v || !isLiveFlv || endedRef.current || liveEnding) return;
      seekNearLiveEdge(v, secondsBehindLive);
      void v.play()?.catch?.(() => undefined);
    },
    [isLiveFlv, liveEnding],
  );

  useEffect(() => {
    endedRef.current = false;
    setBuffering(false);
    clearStallTimer();
    clearReconnectTimer();
  }, [clearReconnectTimer, clearStallTimer, flvUrl]);

  useEffect(() => {
    return () => {
      clearStallTimer();
      clearReconnectTimer();
    };
  }, [clearReconnectTimer, clearStallTimer]);

  useEffect(() => {
    if (!liveEnding) return;
    endedRef.current = true;
    setBuffering(false);
    setFlvError(null);
    clearStallTimer();
    clearReconnectTimer();
  }, [clearReconnectTimer, clearStallTimer, liveEnding]);

  useEffect(() => {
    const v = videoRef.current;
    if (!v) return;
    if (Math.abs(v.volume - volume) > 0.001) v.volume = volume;
    if (v.muted !== muted) v.muted = muted;
  }, [flvUrl, muted, volume]);

  useEffect(() => {
    if (!isLiveFlv || !flvUrl || !videoRef.current) return;
    if (!mpegts.isSupported()) {
      setFlvError('Your browser does not support HTTP-FLV playback.');
      return;
    }
    setFlvError(null);

    const player = mpegts.createPlayer(
      { type: 'flv', url: flvUrl, isLive: true, hasAudio: true, hasVideo: true },
      {
        isLive: true,
        enableWorker: false,
        enableStashBuffer: true,
        stashInitialSize: LIVE_STASH_INITIAL_SIZE,
        liveBufferLatencyChasing: true,
        liveBufferLatencyMaxLatency: LIVE_MAX_BUFFER_LATENCY_SECONDS,
        liveBufferLatencyMinRemain: LIVE_TARGET_LATENCY_SECONDS,
        lazyLoad: false,
        autoCleanupSourceBuffer: true,
        autoCleanupMaxBackwardDuration: 20,
        autoCleanupMinBackwardDuration: 8,
      },
    );
    const onErr = (errType: string, errDetail: string) => {
      if (liveEnding) {
        setBuffering(false);
        setFlvError(null);
        return;
      }
      const detail = `${errType} ${errDetail}`.toLowerCase();
      if (
        detail.includes('eof') ||
        detail.includes('ended') ||
        detail.includes('loading_complete') ||
        detail.includes('network') ||
        detail.includes('exception')
      ) {
        if (scheduleLiveReconnect()) return;
      }
      if (stream.isLive === false) {
        showPlaybackEnded();
        return;
      }
      setFlvError(`Stream error: ${errType}${errDetail ? ' / ' + errDetail : ''}`);
    };
    const onEnd = () => {
      if (liveEnding) {
        setBuffering(false);
        return;
      }
      if (!scheduleLiveReconnect()) showPlaybackEnded();
    };
    player.on(mpegts.Events.ERROR, onErr);
    player.on(mpegts.Events.LOADING_COMPLETE, onEnd);

    player.attachMediaElement(videoRef.current);
    try {
      player.load();
      void player.play()?.catch?.(() => {
        setPlaying(false);
        showControlsTemporarily();
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
      setBuffering(false);
      clearStallTimer();
      clearReconnectTimer();
    };
  }, [
    clearReconnectTimer,
    clearStallTimer,
    isLiveFlv,
    flvUrl,
    liveEnding,
    retryNonce,
    scheduleLiveReconnect,
    showPlaybackEnded,
    showControlsTemporarily,
    stream.isLive,
  ]);

  const togglePlay = useCallback(() => {
    const v = videoRef.current;
    if (!v || liveEnding) return;
    if (v.paused) void v.play();
    else v.pause();
  }, [liveEnding]);

  const toggleMute = useCallback(() => {
    const v = videoRef.current;
    if (!v) return;
    const nextMuted = !v.muted;
    v.muted = nextMuted;
    setPlayerAudio({ muted: nextMuted, volume: v.volume });
  }, [setPlayerAudio]);

  const changeVolume = useCallback(
    (value: number) => {
      const v = videoRef.current;
      if (!v) return;
      const clamped = Math.min(1, Math.max(0, value));
      v.volume = clamped;
      if (clamped > 0 && v.muted) v.muted = false;
      setPlayerAudio({ muted: v.muted, volume: clamped });
    },
    [setPlayerAudio],
  );

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
    const video = videoRef.current as WebKitVideoElement | null;
    const active = document.fullscreenElement ?? doc.webkitFullscreenElement ?? null;
    if (active) {
      (document.exitFullscreen?.() ?? doc.webkitExitFullscreen?.())?.catch(() => undefined);
      return;
    }

    if (video?.webkitDisplayingFullscreen && video.webkitExitFullscreen) {
      video.webkitExitFullscreen();
      return;
    }

    const request = node.requestFullscreen?.() ?? node.webkitRequestFullscreen?.();
    if (request) {
      request.catch(() => {
        video?.webkitEnterFullscreen?.();
      });
      return;
    }

    video?.webkitEnterFullscreen?.();
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
      if (liveEnding) return;
      if (isLiveFlv) scheduleLiveReconnect();
    };
    const onVol = () => {
      setPlayerAudio({ muted: v.muted, volume: v.volume });
    };
    const onEnterPip = () => setPip(true);
    const onLeavePip = () => setPip(false);
    const onWebkitBeginFullscreen = () => setFullscreen(true);
    const onWebkitEndFullscreen = () => setFullscreen(false);
    v.addEventListener('play', onPlay);
    v.addEventListener('pause', onPause);
    v.addEventListener('ended', onEnded);
    v.addEventListener('volumechange', onVol);
    v.addEventListener('enterpictureinpicture', onEnterPip);
    v.addEventListener('leavepictureinpicture', onLeavePip);
    v.addEventListener('webkitbeginfullscreen', onWebkitBeginFullscreen);
    v.addEventListener('webkitendfullscreen', onWebkitEndFullscreen);
    return () => {
      v.removeEventListener('play', onPlay);
      v.removeEventListener('pause', onPause);
      v.removeEventListener('ended', onEnded);
      v.removeEventListener('volumechange', onVol);
      v.removeEventListener('enterpictureinpicture', onEnterPip);
      v.removeEventListener('leavepictureinpicture', onLeavePip);
      v.removeEventListener('webkitbeginfullscreen', onWebkitBeginFullscreen);
      v.removeEventListener('webkitendfullscreen', onWebkitEndFullscreen);
    };
  }, [isLiveFlv, liveEnding, scheduleLiveReconnect, setPlayerAudio]);

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
      const player = containerRef.current;
      if (!player || !player.isConnected) return;
      const action = playerShortcutFor(e, player);
      if (!action) return;
      e.preventDefault();
      switch (action) {
        case 'togglePlay':
          togglePlay();
          break;
        case 'fullscreen':
          toggleFullscreen();
          break;
        case 'mute':
          toggleMute();
          break;
        case 'danmu':
          toggleDanmu();
          break;
        case 'volumeUp':
          changeVolume(volume + 0.05);
          break;
        case 'volumeDown':
          changeVolume(volume - 0.05);
          break;
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [togglePlay, toggleFullscreen, toggleMute, toggleDanmu, changeVolume, volume]);

  const jumpToLive = () => {
    const v = videoRef.current;
    if (!v || liveEnding) return;
    try {
      if (isLiveFlv) {
        seekNearLiveEdge(v, LIVE_RECOVERY_BACKOFF_SECONDS);
      } else {
        v.currentTime = v.duration || 0;
      }
      void v.play();
    } catch {
      /* noop */
    }
  };

  useEffect(() => {
    showControlsTemporarily();
    return () => {
      if (hideControlsTimerRef.current) {
        window.clearTimeout(hideControlsTimerRef.current);
        hideControlsTimerRef.current = null;
      }
    };
  }, [showControlsTemporarily]);

  useEffect(() => {
    const v = videoRef.current;
    if (!v || !isLiveFlv || liveEnding) return;

    const onBuffering = () => {
      if (endedRef.current || liveEnding) return;
      setBuffering(true);
      showControlsTemporarily();
      clearStallTimer();
      stallTimerRef.current = window.setTimeout(() => {
        stallTimerRef.current = null;
        recoverLivePlayback();
      }, LIVE_RECOVERY_DELAY_MS);
    };
    const onReady = () => {
      setBuffering(false);
      clearStallTimer();
    };
    const onTimeUpdate = () => {
      if (!v.paused && v.readyState >= 2) onReady();
    };

    v.addEventListener('waiting', onBuffering);
    v.addEventListener('stalled', onBuffering);
    v.addEventListener('playing', onReady);
    v.addEventListener('canplay', onReady);
    v.addEventListener('timeupdate', onTimeUpdate);
    return () => {
      v.removeEventListener('waiting', onBuffering);
      v.removeEventListener('stalled', onBuffering);
      v.removeEventListener('playing', onReady);
      v.removeEventListener('canplay', onReady);
      v.removeEventListener('timeupdate', onTimeUpdate);
      clearStallTimer();
    };
  }, [clearStallTimer, isLiveFlv, liveEnding, recoverLivePlayback, showControlsTemporarily]);

  useEffect(() => {
    if (!isLiveFlv || flvError || liveEnding) return;
    const v = videoRef.current;
    if (!v) return;
    lastPlaybackRef.current = { time: v.currentTime, changedAt: Date.now() };

    const timer = window.setInterval(() => {
      const video = videoRef.current;
      if (!video || video.paused || endedRef.current || liveEnding) return;

      const now = Date.now();
      const moved = Math.abs(video.currentTime - lastPlaybackRef.current.time) > 0.08;
      if (moved) {
        lastPlaybackRef.current = { time: video.currentTime, changedAt: now };
        if (bufferedAhead(video) > LIVE_MAX_BUFFER_LATENCY_SECONDS) {
          seekNearLiveEdge(video, LIVE_TARGET_LATENCY_SECONDS);
        }
        return;
      }

      if (now - lastPlaybackRef.current.changedAt <= LIVE_STUCK_RELOAD_MS) return;
      if (bufferedAhead(video) > 0.5) {
        recoverLivePlayback();
      } else {
        setRetryNonce((n) => n + 1);
      }
      lastPlaybackRef.current = { time: video.currentTime, changedAt: now };
    }, LIVE_BUFFER_CHECK_MS);

    return () => window.clearInterval(timer);
  }, [flvError, isLiveFlv, liveEnding, recoverLivePlayback]);

  useEffect(() => {
    if (!isLiveFlv || liveEnding) return;

    let cancelled = false;
    const releaseWakeLock = () => {
      const lock = wakeLockRef.current;
      wakeLockRef.current = null;
      if (lock && !lock.released) {
        void lock.release().catch(() => undefined);
      }
    };
    const requestWakeLock = async () => {
      if (cancelled || wakeLockRef.current || document.visibilityState !== 'visible') return;
      const wakeLock = (navigator as ScreenWakeLockNavigator).wakeLock;
      if (!wakeLock?.request) return;
      try {
        const lock = await wakeLock.request('screen');
        if (cancelled) {
          await lock.release().catch(() => undefined);
          return;
        }
        wakeLockRef.current = lock;
        lock.addEventListener('release', () => {
          if (wakeLockRef.current === lock) wakeLockRef.current = null;
        });
      } catch {
        // Unsupported browsers simply fall back to normal system behavior.
      }
    };
    const onVisibilityChange = () => {
      if (document.visibilityState === 'visible') {
        void requestWakeLock();
      } else {
        releaseWakeLock();
      }
    };

    void requestWakeLock();
    document.addEventListener('visibilitychange', onVisibilityChange);
    return () => {
      cancelled = true;
      document.removeEventListener('visibilitychange', onVisibilityChange);
      releaseWakeLock();
    };
  }, [isLiveFlv, liveEnding]);

  const viewers = viewerCount ?? stream.viewers + extraViewers;
  const menuPortalContainer = fullscreen ? containerRef.current : undefined;

  return (
    <div
      ref={containerRef}
      role="region"
      aria-label={t('player.region')}
      tabIndex={0}
      className={cn(
        'gl-player group',
        fullscreen && 'is-fullscreen',
        fullscreen && !controlsVisible && 'is-idle',
        liveEnding && 'is-ending',
      )}
      onMouseEnter={showControlsTemporarily}
      onMouseMove={showControlsTemporarily}
      onPointerDown={showControlsTemporarily}
      onTouchStart={showControlsTemporarily}
      onFocusCapture={showControlsTemporarily}
    >
      <video
        ref={videoRef}
        className="gl-video"
        src={isLiveFlv ? undefined : videoSrc}
        poster={noSignal ? stream.cover || undefined : undefined}
        autoPlay
        muted={muted}
        playsInline
        preload="auto"
        loop={!isLiveFlv}
        controls={false}
      />

      <div className="gl-player-top">
        <span className="gl-player-title">{stream.title}</span>
        <span className="gl-player-viewers">
          <span className="gl-live-dot-red" aria-hidden="true" />
          {formatNumber(viewers)}
        </span>
      </div>

      {noSignal && (
        <div className="gl-player-offline" role="status">
          <strong>
            {t('player.offlineTitle', { defaultValue: 'Not broadcasting right now' })}
          </strong>
          <span>
            {t('player.offlineBody', {
              defaultValue: 'The video starts here as soon as the creator’s stream comes in.',
            })}
          </span>
        </div>
      )}

      {isLiveFlv && flvError && !liveEnding && (
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

      {isLiveFlv && buffering && !flvError && !liveEnding && (
        <div className="gl-player-buffering" role="status" aria-live="polite">
          <span>
            <i aria-hidden="true" />
            {t('player.buffering', { defaultValue: 'Buffering...' })}
          </span>
        </div>
      )}

      <DanmuLayer bullets={bullets} onBulletEnd={onBulletEnd} />

      {liveEnding && (
        <div className="gl-player-ending" role="status" aria-live="polite">
          <div className="gl-player-ending-card">
            <span className="gl-player-ending-badge">
              <span className="gl-live-dot-red" aria-hidden="true" />
              {t('player.endingBadge', { defaultValue: 'Ended' })}
            </span>
            <strong>{t('player.endingTitle', { defaultValue: 'Live ended' })}</strong>
            <span>
              {t('player.endingSubtitle', {
                defaultValue: 'Thanks for watching. Preparing the room...',
              })}
            </span>
            <i aria-hidden="true" />
          </div>
        </div>
      )}

      <div
        className={cn('gl-player-ctl', controlsVisible && 'is-on')}
        hidden={noSignal}
        onPointerDown={showControlsTemporarily}
      >
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
          <DropdownMenu
            onOpenChange={(open) => {
              if (open) showControlsTemporarily();
            }}
          >
            <DropdownMenuTrigger asChild>
              <button
                className="gl-pbtn"
                aria-label={t('player.danmuFontSize', { defaultValue: 'Danmu size' })}
                title={t('player.danmuFontSize', { defaultValue: 'Danmu size' })}
              >
                <Type size={20} />
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent
              align="end"
              side="top"
              className="w-36"
              container={menuPortalContainer}
            >
              <DropdownMenuLabel>
                {t('player.danmuFontSize', { defaultValue: 'Danmu size' })}
              </DropdownMenuLabel>
              <DropdownMenuRadioGroup
                value={danmuFontSize}
                onValueChange={(value) => setDanmuFontSize(value as DanmuFontSize)}
              >
                {DANMU_FONT_OPTIONS.map((option) => (
                  <DropdownMenuRadioItem key={option.value} value={option.value}>
                    {t(`player.danmuFontSizeOptions.${option.value}`, {
                      defaultValue: option.label,
                    })}
                  </DropdownMenuRadioItem>
                ))}
              </DropdownMenuRadioGroup>
            </DropdownMenuContent>
          </DropdownMenu>
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
