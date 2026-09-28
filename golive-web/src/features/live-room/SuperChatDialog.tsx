import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { toast } from 'sonner';
import { Coins } from 'lucide-react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { tierSpec } from '@/constants/chat';
import { amountToTier, SC_MAX_TEXT_BY_TIER, superChatTextLength } from '@/types/gift';
import { useMe } from '@/api/auth';
import { useSendSuperChat, newRequestId } from '@/api/gift';
import { useAuthStore } from '@/stores/useAuthStore';
import { userDisplayName } from '@/types/user';
import { useRealtimeStore } from '@/stores/useRealtimeStore';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import { cn } from '@/lib/cn';
import { formatNumber } from '@/lib/format';
import { CoinAmount } from '@/components/CoinAmount';
import type { SuperChatMessage } from '@/types/message';

export interface SuperChatDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  roomId: string;
}

const MIN = 200;
const MAX = 50000;
const QUICK_AMOUNTS = [200, 500, 1000, 2000, 5000, 10000];

export function SuperChatDialog({ open, onOpenChange, roomId }: SuperChatDialogProps) {
  const { t, i18n } = useTranslation('pages');
  const isMobile = useMediaQuery('(max-width: 640px)');
  const navigate = useNavigate();
  const me = useMe();
  const balance = me.data?.coinBalance ?? 0;
  const user = useAuthStore((s) => s.user);
  const send = useSendSuperChat();
  const appendMessage = useRealtimeStore((s) => s.appendMessage);
  const replaceMessage = useRealtimeStore((s) => s.replaceMessage);
  const removeMessage = useRealtimeStore((s) => s.removeMessage);

  const [amount, setAmount] = useState(500);
  const [text, setText] = useState('');

  const tier = useMemo(() => amountToTier(amount), [amount]);
  const spec = tierSpec(tier);
  const canText = tier >= 1;
  const maxText = SC_MAX_TEXT_BY_TIER[tier];
  // Lowering the amount can leave the text over the new tier's limit, which
  // the server rejects; it has to be shortened before sending.
  const textTooLong = canText && superChatTextLength(text.trim()) > maxText;
  const insufficient = amount > balance;
  const disabled = send.isPending || amount < MIN || (textTooLong && !insufficient);
  const locale = i18n.resolvedLanguage ?? i18n.language;

  const updateAmount = (value: number) => {
    const next = Math.min(MAX, Math.max(MIN, Math.round(value / 100) * 100));
    setAmount(next);
  };

  const handleSend = () => {
    if (!user) return;
    if (insufficient) {
      onOpenChange(false);
      navigate('/coins?focus=recharge');
      return;
    }
    const requestId = newRequestId();
    const pendingId = `pending:${requestId}`;
    const now = Date.now();
    const payloadText = canText ? text.trim() : '';
    const displayName = userDisplayName(user);
    const avatar = user.avatar;

    const pendingMsg: SuperChatMessage = {
      id: pendingId,
      kind: 'super_chat',
      userId: user.id,
      user: displayName,
      avatar,
      amount: String(amount),
      tier,
      text: payloadText,
      userLevel: user.levelInfo?.level,
      ts: now,
      pending: true,
      requestId,
    };
    appendMessage(roomId, pendingMsg);
    onOpenChange(false);

    send.mutate(
      { roomId, amount, text: payloadText, displayName, requestId },
      {
        onSuccess: (order) => {
          if (order.status !== 'success') {
            removeMessage(roomId, pendingId);
            toast.error(
              t('liveRoom.superChatDialog.insufficientCoins', {
                defaultValue: 'Insufficient coins',
              }),
            );
            return;
          }
          replaceMessage(roomId, pendingId, {
            id: order.orderId,
            kind: 'super_chat',
            userId: user.id,
            user: displayName,
            avatar,
            amount: String(amount),
            tier,
            text: payloadText,
            userLevel: user.levelInfo?.level,
            ts: now,
          });
          setText('');
        },
        onError: (err) => {
          removeMessage(roomId, pendingId);
          if (err.reason === 'insufficient_coin') {
            toast.error(
              t('liveRoom.superChatDialog.insufficientCoins', {
                defaultValue: 'Insufficient coins',
              }),
            );
          } else if (err.reason === 'blocked_word') {
            toast.error(
              t('contentPolicy.blockedWord', {
                defaultValue: 'Content contains blocked words and cannot be sent.',
              }),
            );
          } else if (err.reason === 'user_restricted') {
            toast.error(
              t('contentPolicy.userRestricted', {
                defaultValue: "Your account is restricted, so you can't post or chat right now.",
              }),
            );
          } else if (err.reason === 'super_chat_text_too_long') {
            toast.error(
              t('liveRoom.superChatDialog.textTooLong', {
                count: maxText,
                defaultValue: 'Message is too long for this tier (max {{count}} characters).',
              }),
            );
          } else {
            toast.error(
              err.message ||
                t('liveRoom.superChatDialog.failed', { defaultValue: 'Failed to send SuperChat' }),
            );
          }
        },
      },
    );
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange} modal={false}>
      <DialogContent showOverlay={false} className={cn('gl-sc-dialog', isMobile && 'is-mobile')}>
        <DialogHeader className="gl-sc-dialog-head">
          <DialogTitle>
            {t('liveRoom.superChatDialog.title', { defaultValue: 'Send SuperChat' })}
          </DialogTitle>
          <DialogDescription className="flex items-center gap-1 text-xs">
            <Coins size={12} />
            <span>
              {t('liveRoom.superChatDialog.balance', {
                coins: balance.toLocaleString(locale),
                defaultValue: '{{coins}} coins available',
              })}
            </span>
          </DialogDescription>
        </DialogHeader>

        <div className="gl-sc-preview text-white" style={{ background: spec.bg }}>
          <div className="flex items-center justify-between">
            <span className="font-semibold">{userDisplayName(user)}</span>
            <CoinAmount value={amount} className="font-bold" />
          </div>
          {canText && text.trim() && (
            <div className="gl-sc-preview-message text-black" style={{ background: spec.soft }}>
              {text}
            </div>
          )}
          {!canText && (
            <div className="mt-2 text-xs opacity-80">
              {t('liveRoom.superChatDialog.tierUnlockHint', {
                tier,
                amount: t('account.coins', {
                  ns: 'common',
                  amount: formatNumber(MIN, locale),
                  defaultValue: '{{amount}} coins',
                }),
                defaultValue: 'Tier {{tier}} text messages unlock at {{amount}} or more',
              })}
            </div>
          )}
        </div>

        <div className="gl-sc-amount-block space-y-2">
          <div className="flex items-center justify-between text-sm">
            <span className="text-text-secondary">
              {t('liveRoom.superChatDialog.amount', { defaultValue: 'Amount' })}
            </span>
            <span className="font-medium">
              {t('liveRoom.superChatDialog.tierMaxChars', {
                tier,
                count: maxText || 0,
                defaultValue: 'Tier {{tier}} · max {{count}} chars',
              })}
            </span>
          </div>
          <label className="gl-sc-amount-input">
            <Coins size={16} />
            <input
              inputMode="numeric"
              pattern="[0-9]*"
              value={amount}
              onChange={(e) => {
                const next = Number(e.target.value.replace(/[^\d]/g, ''));
                if (Number.isFinite(next)) setAmount(Math.min(MAX, next || MIN));
              }}
              onBlur={() => updateAmount(amount)}
              aria-label={t('liveRoom.superChatDialog.amountAria', {
                defaultValue: 'SuperChat amount',
              })}
            />
          </label>
          <div className="gl-sc-quick-grid">
            {QUICK_AMOUNTS.map((v) => (
              <button
                key={v}
                onClick={() => updateAmount(v)}
                className={cn('gl-sc-quick-btn', amount === v ? 'is-active' : '')}
              >
                <CoinAmount value={v} iconSize={12} />
              </button>
            ))}
          </div>
        </div>

        <div className="gl-sc-text-block">
          <textarea
            value={text}
            onChange={(e) => {
              if (!canText) return;
              const next = e.target.value;
              // Over the limit, only edits that shorten the text go through.
              if (superChatTextLength(next) > maxText && next.length >= text.length) return;
              setText(next);
            }}
            placeholder={
              canText
                ? t('liveRoom.superChatDialog.textPlaceholder', {
                    defaultValue: 'Say something...',
                  })
                : t('liveRoom.superChatDialog.upgradeTextPlaceholder', {
                    defaultValue: 'Upgrade to tier 1+ to include a message',
                  })
            }
            disabled={!canText}
            rows={3}
            className="gl-sc-textarea"
          />
          {canText && (
            <div
              className={cn(
                'mt-1 text-right text-xs',
                textTooLong ? 'text-red-600 dark:text-red-400' : 'text-text-secondary',
              )}
            >
              {superChatTextLength(text)} / {maxText}
            </div>
          )}
        </div>

        {textTooLong && (
          <div className="text-xs text-red-600 dark:text-red-400">
            {t('liveRoom.superChatDialog.textTooLong', {
              count: maxText,
              defaultValue: 'Message is too long for this tier (max {{count}} characters).',
            })}
          </div>
        )}

        {insufficient && (
          <div className="text-xs text-red-600 dark:text-red-400">
            {t('liveRoom.superChatDialog.insufficientBalance', {
              coins: (amount - balance).toLocaleString(locale),
              defaultValue: 'Insufficient balance (need {{coins}} more).',
            })}
          </div>
        )}

        <div className="gl-sc-actions">
          <button onClick={() => onOpenChange(false)} className="gl-sc-cancel">
            {t('liveRoom.superChatDialog.cancel', { defaultValue: 'Cancel' })}
          </button>
          <button
            disabled={disabled}
            onClick={handleSend}
            className={cn('gl-sc-send', disabled ? 'is-disabled' : 'is-ready')}
          >
            {insufficient
              ? t('liveRoom.superChatDialog.topUp', { defaultValue: 'Top up' })
              : send.isPending
                ? t('liveRoom.superChatDialog.sending', { defaultValue: 'Sending...' })
                : t('liveRoom.superChatDialog.send', { defaultValue: 'Send' })}
          </button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
