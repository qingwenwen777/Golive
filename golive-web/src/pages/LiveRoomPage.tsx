import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import {
  Bell,
  CalendarClock,
  CheckCircle2,
  CloudOff,
  Copy,
  MessageSquare,
  PlayCircle,
  Radio,
  Square,
  Trophy,
  UserPlus,
  X,
} from 'lucide-react';
import { useQueryClient } from '@tanstack/react-query';
import { Sheet, SheetContent, SheetHeader, SheetTitle } from '@/components/ui/sheet';
import { Player } from '@/features/live-room/Player';
import { Chat } from '@/features/live-room/Chat';
import { InfoBlock } from '@/features/live-room/InfoBlock';
import { GiftPanel } from '@/features/live-room/GiftPanel';
import { SuperChatDialog } from '@/features/live-room/SuperChatDialog';
import { BettingPanel } from '@/features/live-room/BettingPanel';
import { FlyingGiftLayer, type FlyingGift } from '@/features/live-room/FlyingGiftLayer';
import { useRoomRealtime } from '@/features/live-room/useRoomRealtime';
import { useRealtimeStore } from '@/stores/useRealtimeStore';
import { useAuthHydrated, useAuthStore, useIsAuthed } from '@/stores/useAuthStore';
import { useAuthModalStore } from '@/stores/useAuthModalStore';
import { useRoom, useStopLive } from '@/api/room';
import { useLatestBet } from '@/api/bet';
import { fanBadgesQueryKey, useFanBadges } from '@/api/gift';
import {
  useChannelAppointments,
  useReserveAppointment,
  useStartAppointment,
  useUnreserveAppointment,
} from '@/api/room';
import { copyText } from '@/lib/clipboard';
import { addDailyCoinWatchSeconds, markDailyCoinRoomWatched } from '@/lib/coinActivity';
import { WATCH_HISTORY_KEY, markStreamEndedInLibraries, saveToLibrary } from '@/lib/liveLibrary';
import {
  clearPublisherSession,
  loadPublisherSession,
  publisherSessionFromStream,
  savePublisherSession,
  type PublisherSession,
} from '@/features/creator/publisherSession';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import { LoadableImage } from '@/components/LoadableImage';
import type { AppointmentItem } from '@/api/room';
import { userDisplayName } from '@/types/user';
import { streamChannelName, type Stream } from '@/types/stream';
import type { FanBadge } from '@/types/gift';

function fanBadgeLevel(totalContribution: number): number {
  if (totalContribution <= 0) return 1;
  let level = 1;
  let threshold = 1000;
  while (level < 99 && totalContribution >= threshold) {
    level += 1;
    threshold += level * 1000;
  }
  return level;
}

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
  const incrementViewerContribution = useRealtimeStore((s) => s.incrementViewerContribution);
  const stopLive = useStopLive();
  const liveEndedRef = useRef(false);
  const betAnchorRef = useRef<HTMLDivElement | null>(null);
  const [publisherSession, setPublisherSession] = useState<PublisherSession | null>(() =>
    loadPublisherSession(),
  );

  const { data: stream, isPending, isError, refetch } = useRoom(id, authHydrated);
  const isScheduledRoom = stream?.status === 'scheduled';
  const appointmentList = useChannelAppointments(
    stream?.channelId ?? '',
    Boolean(stream?.channelId && isScheduledRoom),
    1,
    20,
  );
  const appointment = useMemo(
    () => appointmentList.data?.items.find((item) => item.id === id) ?? null,
    [appointmentList.data?.items, id],
  );
  const roomId = stream?.id ?? id;
  const latestBet = useLatestBet(roomId, Boolean(roomId) && authHydrated);
  const roomIsLive = Boolean(stream?.isLive === true || stream?.status === 'live');
  const roomIsStarting = Boolean(stream?.status === 'publishing' && !roomIsLive);
  const roomCanWatch = Boolean(stream && stream.status !== 'ended');
  const fanBadges = useFanBadges(isAuthed, currentUser?.id);
  const reserveAppointment = useReserveAppointment(roomId);
  const unreserveAppointment = useUnreserveAppointment(roomId);
  const startAppointment = useStartAppointment(roomId);
  const activeFanBadge = useMemo(() => {
    if (!stream?.ownerId) return null;
    const badge = fanBadges.data?.find((item) => item.creatorId === stream.ownerId);
    if (!badge) return null;
    return { creatorId: badge.creatorId, level: badge.level };
  }, [fanBadges.data, stream?.ownerId]);

  const handleLiveEnded = useCallback(() => {
    if (liveEndedRef.current) return;
    liveEndedRef.current = true;
    clearPublisherSession();
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
    toast.info(t('liveRoom.connection.liveEnded'));
  }, [id, queryClient, stream, t]);

  const { readyState, retryCount, messages, viewers, bullets, viewerCount, sendChat, clearBullet } =
    useRoomRealtime(roomId, roomCanWatch, { onLiveEnded: handleLiveEnded, activeFanBadge });

  useEffect(() => {
    liveEndedRef.current = false;
  }, [id]);

  // Connection state toasts.
  const prevStateRef = useRef(readyState);
  useEffect(() => {
    const connectionToastId = `live-room-connection:${roomId || 'unknown'}`;
    const prev = prevStateRef.current;
    prevStateRef.current = readyState;
    if (!roomId) return;
    if (readyState === prev) return;

    if (readyState === 'connecting' && prev === 'closed') {
      toast.loading(t('liveRoom.connection.connecting'), { id: connectionToastId });
    } else if (readyState === 'open') {
      toast.success(t('liveRoom.connection.connected'), { id: connectionToastId });
      window.setTimeout(() => toast.dismiss(connectionToastId), 1500);
    } else if (readyState === 'reconnecting') {
      toast.loading(t('liveRoom.connection.reconnecting', { count: retryCount }), {
        id: connectionToastId,
      });
    } else if (readyState === 'closed' && prev !== 'closed') {
      toast.error(t('liveRoom.connection.disconnected'), { id: connectionToastId });
    }
  }, [readyState, retryCount, roomId, t]);

  useEffect(() => {
    return () => {
      toast.dismiss(`live-room-connection:${roomId || 'unknown'}`);
    };
  }, [roomId]);

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
    if (!roomIsLive || !roomId || !currentUser?.id) return;
    markDailyCoinRoomWatched(currentUser.id, roomId);
    const timer = window.setInterval(() => {
      addDailyCoinWatchSeconds(currentUser.id, 30);
    }, 30_000);
    return () => window.clearInterval(timer);
  }, [currentUser?.id, roomId, roomIsLive]);

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

  if ((isError && !stream) || !stream) {
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
    readyState === 'closed'
      ? t('liveRoom.connection.disconnected')
      : t('liveRoom.connection.reconnecting', { count: retryCount });

  const ownsStream = Boolean(currentUser?.id && stream.ownerId === currentUser.id);
  const activeBetRound =
    !ownsStream &&
    latestBet.data?.round &&
    latestBet.data.round.status !== 'settled' &&
    latestBet.data.round.status !== 'cancelled'
      ? latestBet.data.round
      : null;
  const scrollToBetPanel = () => {
    betAnchorRef.current?.scrollIntoView({ behavior: 'smooth', block: 'center' });
  };
  const effectivePublisherSession = ownsStream
    ? (publisherSessionFromStream(stream) ??
      (publisherSession?.streamId === stream.id ? publisherSession : null))
    : null;
  const canShowPublisherPanel = ownsStream && effectivePublisherSession;
  const ownerName = streamChannelName(stream, currentUser);
  const openSuperChat = () => {
    if (!isAuthed) {
      openLogin(() => setSuperChatOpen(true));
      return;
    }
    setSuperChatOpen(true);
  };
  const updateLocalFanBadge = (coin: number, createIfMissing: boolean) => {
    if (!currentUser?.id || !stream.ownerId || currentUser.id === stream.ownerId) return;
    const creatorId = stream.ownerId;
    queryClient.setQueryData<FanBadge[]>(fanBadgesQueryKey(currentUser.id), (prev = []) => {
      const now = new Date().toISOString();
      const existing = prev.find((badge) => badge.creatorId === creatorId);
      if (!existing && !createIfMissing) return prev;

      const contribution = Math.max(0, Math.floor(coin));
      if (existing) {
        const totalContribution = existing.totalContribution + contribution;
        return prev.map((badge) =>
          badge.creatorId === creatorId
            ? {
                ...badge,
                creatorName: ownerName,
                creatorAvatar: stream.avatar || badge.creatorAvatar,
                totalContribution,
                level: fanBadgeLevel(totalContribution),
                updatedAt: now,
              }
            : badge,
        );
      }

      return [
        {
          userId: currentUser.id,
          creatorId,
          creatorName: ownerName,
          creatorAvatar: stream.avatar,
          totalContribution: contribution,
          level: fanBadgeLevel(contribution),
          createdAt: now,
          updatedAt: now,
        },
        ...prev,
      ];
    });
  };
  const handleStopLive = () => {
    stopLive.mutate(undefined, {
      onSuccess: () => {
        clearPublisherSession();
        markStreamEndedInLibraries(stream);
        setPublisherSession(null);
        toast.success('Live ended.');
        navigate('/');
      },
      onError: () => toast.error('Could not end the live.'),
    });
  };

  if (!roomIsLive && !isScheduledRoom && !roomIsStarting) {
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

  if (roomIsStarting && stream) {
    return (
      <>
        <div
          className={[
            isNarrow ? 'grid grid-cols-1 gap-6 px-4 pb-20' : 'grid grid-cols-[1fr_402px] gap-6 px-6 pb-20',
            isMobile && mobileComposerFocused ? 'gl-live-room-chatting' : '',
          ]
            .filter(Boolean)
            .join(' ')}
        >
          <div className="min-w-0 flex-1 xl:pt-6">
            <StartingRoomPlayer stream={stream} />
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

          {!isNarrow && (
            <div className="gl-side-rail sticky top-20 self-start">
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
      </>
    );
  }

  if (isScheduledRoom && stream) {
    return (
      <>
        <div
          className={[
            isNarrow ? 'grid grid-cols-1 gap-6 px-4 pb-20' : 'grid grid-cols-[1fr_402px] gap-6 px-6 pb-20',
            isMobile && mobileComposerFocused ? 'gl-live-room-chatting' : '',
          ]
            .filter(Boolean)
            .join(' ')}
        >
          <div className="min-w-0 flex-1 xl:pt-6">
            <ScheduledRoomPlayer
              stream={stream}
              appointment={appointment}
              owner={ownsStream}
              canStart={Boolean(appointment?.canStart)}
              pending={startAppointment.isPending}
              onReserve={() => {
                if (!isAuthed) {
                  openLogin(() => reserveAppointment.mutate(undefined));
                  return;
                }
                reserveAppointment.mutate(undefined, {
                  onSuccess: () => toast.success(t('liveRoom.appointmentReserved', { defaultValue: 'Appointment reserved.' })),
                  onError: (err) => toast.error(err.message || t('liveRoom.appointmentReserveFailed', { defaultValue: 'Could not reserve this appointment.' })),
                });
              }}
              onUnreserve={() => {
                unreserveAppointment.mutate(undefined, {
                  onSuccess: () => toast.success(t('liveRoom.appointmentUnreserved', { defaultValue: 'Reservation removed.' })),
                  onError: (err) => toast.error(err.message || t('liveRoom.appointmentReserveFailed', { defaultValue: 'Could not update reservation.' })),
                });
              }}
              onStart={() => {
                startAppointment.mutate(undefined, {
                  onSuccess: (next) => {
                    savePublisherSession(next);
                    toast.success(t('liveRoom.appointmentStartSuccess', { defaultValue: 'Appointment live started.' }));
                    navigate(`/studio/live/${encodeURIComponent(next.id)}`);
                  },
                  onError: (err) => toast.error(err.message || t('liveRoom.appointmentStartFailed', { defaultValue: 'Could not start this appointment.' })),
                });
              }}
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
            <div ref={betAnchorRef}>
              <BettingPanel roomId={roomId} ownsStream={ownsStream} />
            </div>
            {ownsStream && (
              <div className="gl-owner-live-actions">
                <div>
                  <h2>{t('liveRoom.appointmentOwnerTitle', { defaultValue: 'Appointment management' })}</h2>
                  <p>{t('liveRoom.appointmentOwnerSub', { defaultValue: 'Start the appointment within 30 minutes of the scheduled time.' })}</p>
                </div>
                {appointment?.canStart && (
                  <button
                    type="button"
                    className="gl-owner-end-live"
                    onClick={() => {
                      startAppointment.mutate(undefined, {
                        onSuccess: (next) => {
                          savePublisherSession(next);
                          navigate(`/studio/live/${encodeURIComponent(next.id)}`);
                        },
                        onError: (err) => toast.error(err.message || t('liveRoom.appointmentStartFailed', { defaultValue: 'Could not start this appointment.' })),
                      });
                    }}
                    disabled={startAppointment.isPending}
                  >
                    <PlayCircle size={15} />
                    <span>{startAppointment.isPending ? t('liveRoom.starting', { defaultValue: 'Starting...' }) : t('liveRoom.startLive', { defaultValue: 'Start live' })}</span>
                  </button>
                )}
              </div>
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

          {!isNarrow && (
            <div className="gl-side-rail sticky top-20 self-start">
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
          roomId={roomId}
          onSent={({ gift, count, requestId }) => {
            const totalCoin = gift.priceCoin * count;
            const currentName = currentUser ? userDisplayName(currentUser) : 'You';
            if (currentUser) {
              incrementViewerContribution(
                roomId,
                {
                  userId: currentUser.id,
                  user: currentName,
                  avatar: currentUser.avatar,
                  contribution: 0,
                },
                totalCoin,
              );
            }
            updateLocalFanBadge(totalCoin, gift.id === 'fan_light');
            setFlying((prev) => [
              ...prev,
              {
                id: `fg-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`,
                icon: gift.icon,
                label: `${gift.name} x${count}`,
              },
            ]);
            appendMessage(roomId, {
              id: `gift:${requestId}`,
              kind: 'gift',
              requestId,
              userId: currentUser?.id,
              user: currentName,
              avatar: currentUser?.avatar,
              giftName: gift.name,
              giftIcon: gift.icon,
              count,
              tier: gift.tier,
              totalCoin,
              self: true,
              ts: Date.now(),
            });
          }}
        />

        <SuperChatDialog open={superChatOpen} onOpenChange={setSuperChatOpen} roomId={roomId} />

        <FlyingGiftLayer
          items={flying}
          onDone={(fid) => setFlying((prev) => prev.filter((f) => f.id !== fid))}
        />
      </>
    );
  }

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
      <div ref={betAnchorRef}>
        <BettingPanel roomId={roomId} ownsStream={ownsStream} />
      </div>
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
          {activeBetRound && (
            <BetEntryNotice question={activeBetRound.question} onClick={scrollToBetPanel} />
          )}
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
          <div className="gl-side-rail sticky top-20 self-start">
            {activeBetRound && (
              <BetEntryNotice question={activeBetRound.question} onClick={scrollToBetPanel} />
            )}
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
              {activeBetRound && (
                <BetEntryNotice question={activeBetRound.question} onClick={scrollToBetPanel} />
              )}
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
        roomId={roomId}
        onSent={({ gift, count, requestId }) => {
          const totalCoin = gift.priceCoin * count;
          const currentName = currentUser ? userDisplayName(currentUser) : 'You';
          if (currentUser) {
            incrementViewerContribution(
              roomId,
              {
                userId: currentUser.id,
                user: currentName,
                avatar: currentUser.avatar,
                contribution: 0,
              },
              totalCoin,
            );
          }
          updateLocalFanBadge(totalCoin, gift.id === 'fan_light');
          setFlying((prev) => [
            ...prev,
            {
              id: `fg-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`,
              icon: gift.icon,
              label: `${gift.name} x${count}`,
            },
          ]);
          appendMessage(roomId, {
            id: `gift:${requestId}`,
            kind: 'gift',
            requestId,
            userId: currentUser?.id,
            user: currentName,
            avatar: currentUser?.avatar,
            giftName: gift.name,
            giftIcon: gift.icon,
            count,
            tier: gift.tier,
            totalCoin,
            self: true,
            ts: Date.now(),
          });
        }}
      />

      <SuperChatDialog open={superChatOpen} onOpenChange={setSuperChatOpen} roomId={roomId} />

      <FlyingGiftLayer
        items={flying}
        onDone={(fid) => setFlying((prev) => prev.filter((f) => f.id !== fid))}
      />
    </>
  );
}

function StartingRoomPlayer({ stream }: { stream: Stream }) {
  const { t } = useTranslation('pages');
  return (
    <section className="gl-scheduled-player gl-starting-player">
      <div className="gl-scheduled-player-cover">
        {stream.cover ? <LoadableImage src={stream.cover} alt="" /> : <div className="gl-scheduled-player-fallback" />}
        <div className="gl-scheduled-player-overlay">
          <span className="gl-scheduled-player-badge">
            <Radio size={14} />
            {t('liveRoom.startingBadge', { defaultValue: '主播已开播' })}
          </span>
          <h2>{stream.title}</h2>
          <p>{t('liveRoom.startingHint', { defaultValue: '正在连接直播画面，请留在直播间。' })}</p>
        </div>
      </div>
    </section>
  );
}

function ScheduledRoomPlayer({
  stream,
  appointment,
  owner,
  canStart,
  pending,
  onReserve,
  onUnreserve,
  onStart,
}: {
  stream: Stream;
  appointment?: AppointmentItem | null;
  owner: boolean;
  canStart: boolean;
  pending: boolean;
  onReserve: () => void;
  onUnreserve: () => void;
  onStart: () => void;
}) {
  const { t, i18n } = useTranslation('pages');
  const scheduled = new Intl.DateTimeFormat(i18n.language, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(stream.startedAt));
  const reserved = appointment?.reserved ?? false;
  const reservationCount = appointment?.reservationCount ?? 0;
  return (
    <section className="gl-scheduled-player">
      <div className="gl-scheduled-player-cover">
        {stream.cover ? <LoadableImage src={stream.cover} alt="" /> : <div className="gl-scheduled-player-fallback" />}
        <div className="gl-scheduled-player-overlay">
          <span className="gl-scheduled-player-badge">
            <CalendarClock size={14} />
            {t('liveRoom.scheduledBadge', { defaultValue: 'Appointment' })}
          </span>
          <h2>{stream.title}</h2>
          <p>{scheduled}</p>
          <div className="gl-scheduled-player-meta">
            <span>
              <Bell size={14} />
              {t('liveRoom.scheduledReserved', { count: reservationCount, defaultValue: '{{count}} reserved' })}
            </span>
            <span>
              <CheckCircle2 size={14} />
              {owner
                ? t('liveRoom.scheduledOwner', { defaultValue: 'Owner view' })
                : reserved
                  ? t('liveRoom.scheduledReservedByYou', { defaultValue: 'Reserved by you' })
                  : t('liveRoom.scheduledOpen', { defaultValue: 'Open for reservation' })}
            </span>
          </div>
          <div className="gl-scheduled-player-actions">
            {owner ? (
              <button type="button" className="gl-retry-btn" disabled={pending || !canStart} onClick={onStart}>
                <PlayCircle size={16} />
                {t('liveRoom.startLive', { defaultValue: 'Start live' })}
              </button>
            ) : reserved ? (
              <button type="button" className="gl-secondary-btn" disabled={pending} onClick={onUnreserve}>
                <X size={16} />
                {t('liveRoom.cancelReserve', { defaultValue: 'Cancel reservation' })}
              </button>
            ) : (
              <button type="button" className="gl-retry-btn" disabled={pending} onClick={onReserve}>
                <UserPlus size={16} />
                {t('liveRoom.reserve', { defaultValue: 'Reserve' })}
              </button>
            )}
          </div>
        </div>
      </div>
    </section>
  );
}

function BetEntryNotice({ question, onClick }: { question: string; onClick: () => void }) {
  const { t } = useTranslation('pages');
  return (
    <button type="button" className="gl-bet-entry" onClick={onClick}>
      <span className="gl-bet-entry-icon" aria-hidden="true">
        <Trophy size={16} />
      </span>
      <span className="gl-bet-entry-copy">
        <span>{t('betting.entryActive', { defaultValue: 'Betting is open' })}</span>
        <strong>{question}</strong>
      </span>
      <span className="gl-bet-entry-action">
        {t('betting.entryView', { defaultValue: 'View' })}
      </span>
    </button>
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
