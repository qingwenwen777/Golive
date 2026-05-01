import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { ChevronDown, MoreVertical, X, Smile, CircleDollarSign, Send, Gift } from 'lucide-react';
import { Avatar } from '@/components/Avatar';
import { tierSpec } from '@/constants/chat';
import type {
  Message,
  ChatMessage,
  SuperChatMessage,
  SystemMessage,
  GiftMessage,
} from '@/types/message';
import { cn } from '@/lib/cn';
import { useIsAuthed, useAuthStore } from '@/stores/useAuthStore';
import { userDisplayName } from '@/types/user';
import { useAuthModalStore } from '@/stores/useAuthModalStore';

export interface ChatProps {
  messages: Message[];
  onSendSuperChat?: () => void;
  sheetMode?: boolean;
  onClose?: () => void;
  reconnecting?: boolean;
  reconnectingLabel?: string;
  onSendChat?: (text: string) => boolean | void;
}

const EMOJI_GROUPS = [
  {
    id: 'faces',
    icon: '😀',
    label: 'Faces',
    items: ['😀', '😄', '😂', '😊', '😍', '🥳', '😎', '🤔', '😭', '😡', '😴', '🤯'],
  },
  {
    id: 'gestures',
    icon: '👍',
    label: 'Gestures',
    items: ['👍', '👎', '👏', '🙌', '🙏', '🤝', '💪', '👀', '✌️', '🤘', '👌', '🫶'],
  },
  {
    id: 'stream',
    icon: '❤️',
    label: 'Stream',
    items: ['❤️', '🔥', '✨', '🎉', '💯', '⭐', '🌟', '⚡', '🎵', '🎮', '🏆', '💎'],
  },
] as const;

const MAX_CHAT_CHARS = 200;

const GIFT_META: Record<string, { icon: string; tier: 0 | 1 | 2 | 3 }> = {
  flower: { icon: '\u{1f33c}', tier: 0 },
  donut: { icon: '\u{1f369}', tier: 0 },
  cake: { icon: '\u{1f370}', tier: 0 },
  ramen: { icon: '\u{1f35c}', tier: 0 },
  rocket: { icon: '\u{1f680}', tier: 1 },
  crown: { icon: '\u{1f451}', tier: 2 },
  gem: { icon: '\u{1f48e}', tier: 2 },
  yacht: { icon: '\u{1f6e5}\ufe0f', tier: 3 },
  castle: { icon: '\u{1f3f0}', tier: 3 },
};

function giftMeta(name: string): { icon?: string; tier: 0 | 1 | 2 | 3 } {
  const fallback = GIFT_META[name.trim().toLowerCase()];
  return {
    icon: fallback?.icon,
    tier: fallback?.tier ?? 0,
  };
}

function charCount(s: string): number {
  return Array.from(s).length;
}

function hashHue(name: string): number {
  let x = 0;
  for (let i = 0; i < name.length; i++) x = (x * 31 + name.charCodeAt(i)) & 0xffff;
  return x % 360;
}

function userColor(name: string): string {
  return `hsl(${hashHue(name)}, 55%, 48%)`;
}

function ChatRow({ m }: { m: ChatMessage }) {
  return (
    <div className="gl-chat-line">
      <Avatar name={m.user} src={m.avatar} size={24} />
      <div className="gl-chat-body">
        <span className="gl-chat-user" style={{ color: m.color ?? userColor(m.user) }}>
          {m.user}
        </span>
        <span className="gl-chat-text">{m.text}</span>
      </div>
    </div>
  );
}

function SuperChatCard({ m }: { m: SuperChatMessage }) {
  const spec = tierSpec(m.tier);
  return (
    <div
      className={cn('gl-sc', m.pending && 'opacity-50 saturate-50')}
      style={{ background: spec.bg }}
      aria-busy={m.pending ? 'true' : undefined}
    >
      <div className="gl-sc-head">
        <Avatar name={m.user} src={m.avatar} size={24} />
        <span className="gl-sc-user">{m.user}</span>
        <span className="gl-sc-amt">{m.amount}</span>
      </div>
      {m.text && (
        <div className="gl-sc-body" style={{ background: spec.soft, color: '#0f0f0f' }}>
          {m.text}
        </div>
      )}
    </div>
  );
}

function SystemNotice({ m }: { m: SystemMessage }) {
  return (
    <div className="gl-chat-notice">
      <div className="gl-chat-notice-body">{m.text}</div>
    </div>
  );
}

function GiftNotice({ m }: { m: GiftMessage }) {
  const count = m.count ?? 1;
  const meta = giftMeta(m.giftName);
  // Treat empty string as missing so server broadcasts that omit the icon
  // (or send "") still render the same colored card sender sees locally.
  const icon = m.giftIcon && m.giftIcon.length > 0 ? m.giftIcon : meta.icon;
  const tier = (m.tier ?? meta.tier) as 0 | 1 | 2 | 3;
  return (
    <div className={cn('gl-gift-notice', `tier-${tier}`)}>
      <div className="gl-gift-notice-icon" aria-hidden="true">
        {icon ? <span>{icon}</span> : <Gift size={18} />}
      </div>
      <div className="gl-gift-notice-body">
        <div className="gl-gift-notice-title">
          <span className="gl-gift-notice-user">{m.self ? 'You' : m.user}</span>
          <span>sent</span>
        </div>
        <div className="gl-gift-notice-meta">
          <span className="gl-gift-notice-name">{m.giftName}</span>
          <span className="gl-gift-notice-count">x{count}</span>
        </div>
      </div>
    </div>
  );
}

export function Chat({
  messages,
  onSendSuperChat,
  sheetMode,
  onClose,
  reconnecting,
  reconnectingLabel,
  onSendChat,
}: ChatProps) {
  const { t } = useTranslation('pages');
  const listRef = useRef<HTMLDivElement | null>(null);
  const inputRef = useRef<HTMLInputElement | null>(null);
  const emojiWrapRef = useRef<HTMLDivElement | null>(null);
  const inputValueRef = useRef('');
  const [input, setInput] = useState('');
  const [emojiOpen, setEmojiOpen] = useState(false);
  const [emojiGroup, setEmojiGroup] = useState<(typeof EMOJI_GROUPS)[number]['id']>('faces');
  // Track IME composition so Enter during candidate selection (CJK input
  // methods) does not submit a half-finished message.
  const composingRef = useRef(false);
  const isAuthed = useIsAuthed();
  const currentUser = useAuthStore((s) => s.user);
  const openLogin = useAuthModalStore((s) => s.openLogin);

  const trySend = () => {
    const text = inputValueRef.current.trim();
    if (!text) return;
    if (!isAuthed) {
      openLogin();
      return;
    }
    if (charCount(text) > MAX_CHAT_CHARS) {
      toast.error(`Message must be ${MAX_CHAT_CHARS} characters or fewer.`);
      return;
    }
    if (!onSendChat) return;
    const ok = onSendChat(text);
    if (ok === false) {
      toast.error(t('liveRoom.chatSendFailed'));
      return;
    }
    setInput('');
    inputValueRef.current = '';
    setEmojiOpen(false);
  };

  const insertEmoji = (emoji: string) => {
    if (!isAuthed) {
      openLogin();
      return;
    }
    const el = inputRef.current;
    const current = inputValueRef.current;
    const start = el?.selectionStart ?? current.length;
    const end = el?.selectionEnd ?? start;
    const next = `${current.slice(0, start)}${emoji}${current.slice(end)}`;
    if (charCount(next.trim()) > MAX_CHAT_CHARS) {
      toast.error(`Message must be ${MAX_CHAT_CHARS} characters or fewer.`);
      return;
    }
    const caret = start + emoji.length;
    inputValueRef.current = next;
    setInput(next);
    window.requestAnimationFrame(() => {
      inputRef.current?.focus();
      inputRef.current?.setSelectionRange(caret, caret);
    });
  };

  useEffect(() => {
    const el = listRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
  }, [messages.length]);

  useEffect(() => {
    if (!emojiOpen) return;
    const onPointerDown = (event: PointerEvent) => {
      const target = event.target;
      if (target instanceof Node && emojiWrapRef.current?.contains(target)) return;
      setEmojiOpen(false);
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setEmojiOpen(false);
    };
    document.addEventListener('pointerdown', onPointerDown);
    document.addEventListener('keydown', onKeyDown);
    return () => {
      document.removeEventListener('pointerdown', onPointerDown);
      document.removeEventListener('keydown', onKeyDown);
    };
  }, [emojiOpen]);

  const activeEmojiGroup = EMOJI_GROUPS.find((group) => group.id === emojiGroup) ?? EMOJI_GROUPS[0];

  return (
    <aside className={cn('gl-chat', sheetMode && 'is-sheet')} aria-label={t('liveRoom.chat')}>
      <div className="gl-chat-head">
        <button className="gl-chat-tab" aria-label={t('liveRoom.topChat')}>
          <span>{t('liveRoom.topChat')}</span>
          <ChevronDown size={16} />
        </button>
        <div className="flex-1" />
        <button className="gl-icon-btn sm" aria-label={t('liveRoom.more')}>
          <MoreVertical size={18} />
        </button>
        {sheetMode && (
          <button className="gl-icon-btn sm" aria-label={t('liveRoom.close')} onClick={onClose}>
            <X size={18} />
          </button>
        )}
      </div>

      <div className="gl-chat-list" ref={listRef}>
        {messages.map((m) => {
          if (m.kind === 'system') return <SystemNotice key={m.id} m={m} />;
          if (m.kind === 'gift') return <GiftNotice key={m.id} m={m} />;
          if (m.kind === 'super_chat') return <SuperChatCard key={m.id} m={m} />;
          return <ChatRow key={m.id} m={m as ChatMessage} />;
        })}
      </div>

      {reconnecting && (
        <div
          className="gl-chat-reconnect-bar flex items-center justify-center gap-2 bg-bg-hover px-3 py-1 text-xs text-text-secondary"
          role="status"
          aria-live="polite"
        >
          <span className="h-2 w-2 animate-pulse rounded-full bg-text-secondary" />
          <span>{reconnectingLabel ?? 'Reconnecting…'}</span>
        </div>
      )}

      <div className="gl-chat-input">
        <Avatar
          name={currentUser ? userDisplayName(currentUser) : 'You Viewer'}
          src={currentUser?.avatar}
          size={24}
        />
        <div className="gl-chat-input-row">
          <input
            ref={inputRef}
            value={input}
            onChange={(e) => {
              if (!isAuthed) return;
              inputValueRef.current = e.target.value;
              setInput(e.target.value);
            }}
            onFocus={() => {
              if (!isAuthed) openLogin();
            }}
            onCompositionStart={() => {
              composingRef.current = true;
            }}
            onCompositionEnd={() => {
              composingRef.current = false;
            }}
            onKeyDown={(e) => {
              if (!isAuthed) {
                e.preventDefault();
                openLogin();
                return;
              }
              if (e.key !== 'Enter') return;
              // Skip Enter while an IME composition is active. Some browsers
              // also fire Enter with keyCode 229 during composition — guard
              // both to be safe.
              if (composingRef.current || e.nativeEvent.isComposing || e.keyCode === 229) {
                return;
              }
              e.preventDefault();
              trySend();
            }}
            placeholder={isAuthed ? t('liveRoom.sayHi') : 'Sign in to chat'}
            aria-label={t('liveRoom.chatInput')}
            readOnly={!isAuthed}
          />
          <div className="gl-emoji-wrap" ref={emojiWrapRef}>
            <button
              type="button"
              className={cn('gl-icon-btn sm', emojiOpen && 'is-active')}
              aria-label={t('liveRoom.emoji')}
              aria-haspopup="dialog"
              aria-expanded={emojiOpen}
              onClick={() => {
                if (!isAuthed) {
                  openLogin();
                  return;
                }
                setEmojiOpen((open) => !open);
                window.requestAnimationFrame(() => inputRef.current?.focus());
              }}
            >
              <Smile size={18} />
            </button>
            {emojiOpen && (
              <div
                className="gl-emoji-popover"
                role="dialog"
                aria-label={t('liveRoom.emoji')}
                onPointerDown={(event) => event.preventDefault()}
              >
                <div className="gl-emoji-tabs" role="tablist" aria-label={t('liveRoom.emoji')}>
                  {EMOJI_GROUPS.map((group) => (
                    <button
                      key={group.id}
                      type="button"
                      role="tab"
                      aria-selected={emojiGroup === group.id}
                      aria-label={group.label}
                      title={group.label}
                      className={cn('gl-emoji-tab', emojiGroup === group.id && 'is-active')}
                      onClick={() => setEmojiGroup(group.id)}
                    >
                      {group.icon}
                    </button>
                  ))}
                </div>
                <div className="gl-emoji-grid" role="group" aria-label={activeEmojiGroup.label}>
                  {activeEmojiGroup.items.map((emoji) => (
                    <button
                      key={emoji}
                      type="button"
                      className="gl-emoji-item"
                      aria-label={emoji}
                      onPointerDown={(event) => {
                        event.preventDefault();
                        event.stopPropagation();
                        insertEmoji(emoji);
                      }}
                    >
                      {emoji}
                    </button>
                  ))}
                </div>
              </div>
            )}
          </div>
          <button
            className="gl-icon-btn sm"
            aria-label={t('liveRoom.send')}
            title={t('liveRoom.send')}
            disabled={isAuthed && (!input.trim() || charCount(input.trim()) > MAX_CHAT_CHARS)}
            onClick={() => {
              if (!isAuthed) {
                openLogin();
                return;
              }
              trySend();
            }}
          >
            <Send size={18} />
          </button>
          <button
            className="gl-icon-btn sm"
            aria-label={t('liveRoom.superChat')}
            onClick={() => {
              if (!isAuthed) {
                openLogin(() => onSendSuperChat?.());
                return;
              }
              onSendSuperChat?.();
            }}
          >
            <CircleDollarSign size={18} />
          </button>
        </div>
      </div>

      <div className="gl-chat-foot">
        <span>{t('liveRoom.liveChat')}</span>
      </div>
    </aside>
  );
}
