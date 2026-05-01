import { useMemo, useState } from 'react';
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
import { amountToTier, SC_MAX_TEXT_BY_TIER } from '@/types/gift';
import { useMe } from '@/api/auth';
import { useSendSuperChat, newRequestId } from '@/api/gift';
import { useAuthStore } from '@/stores/useAuthStore';
import { userDisplayName } from '@/types/user';
import { useRealtimeStore } from '@/stores/useRealtimeStore';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import { cn } from '@/lib/cn';
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
  const isMobile = useMediaQuery('(max-width: 640px)');
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
  const insufficient = amount > balance;
  const disabled = send.isPending || insufficient || amount < MIN;

  const updateAmount = (value: number) => {
    const next = Math.min(MAX, Math.max(MIN, Math.round(value / 100) * 100));
    setAmount(next);
  };

  const handleSend = () => {
    if (!user) return;
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
            toast.error('Insufficient coins');
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
            ts: now,
          });
          setText('');
        },
        onError: (err) => {
          removeMessage(roomId, pendingId);
          if (err.reason === 'insufficient_coin') toast.error('Insufficient coins');
          else toast.error(err.message || 'Failed to send SuperChat');
        },
      },
    );
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className={cn('gl-sc-dialog max-w-md', isMobile && 'is-mobile')}>
        <DialogHeader className="gl-sc-dialog-head">
          <DialogTitle>Send SuperChat</DialogTitle>
          <DialogDescription className="flex items-center gap-1 text-xs">
            <Coins size={12} /> <span>{balance.toLocaleString()} coins available</span>
          </DialogDescription>
        </DialogHeader>

        <div className="gl-sc-preview rounded-md p-3 text-white" style={{ background: spec.bg }}>
          <div className="flex items-center justify-between">
            <span className="font-semibold">{userDisplayName(user)}</span>
            <span className="font-bold">¥{amount.toLocaleString()}</span>
          </div>
          {canText && text.trim() && (
            <div
              className="mt-2 rounded-sm p-2 text-sm text-black"
              style={{ background: spec.soft }}
            >
              {text}
            </div>
          )}
          {!canText && (
            <div className="mt-2 text-xs opacity-80">
              Tier {tier} · text messages unlock at ¥200+
            </div>
          )}
        </div>

        <div className="gl-sc-amount-block space-y-2">
          <div className="flex items-center justify-between text-sm">
            <span className="text-text-secondary">Amount</span>
            <span className="font-medium">
              tier {tier} · max {maxText || 0} chars
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
              aria-label="SuperChat amount"
            />
          </label>
          <input
            type="range"
            min={MIN}
            max={MAX}
            step={100}
            value={amount}
            onChange={(e) => setAmount(Number(e.target.value))}
            className="w-full accent-accent"
          />
          <div className="gl-sc-quick-grid flex flex-wrap gap-1.5">
            {QUICK_AMOUNTS.map((v) => (
              <button
                key={v}
                onClick={() => updateAmount(v)}
                className={cn(
                  'gl-sc-quick-btn rounded-full px-2.5 py-1 text-xs',
                  amount === v ? 'bg-accent text-white' : 'bg-bg-hover text-text-secondary',
                )}
              >
                ¥{v.toLocaleString()}
              </button>
            ))}
          </div>
        </div>

        <div className="gl-sc-text-block">
          <textarea
            value={text}
            onChange={(e) => {
              if (!canText) return;
              if (e.target.value.length > maxText) return;
              setText(e.target.value);
            }}
            placeholder={canText ? 'Say something…' : 'Upgrade to tier 1+ to include a message'}
            disabled={!canText}
            rows={3}
            className="gl-sc-textarea w-full resize-none rounded-md border border-border bg-bg-primary p-2 text-sm outline-none focus:border-accent disabled:opacity-50"
          />
          {canText && (
            <div className="mt-1 text-right text-xs text-text-secondary">
              {text.length} / {maxText}
            </div>
          )}
        </div>

        {insufficient && (
          <div className="text-red-500 text-xs">
            Insufficient balance (need {(amount - balance).toLocaleString()} more).
          </div>
        )}

        <div className="gl-sc-actions flex justify-end gap-2">
          <button
            onClick={() => onOpenChange(false)}
            className="gl-sc-cancel rounded-full px-4 py-2 text-sm text-text-secondary hover:bg-bg-hover"
          >
            Cancel
          </button>
          <button
            disabled={disabled}
            onClick={handleSend}
            className={cn(
              'gl-sc-send rounded-full px-5 py-2 text-sm font-semibold text-white transition',
              disabled ? 'bg-accent/40' : 'bg-accent hover:bg-accent/90',
            )}
          >
            {send.isPending ? 'Sending…' : 'Send'}
          </button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
