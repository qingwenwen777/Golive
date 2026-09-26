import { useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Check,
  ChevronDown,
  Copy,
  Mic,
  MicOff,
  Radio,
  Users,
  X,
} from 'lucide-react';
import { toast } from 'sonner';
import {
  useApproveMicLink,
  useCancelMicLink,
  useConfigMicLink,
  useLatestMicLink,
  useLeaveMicLink,
  useMuteMicLink,
  useRejectMicLink,
  useRemoveMicLink,
  useRequestMicLink,
  type MicLinkErrorReason,
} from '@/api/micLink';
import { useMe } from '@/api/auth';
import { Avatar } from '@/components/Avatar';
import { Dialog, DialogContent, DialogTitle } from '@/components/ui/dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { useAuthModalStore } from '@/stores/useAuthModalStore';
import { useIsAuthed } from '@/stores/useAuthStore';
import { publishMic, micStreamName, MicSetupError, type MicPublishHandle } from '@/lib/micRtc';
import type { MicLinkEligibility, MicLinkGuest } from '@/types/micLink';
import { cn } from '@/lib/cn';

const ELIGIBILITY_OPTIONS: MicLinkEligibility[] = ['all', 'followers', 'fans', 'fans_level'];

export function MicLinkPanel({ roomId, ownsStream }: { roomId: string; ownsStream: boolean }) {
  const latest = useLatestMicLink(roomId, Boolean(roomId));
  const view = latest.data;

  // The panel is hidden entirely for viewers when the feature is off and they
  // are not already involved — keeps the live room uncluttered.
  const myStatus = view?.myStatus ?? 'none';
  const enabled = Boolean(view?.enabled);
  if (!view) return null;
  if (!ownsStream && !enabled && myStatus === 'none') return null;

  return ownsStream ? <MicLinkConsole roomId={roomId} /> : <MicLinkViewer roomId={roomId} />;
}

function PanelShell({ children, count }: { children: React.ReactNode; count?: number }) {
  const { t } = useTranslation('pages');
  return (
    <section className="gl-mic-panel" aria-label={t('micLink.title', { defaultValue: 'Voice mic-link' })}>
      <div className="gl-mic-head">
        <div className="gl-mic-icon" aria-hidden="true">
          <Mic size={17} />
        </div>
        <div>
          <h2>{t('micLink.title', { defaultValue: 'Voice mic-link' })}</h2>
          <p>{t('micLink.subtitle', { defaultValue: 'Invite viewers onto the mic.' })}</p>
        </div>
        {typeof count === 'number' && (
          <span className="gl-mic-count-pill">
            <Users size={13} />
            {t('micLink.slotUsage', {
              used: count,
              max: 3,
              defaultValue: '{{used}}/{{max}}',
            })}
          </span>
        )}
      </div>
      {children}
    </section>
  );
}

// ── Owner console ──────────────────────────────────────────────────────────

function MicLinkConsole({ roomId }: { roomId: string }) {
  const { t } = useTranslation('pages');
  const latest = useLatestMicLink(roomId, true);
  const config = useConfigMicLink(roomId);
  const approve = useApproveMicLink(roomId);
  const reject = useRejectMicLink(roomId);
  const remove = useRemoveMicLink(roomId);

  const view = latest.data;
  const [removeTarget, setRemoveTarget] = useState<MicLinkGuest | null>(null);

  const enabled = Boolean(view?.enabled);
  const eligibility = view?.eligibility ?? 'all';
  const minFanLevel = view?.minFanLevel ?? 1;
  const roster = view?.roster ?? [];
  const requests = view?.requests ?? [];
  const slotMax = view?.slotMax ?? 3;
  const slotFull = roster.length >= slotMax;

  const stageUrl = useMemo(() => `${location.origin}/mic-stage/${encodeURIComponent(roomId)}`, [roomId]);

  const saveConfig = (next: { enabled?: boolean; eligibility?: MicLinkEligibility; minFanLevel?: number }) => {
    config.mutate(
      {
        enabled: next.enabled ?? enabled,
        eligibility: next.eligibility ?? eligibility,
        minFanLevel: next.minFanLevel ?? minFanLevel,
      },
      { onError: (err) => toast.error(micErrorText(err.reason, err.message, t)) },
    );
  };

  const copyStageUrl = async () => {
    try {
      await navigator.clipboard.writeText(stageUrl);
      toast.success(t('micLink.stageUrlCopied', { defaultValue: 'Mic-stage URL copied.' }));
    } catch {
      toast.error(t('micLink.stageUrlCopyFailed', { defaultValue: 'Copy failed. Copy it manually.' }));
    }
  };

  return (
    <PanelShell count={enabled ? roster.length : undefined}>
      <button
        type="button"
        className={cn('gl-mic-toggle', enabled && 'is-on')}
        onClick={() => saveConfig({ enabled: !enabled })}
        disabled={config.isPending}
        aria-pressed={enabled}
      >
        <span>
          <strong>{t('micLink.toggleLabel', { defaultValue: 'Accept mic-link' })}</strong>
          <small>
            {enabled
              ? t('micLink.toggleOnHint', { defaultValue: 'Viewers can request to join.' })
              : t('micLink.toggleOffHint', { defaultValue: 'Mic-link is off for this stream.' })}
          </small>
        </span>
        <i aria-hidden="true" />
      </button>

      {enabled && (
        <>
          <div className="gl-mic-field">
            <span className="gl-mic-field-label">
              {t('micLink.eligibilityLabel', { defaultValue: 'Who can join' })}
            </span>
            <div className="gl-mic-field-controls">
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <button type="button" className="gl-bag-select-trigger">
                    <span>{eligibilityOptionLabel(eligibility, t)}</span>
                    <ChevronDown size={16} aria-hidden="true" />
                  </button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="start" sideOffset={6} className="gl-bag-select-content">
                  {ELIGIBILITY_OPTIONS.map((option) => (
                    <DropdownMenuItem
                      key={option}
                      className={cn('gl-bag-select-item', eligibility === option && 'is-selected')}
                      onSelect={() => saveConfig({ eligibility: option })}
                    >
                      {eligibilityOptionLabel(option, t)}
                    </DropdownMenuItem>
                  ))}
                </DropdownMenuContent>
              </DropdownMenu>
              {eligibility === 'fans_level' && (
                <input
                  type="number"
                  className="gl-mic-level-input"
                  min={1}
                  max={99}
                  step={1}
                  value={minFanLevel}
                  onChange={(event) =>
                    saveConfig({ minFanLevel: Math.max(1, Math.floor(Number(event.target.value) || 1)) })
                  }
                  aria-label={t('micLink.minFanLevelLabel', { defaultValue: 'Min fan level' })}
                />
              )}
            </div>
          </div>

          <div className="gl-mic-stage-url">
            <span>{t('micLink.stageUrlLabel', { defaultValue: 'OBS mic-stage source' })}</span>
            <div className="gl-mic-stage-row">
              <code title={stageUrl}>{stageUrl}</code>
              <button
                type="button"
                onClick={() => void copyStageUrl()}
                aria-label={t('micLink.copy', { defaultValue: 'Copy' })}
              >
                <Copy size={15} />
              </button>
            </div>
            <small>
              {t('micLink.stageUrlHint', {
                defaultValue: 'Add this as a Browser Source in OBS so guest audio joins your stream.',
              })}
            </small>
          </div>

          <div className="gl-mic-section">
            <span className="gl-mic-section-title">
              {t('micLink.queueTitle', { defaultValue: 'Requests' })}
            </span>
            {requests.length > 0 ? (
              <ul className="gl-mic-list">
                {requests.map((req) => (
                  <li key={req.userId} className="gl-mic-row">
                    <Avatar name={req.name} src={req.avatar} size={30} />
                    <span className="gl-mic-row-name" title={req.name}>
                      {req.name}
                    </span>
                    <div className="gl-mic-row-actions">
                      <button
                        type="button"
                        className="gl-mic-approve"
                        disabled={slotFull || approve.isPending}
                        onClick={() =>
                          approve.mutate(
                            { targetId: req.userId },
                            { onError: (err) => toast.error(micErrorText(err.reason, err.message, t)) },
                          )
                        }
                        aria-label={t('micLink.approve', { defaultValue: 'Approve' })}
                      >
                        <Check size={15} />
                      </button>
                      <button
                        type="button"
                        className="gl-mic-reject"
                        disabled={reject.isPending}
                        onClick={() =>
                          reject.mutate(
                            { targetId: req.userId },
                            { onError: (err) => toast.error(micErrorText(err.reason, err.message, t)) },
                          )
                        }
                        aria-label={t('micLink.reject', { defaultValue: 'Reject' })}
                      >
                        <X size={15} />
                      </button>
                    </div>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="gl-mic-empty">
                {slotFull
                  ? t('micLink.queueFull', { defaultValue: 'All mic slots are full.' })
                  : t('micLink.queueEmpty', { defaultValue: 'No pending requests.' })}
              </p>
            )}
          </div>

          <div className="gl-mic-section">
            <span className="gl-mic-section-title">
              {t('micLink.rosterTitle', { defaultValue: 'On the mic' })}
            </span>
            {roster.length > 0 ? (
              <ul className="gl-mic-list">
                {roster.map((guest) => (
                  <li key={guest.userId} className="gl-mic-row">
                    <Avatar name={guest.name} src={guest.avatar} size={30} />
                    <span className="gl-mic-row-name" title={guest.name}>
                      {guest.name}
                    </span>
                    <span className={cn('gl-mic-state', guest.muted ? 'is-muted' : 'is-live')}>
                      {guest.muted ? <MicOff size={13} /> : <Mic size={13} />}
                    </span>
                    <button
                      type="button"
                      className="gl-mic-remove"
                      onClick={() => setRemoveTarget(guest)}
                      aria-label={t('micLink.remove', { defaultValue: 'Remove' })}
                    >
                      <X size={15} />
                    </button>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="gl-mic-empty">
                {t('micLink.rosterEmpty', { defaultValue: 'Nobody is on the mic yet.' })}
              </p>
            )}
          </div>
        </>
      )}

      <Dialog open={Boolean(removeTarget)} onOpenChange={(open) => !open && setRemoveTarget(null)}>
        <DialogContent className="gl-mic-confirm">
          <DialogTitle className="gl-mic-confirm-title">
            {t('micLink.removeConfirmTitle', { defaultValue: 'Remove from mic?' })}
          </DialogTitle>
          <p className="gl-mic-confirm-body">
            {t('micLink.removeConfirmBody', {
              name: removeTarget?.name ?? '',
              defaultValue: '{{name}} will be taken off the mic.',
            })}
          </p>
          <div className="gl-mic-confirm-actions">
            <button type="button" className="is-ghost" onClick={() => setRemoveTarget(null)}>
              {t('micLink.cancel', { defaultValue: 'Cancel' })}
            </button>
            <button
              type="button"
              className="is-danger"
              disabled={remove.isPending}
              onClick={() => {
                if (!removeTarget) return;
                const target = removeTarget.userId;
                setRemoveTarget(null);
                remove.mutate(
                  { targetId: target },
                  { onError: (err) => toast.error(micErrorText(err.reason, err.message, t)) },
                );
              }}
            >
              {t('micLink.remove', { defaultValue: 'Remove' })}
            </button>
          </div>
        </DialogContent>
      </Dialog>
    </PanelShell>
  );
}

// ── Viewer panel ─────────────────────────────────────────────────────────────

function MicLinkViewer({ roomId }: { roomId: string }) {
  const { t } = useTranslation('pages');
  const isAuthed = useIsAuthed();
  const openLogin = useAuthModalStore((s) => s.openLogin);
  const me = useMe();
  const latest = useLatestMicLink(roomId, true);
  const request = useRequestMicLink(roomId);
  const cancel = useCancelMicLink(roomId);
  const leave = useLeaveMicLink(roomId);
  const mute = useMuteMicLink(roomId);

  const view = latest.data;
  const myStatus = view?.myStatus ?? 'none';
  const myMuted = Boolean(view?.myMuted);
  const myPublishToken = view?.myPublishToken;
  const roster = view?.roster ?? [];

  // Guest WebRTC publish lifecycle: start the mic uplink while on air, tear it
  // down on leave/removal. micErrorRef avoids duplicate toasts on re-renders.
  const publishRef = useRef<MicPublishHandle | null>(null);
  const myId = me.data?.id;

  useEffect(() => {
    let cancelled = false;
    const stop = () => {
      publishRef.current?.close();
      publishRef.current = null;
    };
    // The publish token arrives with the on-air view; wait for it.
    if (myStatus === 'on_air' && myId && myPublishToken && !publishRef.current) {
      void publishMic(micStreamName(roomId, myId), myPublishToken)
        .then((handle) => {
          if (cancelled) {
            handle.close();
            return;
          }
          publishRef.current = handle;
          handle.setMuted(myMuted);
        })
        .catch((err: unknown) => {
          if (cancelled) return;
          const reason = err instanceof MicSetupError ? err.reason : 'getusermedia_failed';
          const denied = reason === 'permission_denied';
          const noDevice = reason === 'no_device';
          const insecure = reason === 'insecure_context';
          toast.error(
            denied
              ? t('micLink.micPermissionDenied', {
                  defaultValue: 'Microphone unavailable. Check browser permission.',
                })
              : noDevice
                ? t('micLink.micNoDevice', {
                    defaultValue: 'No microphone found. Connect one and try again.',
                  })
                : insecure
                  ? t('micLink.micInsecure', {
                      defaultValue: 'Mic needs a secure (HTTPS) connection.',
                    })
                  : t('micLink.connectFailed', {
                      defaultValue: 'Could not connect your mic. Please try again.',
                    }),
          );
          // Roll the seat back so the roster does not show a silent guest.
          leave.mutate();
        });
    }
    if (myStatus !== 'on_air') stop();
    return () => {
      cancelled = true;
      if (myStatus !== 'on_air') stop();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [myStatus, myId, myPublishToken, roomId]);

  // Reflect mute state onto the live track.
  useEffect(() => {
    publishRef.current?.setMuted(myMuted);
  }, [myMuted]);

  // Always release the mic on unmount.
  useEffect(() => {
    return () => {
      publishRef.current?.close();
      publishRef.current = null;
    };
  }, []);

  const onRequest = () => {
    if (!isAuthed) {
      openLogin();
      return;
    }
    request.mutate(undefined, {
      onSuccess: () =>
        toast.success(t('micLink.requestSent', { defaultValue: 'Request sent to the streamer.' })),
      onError: (err) => toast.error(micErrorText(err.reason, err.message, t)),
    });
  };

  return (
    <PanelShell count={roster.length}>
      {roster.length > 0 && (
        <ul className="gl-mic-list gl-mic-list-compact">
          {roster.map((guest) => (
            <li key={guest.userId} className="gl-mic-row is-readonly">
              <Avatar name={guest.name} src={guest.avatar} size={28} />
              <span className="gl-mic-row-name" title={guest.name}>
                {guest.name}
              </span>
              <span className={cn('gl-mic-state', guest.muted ? 'is-muted' : 'is-live')}>
                {guest.muted ? <MicOff size={13} /> : <Mic size={13} />}
              </span>
            </li>
          ))}
        </ul>
      )}

      {myStatus === 'none' && (
        <button
          type="button"
          className="gl-mic-request-btn"
          onClick={onRequest}
          disabled={request.isPending}
        >
          <Mic size={16} />
          <span>
            {request.isPending
              ? t('micLink.requesting', { defaultValue: 'Requesting...' })
              : t('micLink.requestButton', { defaultValue: 'Request mic' })}
          </span>
        </button>
      )}

      {myStatus === 'pending' && (
        <div className="gl-mic-self is-pending">
          <Radio size={16} />
          <span>{t('micLink.statusPending', { defaultValue: 'Waiting for approval...' })}</span>
          <button type="button" onClick={() => cancel.mutate()} disabled={cancel.isPending}>
            {t('micLink.cancelRequest', { defaultValue: 'Cancel' })}
          </button>
        </div>
      )}

      {myStatus === 'on_air' && (
        <div className="gl-mic-self is-on-air">
          <span className="gl-mic-self-label">
            <Mic size={16} />
            {t('micLink.statusOnAir', { defaultValue: 'You are on the mic' })}
          </span>
          <div className="gl-mic-self-actions">
            <button
              type="button"
              className={cn('gl-mic-mute-btn', myMuted && 'is-muted')}
              onClick={() => mute.mutate({ muted: !myMuted })}
              disabled={mute.isPending}
            >
              {myMuted ? <MicOff size={15} /> : <Mic size={15} />}
              <span>
                {myMuted
                  ? t('micLink.unmuteSelf', { defaultValue: 'Unmute' })
                  : t('micLink.muteSelf', { defaultValue: 'Mute' })}
              </span>
            </button>
            <button
              type="button"
              className="gl-mic-leave-btn"
              onClick={() => leave.mutate()}
              disabled={leave.isPending}
            >
              {t('micLink.leave', { defaultValue: 'Leave' })}
            </button>
          </div>
        </div>
      )}
    </PanelShell>
  );
}

function eligibilityOptionLabel(
  option: MicLinkEligibility,
  t: ReturnType<typeof useTranslation>['t'],
): string {
  switch (option) {
    case 'followers':
      return t('micLink.eligibility.followers', { defaultValue: 'Followers' });
    case 'fans':
      return t('micLink.eligibility.fans', { defaultValue: 'Fan club members' });
    case 'fans_level':
      return t('micLink.eligibility.fansLevel', { defaultValue: 'Fan club by level' });
    default:
      return t('micLink.eligibility.all', { defaultValue: 'Everyone' });
  }
}

function micErrorText(
  reason: MicLinkErrorReason,
  fallback: string,
  t: ReturnType<typeof useTranslation>['t'],
): string {
  switch (reason) {
    case 'mic_link_disabled':
      return t('micLink.error.disabled', { defaultValue: 'Mic-link is currently off.' });
    case 'mic_link_not_eligible':
      return t('micLink.error.notEligible', { defaultValue: 'You do not meet the join requirement.' });
    case 'mic_link_request_exists':
      return t('micLink.error.requestExists', { defaultValue: 'You already have a request.' });
    case 'mic_link_slot_full':
      return t('micLink.error.slotFull', { defaultValue: 'The mic is full.' });
    case 'mic_link_request_not_found':
      return t('micLink.error.requestNotFound', { defaultValue: 'That request is no longer available.' });
    case 'mic_link_busy':
      return t('micLink.error.busy', { defaultValue: 'Too busy, try again.' });
    case 'forbidden':
      return t('micLink.error.forbidden', { defaultValue: 'Only the streamer can manage this.' });
    default:
      return fallback || t('micLink.error.generic', { defaultValue: 'Action failed.' });
  }
}
