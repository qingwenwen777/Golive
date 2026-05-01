import { useCallback, useEffect, useRef, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { CloudOff, Copy, MessageSquare, Radio, Square } from 'lucide-react';
import { useQueryClient } from '@tanstack/react-query';
import { Sheet, SheetContent, SheetHeader, SheetTitle } from '@/components/ui/sheet';
import { Player } from '@/features/live-room/Player';
import { Chat } from '@/features/live-room/Chat';
import { InfoBlock } from '@/features/live-room/InfoBlock';
import { GiftPanel } from '@/features/live-room/GiftPanel';
import { SuperChatDialog } from '@/features/live-room/SuperChatDialog';
import { FlyingGiftLayer, type FlyingGift } from '@/features/live-room/FlyingGiftLayer';
import { useRoomRealtime } from '@/features/live-room/useRoomRealtime';
import { useRealtimeStore } from '@/stores/useRealtimeStore';
import { useAuthHydrated, useAuthStore, useIsAuthed } from '@/stores/useAuthStore';
import { useAuthModalStore } from '@/stores/useAuthModalStore';
import { useRoom, useStopLive } from '@/api/room';
import { copyText } from '@/lib/clipboard';
import { WATCH_HISTORY_KEY, markStreamEndedInLibraries, saveToLibrary } from '@/lib/liveLibrary';
import {
  LIVE_SESSION_STORAGE_KEY,
  loadPublisherSession,
  publisherSessionFromStream,
  savePublisherSession,
  type PublisherSession,
} from '@/features/creator/CreateLiveDialog';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import { userDisplayName } from '@/types/user';
import { streamChannelName } from '@/types/stream';

export default function LiveRoomPage() {
  const { id = '' } = useParams<{ id: string }>();
  const { t } = useTranslation('pages');
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const isNarrow = useMediaQuery('(max-width: 1279px)');
  const isMobile = useMediaQuery('(max-width: 767px)');
  const [sheetOpen, setSheetOpen] = useState(false);
  const [mobileComposerFocused, setMobileComposerFocused] = useState(false);
  const [giftOpen, setGiftOpen] = useState(false);
  const [superChatOpen, setSuperChatOpen] = useState(false);
  const [flying, setFlying] = useState<FlyingGift[]>([]);
  const isAuthed = useIsAuthed();
  const authHydrated = useAuthHydrated();
  const currentUser = useAuthStore((s) => s.user);
  const openLogin = useAuthModalStore((s) => s.openLogin);
  const appendMessage = useRealtimeStore((s) => s.appendMessage);
  const stopLive = useStopLive();
  const liveEndedRef = useRef(false);
  const [publisherSession, setPublisherSession] = useState<PublisherSession | null>(() =>
    loadPublisherSession(),
  );

  const { data: stream, isPending, isError, refetch } = useRoom(id, authHydrated);
  const roomIsLive = Boolean(stream?.isLive === true || stream?.status === 'live');

  const handleLiveEnded = useCallback(() => {
    if (liveEndedRef.current) return;
    liveEndedRef.current = true;
    setPublisherSession(null);

    if (stream) {
      markStreamEndedInLibraries(stream);
    }
    queryClient.setQueryData(['room', id], (prev: typeof stream | undefined) => {
      const base = prev ?? stream;
      if (!base) return base;
      const { playbackUrl: _playbackUrl, streamKey: _streamKey, ...rest } = base;
      return { ...rest, isLive: false, status: 'ended' };
    });
    void queryClient.invalidateQueries({ queryKey: ['rooms'] });
    toast.info('Live has ended.');
  }, [id, queryClient, stream]);

  const { readyState, retryCount, messages, viewers, bullets, viewerCount, sendChat, clearBullet } =
    useRoomRealtime(id, roomIsLive, { onLiveEnded: handleLiveEnded });

  useEffect(() => {
    liveEndedRef.current = false;
  }, [id]);

  // Connection state toasts.
  const prevStateRef = useRef(readyState);
  useEffect(() => {
    const connectionToastId = `live-room-connection:${id || 'unknown'}`;
    const prev = prevStateRef.current;
    prevStateRef.current = readyState;
    if (!id) return;
    if (readyState === prev) return;

    if (readyState === 'connecting' && prev === 'closed') {
      toast.loading('Connecting...', { id: connectionToastId });
    } else if (readyState === 'open') {
      toast.success('Connected', { id: connectionToastId });
      window.setTimeout(() => toast.dismiss(connectionToastId), 1500);
    } else if (readyState === 'reconnecting') {
      toast.loading(`Reconnecting... (#${retryCount})`, { id: connectionToastId });
    } else if (readyState === 'closed' && prev !== 'closed') {
      toast.error('Disconnected', { id: connectionToastId });
    }
  }, [readyState, retryCount, id]);

  useEffect(() => {
    return () => {
      toast.dismiss(`live-room-connection:${id || 'unknown'}`);
    };
  }, [id]);

  useEffect(() => {
    setPublisherSession(loadPublisherSession());
  }, [id]);

  useEffect(() => {
    if (!stream?.streamKey || !currentUser?.id || stream.ownerId !== currentUser.id) return;
    savePublisherSession(stream);
    setPublisherSession(publisherSessionFromStream(stream));
  }, [currentUser?.id, stream]);

  useEffect(() => {
    if (!stream || !roomIsLive) return;
    saveToLibrary(WATCH_HISTORY_KEY, stream, 'watchedAt');
  }, [roomIsLive, stream]);

  useEffect(() => {
    if (!stream) return;
    const timer = window.setInterval(
      () => {
        void refetch();
      },
      roomIsLive ? 5000 : 3000,
    );
    return () => window.clearInterval(timer);
  }, [refetch, roomIsLive, stream]);

  if (isPending) {
    return (
      <div className="grid grid-cols-1 gap-6 px-6 pb-20 xl:grid-cols-[1fr_402px]">
        <div>
          <div className="aspect-video animate-pulse rounded-card bg-bg-hover" />
          <div className="mt-4 h-6 w-2/3 animate-pulse rounded bg-bg-hover" />
          <div className="mt-2 h-4 w-1/3 animate-pulse rounded bg-bg-hover" />
        </div>
        <div className="hidden xl:block">
          <div className="h-[620px] animate-pulse rounded-card bg-bg-hover" />
        </div>
      </div>
    );
  }

  if (isError || !stream) {
    return (
      <div className="gl-empty">
        <CloudOff size={64} strokeWidth={1.5} />
        <div className="gl-empty-title">{t('liveRoom.ended')}</div>
        <button className="gl-retry-btn mt-4" onClick={() => navigate('/')}>
          {t('notFound.back')}
        </button>
      </div>
    );
  }

  const effectiveViewers = viewerCount > 0 ? viewerCount : stream.viewers;
  const reconnecting = readyState === 'reconnecting' || readyState === 'closed';
  const reconnectingLabel =
    readyState === 'closed' ? 'Disconnected' : `Reconnecting… (#${retryCount})`;

  const ownsStream = Boolean(currentUser?.id && stream.ownerId === currentUser.id);
  const effectivePublisherSession = ownsStream
    ? (publisherSessionFromStream(stream) ??
      (publisherSession?.streamId === stream.id ? publisherSession : null))
    : null;
  const canShowPublisherPanel = ownsStream && effectivePublisherSession;
  const ownerName = streamChannelName(stream, currentUser);
  const handleStopLive = () => {
    stopLive.mutate(undefined, {
      onSuccess: () => {
        localStorage.removeItem(LIVE_SESSION_STORAGE_KEY);
        markStreamEndedInLibraries(stream);
        setPublisherSession(null);
        toast.success('Live ended.');
        navigate('/');
      },
      onError: () => toast.error('Could not end the live.'),
    });
  };

  if (!roomIsLive) {
    if (!canShowPublisherPanel) {
      return (
        <div className="gl-empty">
          <CloudOff size={64} strokeWidth={1.5} />
          <div className="gl-empty-title">{t('liveRoom.ended')}</div>
          <button className="gl-retry-btn mt-4" onClick={() => navigate('/')}>
            {t('notFound.back')}
          </button>
        </div>
      );
    }

    return (
      <div className="gl-page max-w-3xl">
        <div className="gl-owner-live-actions">
          <div>
            <h2>Waiting for publisher</h2>
            <p>Add the stream key in OBS. The room becomes watchable after SRS confirms publish.</p>
          </div>
          <button
            type="button"
            className="gl-owner-end-live"
            onClick={handleStopLive}
            disabled={stopLive.isPending}
          >
            <Square size={15} />
            <span>{stopLive.isPending ? 'Ending...' : 'End live'}</span>
          </button>
        </div>
        <PublisherPanel
          session={canShowPublisherPanel}
          playbackUrl={stream.playbackUrl}
          stopping={stopLive.isPending}
          onStop={handleStopLive}
        />
      </div>
    );
  }

  const openSuperChat = () => {
    if (!isAuthed) {
      openLogin(() => setSuperChatOpen(true));
      return;
    }
    setSuperChatOpen(true);
  };

  const Left = (
    <div className="min-w-0 flex-1 xl:pt-6">
      <Player
        stream={stream}
        viewerCount={effectiveViewers}
        bullets={bullets}
        onBulletEnd={clearBullet}
      />
      <InfoBlock
        stream={stream}
        viewerCount={effectiveViewers}
        onOpenGifts={() => {
          if (!isAuthed) {
            openLogin(() => setGiftOpen(true));
            return;
          }
          setGiftOpen(true);
        }}
      />
      {ownsStream && (
        <div className="gl-owner-live-actions">
          <div>
            <h2>Live controls</h2>
            <p>Stop the current room when your stream is finished.</p>
          </div>
          <button
            type="button"
            className="gl-owner-end-live"
            onClick={handleStopLive}
            disabled={stopLive.isPending}
          >
            <Square size={15} />
            <span>{stopLive.isPending ? 'Ending...' : 'End live'}</span>
          </button>
        </div>
      )}
      {canShowPublisherPanel && (
        <PublisherPanel
          session={canShowPublisherPanel}
          playbackUrl={stream.playbackUrl}
          stopping={stopLive.isPending}
          onStop={handleStopLive}
        />
      )}
      {isMobile && (
        <div className="gl-mobile-chat">
          <Chat
            messages={messages}
            viewers={viewers}
            viewerTotal={effectiveViewers}
            ownerId={stream.ownerId}
            ownerName={ownerName}
            onSendChat={sendChat}
            onSendSuperChat={openSuperChat}
            reconnecting={reconnecting}
            reconnectingLabel={reconnectingLabel}
            onComposerFocusChange={setMobileComposerFocused}
          />
        </div>
      )}
    </div>
  );

  return (
    <>
      <div
        className={[
          isNarrow
            ? 'grid grid-cols-1 gap-6 px-4 pb-20'
            : 'grid grid-cols-[1fr_402px] gap-6 px-6 pb-20',
          isMobile && mobileComposerFocused ? 'gl-live-room-chatting' : '',
        ]
          .filter(Boolean)
          .join(' ')}
      >
        {Left}
        {!isNarrow && (
          <div className="sticky top-20 self-start">
            <Chat
              messages={messages}
              viewers={viewers}
              viewerTotal={effectiveViewers}
              ownerId={stream.ownerId}
              ownerName={ownerName}
              onSendChat={sendChat}
              onSendSuperChat={openSuperChat}
              reconnecting={reconnecting}
              reconnectingLabel={reconnectingLabel}
            />
          </div>
        )}
      </div>

      {isNarrow && !isMobile && (
        <>
          <button
            className="gl-chat-fab"
            aria-label={t('liveRoom.openChat')}
            onClick={() => setSheetOpen(true)}
          >
            <MessageSquare size={24} />
          </button>
          <Sheet open={sheetOpen} onOpenChange={setSheetOpen}>
            <SheetContent side="right" className="w-[402px] max-w-full p-0 sm:max-w-[402px]">
              <SheetHeader className="sr-only">
                <SheetTitle>{t('liveRoom.chat')}</SheetTitle>
              </SheetHeader>
              <Chat
                messages={messages}
                viewers={viewers}
                viewerTotal={effectiveViewers}
                ownerId={stream.ownerId}
                ownerName={ownerName}
                onSendChat={sendChat}
                onSendSuperChat={openSuperChat}
                reconnecting={reconnecting}
                reconnectingLabel={reconnectingLabel}
                sheetMode
              />
            </SheetContent>
          </Sheet>
        </>
      )}

      <GiftPanel
        open={giftOpen}
        onOpenChange={setGiftOpen}
        roomId={id}
        onSent={({ gift, count, requestId }) => {
          setFlying((prev) => [
            ...prev,
            {
              id: `fg-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`,
              icon: gift.icon,
              label: `${gift.name} ×${count}`,
            },
          ]);
          appendMessage(id, {
            id: `gift:${requestId}`,
            kind: 'gift',
            requestId,
            userId: currentUser?.id,
            user: currentUser ? userDisplayName(currentUser) : 'You',
            avatar: currentUser?.avatar,
            giftName: gift.name,
            giftIcon: gift.icon,
            count,
            tier: gift.tier,
            totalCoin: gift.priceCoin * count,
            self: true,
            ts: Date.now(),
          });
        }}
      />

      <SuperChatDialog open={superChatOpen} onOpenChange={setSuperChatOpen} roomId={id} />

      <FlyingGiftLayer
        items={flying}
        onDone={(fid) => setFlying((prev) => prev.filter((f) => f.id !== fid))}
      />
    </>
  );
}

function PublisherPanel({
  session,
  playbackUrl,
  stopping,
  onStop,
}: {
  session: PublisherSession;
  playbackUrl?: string;
  stopping: boolean;
  onStop: () => void;
}) {
  const streamUrl = playbackUrl || session.playbackUrl || '';

  const copy = async (value: string, label: string) => {
    try {
      const method = await copyText(value, label);
      if (method === 'manual') {
        toast.info(`${label} opened for manual copy.`);
      } else {
        toast.success(`${label} copied.`);
      }
    } catch {
      toast.error(`Could not copy ${label.toLowerCase()}.`);
    }
  };

  return (
    <section className="gl-publisher-panel" aria-label="Publisher setup">
      <div className="gl-publisher-head">
        <div className="gl-publisher-icon" aria-hidden="true">
          <Radio size={18} />
        </div>
        <div>
          <h2>Publisher setup</h2>
          <p>Use these values in OBS, then start streaming.</p>
        </div>
      </div>

      <PublisherValue
        label="OBS server"
        value={session.rtmpServer}
        onCopy={() => copy(session.rtmpServer, 'OBS server')}
      />
      <PublisherValue
        label="Stream key"
        value={session.streamKey}
        secret
        onCopy={() => copy(session.streamKey, 'Stream key')}
      />
      {streamUrl && (
        <PublisherValue
          label="Playback URL"
          value={streamUrl}
          onCopy={() => copy(streamUrl, 'Playback URL')}
        />
      )}

      <button type="button" className="gl-publisher-stop" onClick={onStop} disabled={stopping}>
        <Square size={15} />
        <span>{stopping ? 'Ending...' : 'End live'}</span>
      </button>
    </section>
  );
}

function PublisherValue({
  label,
  value,
  secret,
  onCopy,
}: {
  label: string;
  value: string;
  secret?: boolean;
  onCopy: () => void;
}) {
  return (
    <div className="gl-publisher-value">
      <span>{label}</span>
      <code>{secret ? value.replace(/.(?=.{6})/g, '*') : value}</code>
      <button type="button" onClick={onCopy} aria-label={`Copy ${label}`}>
        <Copy size={15} />
      </button>
    </div>
  );
}
