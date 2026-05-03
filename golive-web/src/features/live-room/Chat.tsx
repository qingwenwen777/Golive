import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type CSSProperties,
} from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import {
  ChevronDown,
  Smile,
  CircleDollarSign,
  Send,
  Gift,
  MessageCircle,
  Users,
  Crown,
  ShieldCheck,
} from 'lucide-react';
import { Avatar } from '@/components/Avatar';
import { UserLevelBadge } from '@/components/UserLevelBadge';
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
import type { RoomViewer } from '@/stores/useRealtimeStore';
import { fanBadgeToneClass } from '@/lib/fanBadgeTone';

export interface ChatProps {
  messages: Message[];
  viewers?: RoomViewer[];
  viewerTotal?: number;
  ownerId?: string;
  ownerName?: string;
  onSendSuperChat?: () => void;
  sheetMode?: boolean;
  reconnecting?: boolean;
  reconnectingLabel?: string;
  onSendChat?: (text: string) => boolean | void;
  onComposerFocusChange?: (focused: boolean) => void;
  canModerate?: boolean;
  chatMuted?: boolean;
  moderationRole?: 'owner' | 'moderator' | 'viewer' | string;
  onOpenModeration?: (target: ChatModerationTarget) => void;
}

const EMOJI_GROUPS = [
  {
    id: 'faces',
    icon: '\u{1f600}',
    label: 'Faces',
    items: [
      '\u{1f600}',
      '\u{1f604}',
      '\u{1f602}',
      '\u{1f60a}',
      '\u{1f60d}',
      '\u{1f973}',
      '\u{1f60e}',
      '\u{1f914}',
      '\u{1f62d}',
      '\u{1f621}',
      '\u{1f634}',
      '\u{1f92f}',
    ],
  },
  {
    id: 'gestures',
    icon: '\u{1f44d}',
    label: 'Gestures',
    items: [
      '\u{1f44d}',
      '\u{1f44e}',
      '\u{1f44f}',
      '\u{1f64c}',
      '\u{1f64f}',
      '\u{1f91d}',
      '\u{1f4aa}',
      '\u{1f440}',
      '\u{270c}\ufe0f',
      '\u{1f918}',
      '\u{1f44c}',
      '\u{1faf6}',
    ],
  },
  {
    id: 'stream',
    icon: '\u{2764}\ufe0f',
    label: 'Stream',
    items: [
      '\u{2764}\ufe0f',
      '\u{1f525}',
      '\u{2728}',
      '\u{1f389}',
      '\u{1f4af}',
      '\u{2b50}',
      '\u{1f31f}',
      '\u{26a1}',
      '\u{1f3b5}',
      '\u{1f3ae}',
      '\u{1f3c6}',
      '\u{1f48e}',
    ],
  },
] as const;

const MAX_CHAT_CHARS = 200;
const CHAT_BOTTOM_THRESHOLD_PX = 48;

const SC_PIN_REFRESH_MS = 1000;

type ChatPanelTab = 'chat' | 'viewers';

export interface ChatModerationTarget {
  userId: string;
  user: string;
  avatar?: string;
  role?: 'owner' | 'moderator' | 'viewer' | string;
}

function parseAmountValue(amount: string): number {
  const raw = amount.replace(/[^\d]/g, '');
  const value = Number(raw);
  return Number.isFinite(value) ? value : 0;
}

// formatYenAmount normalises a wire amount (raw "1000", "1,000", or "JPY1,000")
// to a single canonical yen rendering while keeping the source ASCII-safe.
function formatYenAmount(amount: string): string {
  const n = parseAmountValue(amount);
  return `\u00a5${n.toLocaleString('en-US')}`;
}

function superChatPinDurationMs(m: SuperChatMessage): number {
  const amount = parseAmountValue(m.amount);
  if (amount >= 20000) return 15 * 60_000;
  if (amount >= 10000) return 10 * 60_000;
  if (amount >= 5000) return 7 * 60_000;
  if (amount >= 2000) return 5 * 60_000;
  if (amount >= 1000) return 3 * 60_000;
  if (amount >= 500) return 2 * 60_000;
  return 60_000;
}

function formatRemaining(ms: number): string {
  const totalSeconds = Math.max(0, Math.ceil(ms / 1000));
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return minutes > 0 ? `${minutes}:${seconds.toString().padStart(2, '0')}` : `${seconds}s`;
}

const GIFT_META: Record<string, { icon: string; tier: 0 | 1 | 2 | 3 }> = {
  flower: { icon: '\u{1f33c}', tier: 0 },
  donut: { icon: '\u{1f369}', tier: 0 },
  cake: { icon: '\u{1f370}', tier: 0 },
  ramen: { icon: '\u{1f35c}', tier: 0 },
  rocket: { icon: '\u{1f680}', tier: 1 },
  fan_light: { icon: '\u{1f4a1}', tier: 2 },
  crown: { icon: '\u{1f451}', tier: 2 },
  gem: { icon: '\u{1f48e}', tier: 2 },
  yacht: { icon: '\u{1f6e5}\ufe0f', tier: 3 },
  castle: { icon: '\u{1f3f0}', tier: 3 },
};

function giftMeta(name: string): { icon?: string; tier: 0 | 1 | 2 | 3 } {
  const fallback = GIFT_META[name.trim().toLowerCase().replace(/\s+/g, '_')];
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

function normalizeName(name?: string): string {
  return (name ?? '').trim().toLowerCase();
}

function isOwnerMessage(m: ChatMessage, ownerId?: string, ownerName?: string): boolean {
  if (ownerId && m.userId) return m.userId === ownerId;
  if (ownerName) return normalizeName(m.user) === normalizeName(ownerName);
  return false;
}

function isNearChatBottom(el: HTMLDivElement) {
  return el.scrollHeight - el.scrollTop - el.clientHeight <= CHAT_BOTTOM_THRESHOLD_PX;
}

function ChatRow({
  m,
  isOwner,
  isFan,
  canModerate,
  onOpenModeration,
}: {
  m: ChatMessage;
  isOwner?: boolean;
  isFan?: boolean;
  canModerate?: boolean;
  onOpenModeration?: (target: ChatModerationTarget) => void;
}) {
  const { t } = useTranslation('pages');
  const role = isOwner ? 'owner' : m.role;
  const canOpenModeration = Boolean(canModerate && m.userId);

  return (
    <div className={cn('gl-chat-line', isOwner && 'is-owner', isFan && 'is-fan')}>
      <div className="gl-chat-avatar-wrap">
        <button
          type="button"
          className="gl-chat-avatar-btn"
          disabled={!canOpenModeration}
          onClick={() => {
            if (!m.userId) return;
            onOpenModeration?.({
              userId: m.userId,
              user: m.user,
              avatar: m.avatar,
              role,
            });
          }}
          aria-label={t('liveRoom.chatPanel.moderateUser', {
            user: m.user,
            defaultValue: `Moderate ${m.user}`,
          })}
        >
          <Avatar name={m.user} src={m.avatar} size={24} />
        </button>
      </div>
      <div className="gl-chat-body">
        <span className="gl-chat-meta">
          <span
            className="gl-chat-user"
            style={{ color: isOwner ? undefined : (m.color ?? userColor(m.user)) }}
          >
            {m.user}
          </span>
          {isOwner && (
            <span className="gl-chat-owner-badge">{t('liveRoom.chatPanel.host')}</span>
          )}
          {!isOwner && m.role === 'moderator' && (
            <span className="gl-chat-mod-badge">
              <ShieldCheck size={11} strokeWidth={2.5} />
              {t('liveRoom.chatPanel.moderatorBadge', { defaultValue: '房管' })}
            </span>
          )}
          {!isOwner && m.userLevel && <UserLevelBadge level={m.userLevel} size="compact" />}
          {isFan && m.fanBadge && (
            <span
              className={cn('gl-chat-fan-badge', fanBadgeToneClass(m.fanBadge.level))}
              title={t('liveRoom.chatPanel.fanBadgeTitle', { level: m.fanBadge.level })}
            >
              <Crown size={12} strokeWidth={2.4} />
              <span>#{m.fanBadge.level}</span>
            </span>
          )}
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
      aria-busy={m.pending ? 'true' : undefined}
    >
      <div className="gl-sc-head" style={{ background: spec.bg }}>
        <Avatar name={m.user} src={m.avatar} size={28} />
        <span className="gl-sc-user">{m.user}</span>
        {m.userLevel && <UserLevelBadge level={m.userLevel} size="compact" className="gl-sc-level" />}
        <span className="gl-sc-amt">{formatYenAmount(m.amount)}</span>
      </div>
      {m.text && (
        <div className="gl-sc-body" style={{ background: spec.soft, color: '#0f0f0f' }}>
          {m.text}
        </div>
      )}
    </div>
  );
}

function PinnedSuperChatCard({
  m,
  remainingMs,
  durationMs,
  expanded,
  onToggle,
}: {
  m: SuperChatMessage;
  remainingMs: number;
  durationMs: number;
  expanded: boolean;
  onToggle: () => void;
}) {
  const { t } = useTranslation('pages');
  const spec = tierSpec(m.tier);
  const progress = Math.max(0, Math.min(100, (remainingMs / durationMs) * 100));
  const style = {
    background: spec.bg,
    '--sc-soft': spec.soft,
    '--sc-progress': `${progress}%`,
  } as CSSProperties & { '--sc-soft': string; '--sc-progress': string };

  return (
    <button
      type="button"
      className={cn('gl-sc-pin', expanded && 'is-expanded', m.pending && 'opacity-60')}
      style={style}
      onClick={onToggle}
      aria-expanded={expanded}
      aria-label={t('liveRoom.chatPanel.openSuperChat', { user: m.user })}
    >
      <div className="gl-sc-pin-main">
        <Avatar name={m.user} src={m.avatar} size={24} />
        <div className="gl-sc-pin-copy">
          <div className="gl-sc-pin-meta">
            <span className="gl-sc-pin-user">{m.user}</span>
            <span className="gl-sc-pin-amount">{formatYenAmount(m.amount)}</span>
          </div>
          <div className="gl-sc-pin-hint">
            <span>{formatRemaining(remainingMs)}</span>
            <span>
              {m.text ? t('liveRoom.chatPanel.tapToRead') : t('liveRoom.chatPanel.noMessage')}
            </span>
          </div>
        </div>
        <ChevronDown className="gl-sc-pin-chevron" size={18} />
      </div>
      {expanded && (
        <div className="gl-sc-pin-body">
          {m.text || t('liveRoom.chatPanel.noSuperChatMessage')}
        </div>
      )}
      <span className="gl-sc-pin-progress" aria-hidden="true" />
    </button>
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
  const { t } = useTranslation('pages');
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
          <span className="gl-gift-notice-user">
            {m.self ? t('liveRoom.chatPanel.you') : m.user}
          </span>
          {m.userLevel && <UserLevelBadge level={m.userLevel} size="compact" />}
          <span>{t('liveRoom.chatPanel.sentGift')}</span>
        </div>
        <div className="gl-gift-notice-meta">
          <span className="gl-gift-notice-name">{m.giftName}</span>
          <span className="gl-gift-notice-count">x{count}</span>
        </div>
      </div>
    </div>
  );
}

function formatContribution(value: number, locale: string): string {
  return Math.max(0, Math.floor(value)).toLocaleString(locale);
}

function ViewerRankList({
  viewers,
  total,
  canModerate,
  onOpenModeration,
}: {
  viewers: RoomViewer[];
  total: number;
  canModerate?: boolean;
  onOpenModeration?: (target: ChatModerationTarget) => void;
}) {
  const { t, i18n } = useTranslation('pages');
  const locale = i18n.resolvedLanguage ?? i18n.language;
  const sorted = viewers
    .slice()
    .sort((a, b) => {
      if (a.contribution !== b.contribution) return b.contribution - a.contribution;
      return a.user.localeCompare(b.user);
    })
    .slice(0, 100);

  return (
    <div className="gl-viewer-panel">
      <div className="gl-viewer-summary">
        <span>
          {t('liveRoom.chatPanel.watchingNow', {
            count: total,
            formattedCount: total.toLocaleString(locale),
          })}
        </span>
        <span>{t('liveRoom.chatPanel.dailyContribution')}</span>
      </div>
      {sorted.length === 0 ? (
        <div className="gl-viewer-empty">
          <Users size={34} strokeWidth={1.6} />
          <span>{t('liveRoom.chatPanel.emptyViewers')}</span>
        </div>
      ) : (
        <div className="gl-viewer-list">
          {sorted.map((viewer, index) => {
            const rank = index + 1;
            return (
              <div key={`${viewer.userId ?? viewer.user}-${index}`} className="gl-viewer-row">
                <span className={cn('gl-viewer-rank', rank <= 3 && `top-${rank}`)}>
                  {rank <= 3 ? t('liveRoom.chatPanel.rankTop', { rank }) : rank}
                </span>
                <button
                  type="button"
                  className="gl-viewer-avatar-btn"
                  disabled={!canModerate || !viewer.userId}
                  onClick={() => {
                    if (!viewer.userId) return;
                    onOpenModeration?.({
                      userId: viewer.userId,
                      user: viewer.user,
                      avatar: viewer.avatar,
                      role: 'viewer',
                    });
                  }}
                  aria-label={t('liveRoom.chatPanel.moderateUser', {
                    user: viewer.user,
                    defaultValue: `Moderate ${viewer.user}`,
                  })}
                >
                  <Avatar name={viewer.user} src={viewer.avatar} size={34} />
                </button>
                <div className="gl-viewer-main">
                  <span className="gl-viewer-name">
                    <span>{viewer.user}</span>
                    {viewer.userLevel && <UserLevelBadge level={viewer.userLevel} size="compact" />}
                  </span>
                  <span className="gl-viewer-sub">{t('liveRoom.chatPanel.online')}</span>
                </div>
                <div className="gl-viewer-score">
                  <span>{formatContribution(viewer.contribution, locale)}</span>
                  <small>{t('liveRoom.chatPanel.contribution')}</small>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

export function Chat({
  messages,
  viewers = [],
  viewerTotal,
  ownerId,
  ownerName,
  onSendSuperChat,
  sheetMode,
  reconnecting,
  reconnectingLabel,
  onSendChat,
  onComposerFocusChange,
  canModerate,
  chatMuted,
  onOpenModeration,
}: ChatProps) {
  const { t, i18n } = useTranslation('pages');
  const locale = i18n.resolvedLanguage ?? i18n.language;
  const listRef = useRef<HTMLDivElement | null>(null);
  const inputRef = useRef<HTMLInputElement | null>(null);
  const emojiWrapRef = useRef<HTMLDivElement | null>(null);
  const inputValueRef = useRef('');
  const lastMessageCountRef = useRef(messages.length);
  const stickToBottomRef = useRef(true);
  const forceScrollOnChatOpenRef = useRef(false);
  const [input, setInput] = useState('');
  const [emojiOpen, setEmojiOpen] = useState(false);
  const [emojiGroup, setEmojiGroup] = useState<(typeof EMOJI_GROUPS)[number]['id']>('faces');
  const [now, setNow] = useState(() => Date.now());
  const [activeTab, setActiveTab] = useState<ChatPanelTab>('chat');
  const [expandedPinnedId, setExpandedPinnedId] = useState<string | null>(null);
  const [newMessageCount, setNewMessageCount] = useState(0);
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
    if (chatMuted) {
      toast.error(t('liveRoom.moderation.youAreMuted', { defaultValue: '你当前已被禁言，暂时不能发言。' }));
      return;
    }
    if (charCount(text) > MAX_CHAT_CHARS) {
      toast.error(t('liveRoom.chatPanel.messageTooLong', { max: MAX_CHAT_CHARS }));
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
    window.requestAnimationFrame(() => {
      inputRef.current?.focus();
      onComposerFocusChange?.(true);
    });
  };

  const insertEmoji = (emoji: string) => {
    if (!isAuthed) {
      openLogin();
      return;
    }
    if (chatMuted) return;
    const el = inputRef.current;
    const current = inputValueRef.current;
    const start = el?.selectionStart ?? current.length;
    const end = el?.selectionEnd ?? start;
    const next = `${current.slice(0, start)}${emoji}${current.slice(end)}`;
    if (charCount(next.trim()) > MAX_CHAT_CHARS) {
      toast.error(t('liveRoom.chatPanel.messageTooLong', { max: MAX_CHAT_CHARS }));
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

  const scrollChatToBottom = useCallback((behavior: ScrollBehavior = 'auto') => {
    const el = listRef.current;
    if (!el) return;
    el.scrollTo({ top: el.scrollHeight, behavior });
    stickToBottomRef.current = true;
    setNewMessageCount(0);
  }, []);

  const handleChatScroll = () => {
    const el = listRef.current;
    if (!el) return;
    const nearBottom = isNearChatBottom(el);
    stickToBottomRef.current = nearBottom;
    if (nearBottom) setNewMessageCount(0);
  };

  useLayoutEffect(() => {
    const previousCount = lastMessageCountRef.current;
    const nextCount = messages.length;
    const newCount = Math.max(0, nextCount - previousCount);
    lastMessageCountRef.current = nextCount;

    if (activeTab !== 'chat') return;

    if (forceScrollOnChatOpenRef.current) {
      forceScrollOnChatOpenRef.current = false;
      scrollChatToBottom();
      return;
    }

    if (previousCount === 0 || nextCount <= previousCount || stickToBottomRef.current) {
      scrollChatToBottom();
      return;
    }

    if (newCount > 0) {
      setNewMessageCount((count) => count + newCount);
    }
  }, [activeTab, messages.length, scrollChatToBottom]);

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
  const hasSuperChats = messages.some((m) => m.kind === 'super_chat');
  const pinnedSuperChats = messages
    .filter((m): m is SuperChatMessage => m.kind === 'super_chat')
    .map((m) => {
      const durationMs = superChatPinDurationMs(m);
      return {
        message: m,
        durationMs,
        remainingMs: m.ts + durationMs - now,
      };
    })
    .filter((item) => item.remainingMs > 0)
    .sort((a, b) => b.message.ts - a.message.ts);

  useEffect(() => {
    if (!hasSuperChats) return;
    setNow(Date.now());
    const timer = window.setInterval(() => setNow(Date.now()), SC_PIN_REFRESH_MS);
    return () => window.clearInterval(timer);
  }, [hasSuperChats]);

  useEffect(() => {
    if (!expandedPinnedId) return;
    if (pinnedSuperChats.some((item) => item.message.id === expandedPinnedId)) return;
    setExpandedPinnedId(null);
  }, [expandedPinnedId, pinnedSuperChats]);

  return (
    <aside className={cn('gl-chat', sheetMode && 'is-sheet')} aria-label={t('liveRoom.chat')}>
      <div className="gl-chat-tabs" role="tablist" aria-label={t('liveRoom.chat')}>
        <button
          type="button"
          role="tab"
          aria-selected={activeTab === 'chat'}
          className={cn('gl-chat-tab-btn', activeTab === 'chat' && 'is-active')}
          onClick={() => {
            forceScrollOnChatOpenRef.current = true;
            setActiveTab('chat');
          }}
        >
          <MessageCircle size={16} />
          <span>{t('liveRoom.chatPanel.chatTab')}</span>
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={activeTab === 'viewers'}
          className={cn('gl-chat-tab-btn', activeTab === 'viewers' && 'is-active')}
          onClick={() => setActiveTab('viewers')}
        >
          <Users size={16} />
          <span>{t('liveRoom.chatPanel.viewersTab')}</span>
          <span className="gl-chat-tab-count">
            {(viewerTotal ?? viewers.length).toLocaleString(locale)}
          </span>
        </button>
      </div>
      {activeTab === 'chat' && pinnedSuperChats.length > 0 && (
        <div className="gl-sc-pin-stack" aria-label={t('liveRoom.chatPanel.pinnedSuperChats')}>
          {pinnedSuperChats.map(({ message, remainingMs, durationMs }) => (
            <PinnedSuperChatCard
              key={message.id}
              m={message}
              remainingMs={remainingMs}
              durationMs={durationMs}
              expanded={expandedPinnedId === message.id}
              onToggle={() =>
                setExpandedPinnedId((current) => (current === message.id ? null : message.id))
              }
            />
          ))}
        </div>
      )}
      {activeTab === 'chat' ? (
        <div className="gl-chat-list" ref={listRef} onScroll={handleChatScroll}>
          {messages.map((m) => {
            if (m.kind === 'system') return <SystemNotice key={m.id} m={m} />;
            if (m.kind === 'gift') return <GiftNotice key={m.id} m={m} />;
            if (m.kind === 'super_chat') return <SuperChatCard key={m.id} m={m} />;
            const chat = m as ChatMessage;
            const isOwner = isOwnerMessage(chat, ownerId, ownerName);
            const isFan =
              !isOwner &&
              Boolean(ownerId && chat.fanBadge && chat.fanBadge.creatorId === ownerId);
            return (
              <ChatRow
                key={m.id}
                m={chat}
                isOwner={isOwner}
                isFan={isFan}
                canModerate={canModerate}
                onOpenModeration={onOpenModeration}
              />
            );
          })}
        </div>
      ) : (
        <ViewerRankList
          viewers={viewers}
          total={viewerTotal ?? viewers.length}
          canModerate={canModerate}
          onOpenModeration={onOpenModeration}
        />
      )}

      {activeTab === 'chat' && newMessageCount > 0 && (
        <button
          type="button"
          className="gl-chat-new-message"
          onClick={() => scrollChatToBottom('smooth')}
          aria-label={t('liveRoom.chatPanel.newMessagesAria', {
            count: newMessageCount,
            formattedCount: newMessageCount.toLocaleString(locale),
          })}
        >
          <span>
            {t('liveRoom.chatPanel.newMessages', {
              count: newMessageCount,
              formattedCount: newMessageCount.toLocaleString(locale),
            })}
          </span>
          <ChevronDown size={14} strokeWidth={2.6} />
        </button>
      )}

      {activeTab === 'chat' && reconnecting && (
        <div
          className="gl-chat-reconnect-bar flex items-center justify-center gap-2 bg-bg-hover px-3 py-1 text-xs text-text-secondary"
          role="status"
          aria-live="polite"
        >
          <span className="h-2 w-2 animate-pulse rounded-full bg-text-secondary" />
          <span>{reconnectingLabel ?? t('liveRoom.connection.reconnectingShort')}</span>
        </div>
      )}

      {activeTab === 'chat' && (
        <div className="gl-chat-input">
          <Avatar
            name={currentUser ? userDisplayName(currentUser) : t('liveRoom.chatPanel.guestViewer')}
            src={currentUser?.avatar}
            size={24}
          />
          <div className="gl-chat-input-row">
            <input
              ref={inputRef}
              value={input}
              onChange={(e) => {
                if (!isAuthed || chatMuted) return;
                inputValueRef.current = e.target.value;
                setInput(e.target.value);
              }}
              onFocus={() => {
                onComposerFocusChange?.(true);
                if (!isAuthed) openLogin();
              }}
              onBlur={() => onComposerFocusChange?.(false)}
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
                // also fire Enter with keyCode 229 during composition; guard
                // both to be safe.
                if (composingRef.current || e.nativeEvent.isComposing || e.keyCode === 229) {
                  return;
                }
                e.preventDefault();
                trySend();
              }}
              placeholder={
                chatMuted
                  ? t('liveRoom.moderation.chatMutedPlaceholder', { defaultValue: '你当前已被禁言' })
                  : isAuthed
                  ? t('liveRoom.sayHi')
                  : t('liveRoom.signInToChat', { defaultValue: 'Sign in to chat' })
              }
              aria-label={t('liveRoom.chatInput')}
              readOnly={!isAuthed || chatMuted}
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
                  if (chatMuted) return;
                  setEmojiOpen((open) => !open);
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
                    {EMOJI_GROUPS.map((group) => {
                      const label = t(`liveRoom.chatPanel.emojiGroups.${group.id}`, {
                        defaultValue: group.label,
                      });
                      return (
                        <button
                          key={group.id}
                          type="button"
                          role="tab"
                          aria-selected={emojiGroup === group.id}
                          aria-label={label}
                          title={label}
                          className={cn('gl-emoji-tab', emojiGroup === group.id && 'is-active')}
                          onClick={() => setEmojiGroup(group.id)}
                        >
                          {group.icon}
                        </button>
                      );
                    })}
                  </div>
                  <div
                    className="gl-emoji-grid"
                    role="group"
                    aria-label={t(`liveRoom.chatPanel.emojiGroups.${activeEmojiGroup.id}`, {
                      defaultValue: activeEmojiGroup.label,
                    })}
                  >
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
              disabled={isAuthed && (chatMuted || !input.trim() || charCount(input.trim()) > MAX_CHAT_CHARS)}
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
      )}
    </aside>
  );
}
