import {
  useCallback,
  useEffect,
  useLayoutEffect,
  memo,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type WheelEvent,
} from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import {
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  Smile,
  CircleDollarSign,
  Send,
  MessageCircle,
  Users,
  Crown,
  ShieldCheck,
  Flag,
  Ban,
  MoreVertical,
} from 'lucide-react';
import { Avatar } from '@/components/Avatar';
import { GiftArt } from '@/features/gifts/GiftArt';
import { builtInGiftKey, GIFT_ART } from '@/features/gifts/giftArt';
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
import { formatNumber } from '@/lib/format';
import { CoinAmount } from '@/components/CoinAmount';
import { useIsAuthed, useAuthStore } from '@/stores/useAuthStore';
import { userDisplayName } from '@/types/user';
import { useAuthModalStore } from '@/stores/useAuthModalStore';
import type { RoomViewer } from '@/stores/useRealtimeStore';
import { fanBadgeToneClass } from '@/lib/fanBadgeTone';
import type { ReportTargetDraft } from '@/features/reporting/ReportDialog';

export interface ChatProps {
  messages: Message[];
  viewers?: RoomViewer[];
  viewerTotal?: number;
  ownerId?: string;
  ownerName?: string;
  roomId?: string;
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
  onReportMessage?: (target: ReportTargetDraft) => void;
  readOnly?: boolean;
  readOnlyLabel?: string;
  showViewersTab?: boolean;
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
const CHAT_VIRTUAL_OVERSCAN_PX = 360;
const CHAT_ROW_ESTIMATE_PX = 40;
const CHAT_OWNER_ROW_ESTIMATE_PX = 56;
const CHAT_NOTICE_ESTIMATE_PX = 68;
const CHAT_GIFT_ESTIMATE_PX = 76;
const CHAT_SUPER_CHAT_ESTIMATE_PX = 112;

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

function comparePinnedSuperChats(
  a: {
    message: SuperChatMessage;
  },
  b: {
    message: SuperChatMessage;
  },
): number {
  if (a.message.tier !== b.message.tier) return b.message.tier - a.message.tier;
  const amountDelta = parseAmountValue(b.message.amount) - parseAmountValue(a.message.amount);
  if (amountDelta !== 0) return amountDelta;
  return b.message.ts - a.message.ts;
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
  // Display names aren't unique, so only fall back to them when the owner's
  // id is unknown; otherwise a message without a userId could pose as host.
  if (ownerId) return m.userId === ownerId;
  if (ownerName) return normalizeName(m.user) === normalizeName(ownerName);
  return false;
}

function isNearChatBottom(el: HTMLDivElement) {
  return el.scrollHeight - el.scrollTop - el.clientHeight <= CHAT_BOTTOM_THRESHOLD_PX;
}

function estimatedMessageHeight(message: Message, ownerId?: string, ownerName?: string): number {
  if (message.kind === 'system') {
    return CHAT_NOTICE_ESTIMATE_PX + Math.max(0, Math.ceil(charCount(message.text) / 42) - 1) * 18;
  }
  if (message.kind === 'gift') return CHAT_GIFT_ESTIMATE_PX;
  if (message.kind === 'super_chat') {
    return (
      CHAT_SUPER_CHAT_ESTIMATE_PX + Math.max(0, Math.ceil(charCount(message.text) / 38) - 1) * 20
    );
  }
  const base = isOwnerMessage(message, ownerId, ownerName)
    ? CHAT_OWNER_ROW_ESTIMATE_PX
    : CHAT_ROW_ESTIMATE_PX;
  return base + Math.max(0, Math.ceil(charCount(message.text) / 42) - 1) * 18;
}

const ChatRow = memo(function ChatRow({
  m,
  isOwner,
  isFan,
  canModerate,
  menuOpen,
  onToggleMenu,
  onCloseMenu,
  onOpenModeration,
  onReportMessage,
}: {
  m: ChatMessage;
  isOwner?: boolean;
  isFan?: boolean;
  canModerate?: boolean;
  menuOpen: boolean;
  onToggleMenu: () => void;
  onCloseMenu: () => void;
  onOpenModeration?: (target: ChatModerationTarget) => void;
  onReportMessage?: (target: ChatModerationTarget & { messageId: string; text: string }) => void;
}) {
  const { t } = useTranslation('pages');
  const role = isOwner ? 'owner' : m.role;
  const canOpenModeration = Boolean(canModerate && m.userId);
  const target = {
    userId: m.userId ?? '',
    user: m.user,
    avatar: m.avatar,
    role,
  };

  return (
    <div
      className={cn(
        'gl-chat-line',
        isOwner && 'is-owner',
        isFan && 'is-fan',
        menuOpen && 'is-menu-open',
      )}
      onClick={onToggleMenu}
      role="button"
      tabIndex={0}
      onKeyDown={(event) => {
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault();
          onToggleMenu();
        }
        if (event.key === 'Escape') onCloseMenu();
      }}
    >
      <div className="gl-chat-avatar-wrap">
        <button
          type="button"
          className="gl-chat-avatar-btn"
          disabled={!canOpenModeration}
          onClick={(event) => {
            event.stopPropagation();
            if (!m.userId) return;
            onCloseMenu();
            onOpenModeration?.(target);
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
          {isOwner && <span className="gl-chat-owner-badge">{t('liveRoom.chatPanel.host')}</span>}
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
        {menuOpen && (
          <div className="gl-chat-action-popover" onClick={(event) => event.stopPropagation()}>
            <button
              type="button"
              onClick={() => {
                onCloseMenu();
                onReportMessage?.({ ...target, messageId: m.id, text: m.text });
              }}
            >
              <Flag size={14} />
              <span>{t('report.chatAction')}</span>
            </button>
            {canModerate && m.userId && (
              <button
                type="button"
                onClick={() => {
                  onCloseMenu();
                  onOpenModeration?.(target);
                }}
              >
                <Ban size={14} />
                <span>
                  {t('liveRoom.moderation.muteActionShort', { defaultValue: 'Mute user' })}
                </span>
              </button>
            )}
          </div>
        )}
      </div>
    </div>
  );
});

const SuperChatCard = memo(function SuperChatCard({
  m,
  menuOpen,
  onToggleMenu,
  onCloseMenu,
  onReport,
}: {
  m: SuperChatMessage;
  menuOpen: boolean;
  onToggleMenu: () => void;
  onCloseMenu: () => void;
  onReport?: (message: SuperChatMessage) => void;
}) {
  const spec = tierSpec(m.tier);
  const { t } = useTranslation('pages');
  return (
    <div
      className={cn('gl-sc', m.pending && 'opacity-50 saturate-50')}
      aria-busy={m.pending ? 'true' : undefined}
    >
      <div className="gl-sc-head" style={{ background: spec.bg }}>
        <Avatar name={m.user} src={m.avatar} size={28} />
        <span className="gl-sc-user">{m.user}</span>
        {m.userLevel && (
          <UserLevelBadge level={m.userLevel} size="compact" className="gl-sc-level" />
        )}
        <CoinAmount value={parseAmountValue(m.amount)} className="gl-sc-amt" />
        <span className="gl-sc-menu-wrap">
          <button type="button" aria-label={t('report.moreActions')} onClick={onToggleMenu}>
            <MoreVertical size={15} />
          </button>
          {menuOpen && (
            <span className="gl-sc-menu">
              <button
                type="button"
                onClick={() => {
                  onCloseMenu();
                  onReport?.(m);
                }}
              >
                <Flag size={13} />
                {t('report.action')}
              </button>
            </span>
          )}
        </span>
      </div>
      {m.text && (
        <div className="gl-sc-body" style={{ background: spec.soft, color: '#0f0f0f' }}>
          {m.text}
        </div>
      )}
    </div>
  );
});

function PinnedSuperChatPill({
  m,
  remainingMs,
  durationMs,
  active,
  onToggle,
}: {
  m: SuperChatMessage;
  remainingMs: number;
  durationMs: number;
  active: boolean;
  onToggle: () => void;
}) {
  const { t } = useTranslation('pages');
  const spec = tierSpec(m.tier);
  const progress = Math.max(0, Math.min(100, (remainingMs / durationMs) * 100));
  const style = {
    '--sc-bg': spec.bg,
    '--sc-soft': spec.soft,
    '--sc-deep-progress': `${progress}%`,
  } as CSSProperties & {
    '--sc-bg': string;
    '--sc-soft': string;
    '--sc-deep-progress': string;
  };

  return (
    <button
      type="button"
      className={cn('gl-sc-pin', active && 'is-expanded', m.pending && 'opacity-60')}
      style={style}
      onClick={onToggle}
      aria-expanded={active}
      data-sc-pin-id={m.id}
      aria-label={t('liveRoom.chatPanel.openSuperChat', {
        user: m.user,
        defaultValue: 'Open SuperChat from {{user}}',
      })}
    >
      <Avatar name={m.user} src={m.avatar} size={26} ring="rgba(255, 255, 255, 0.76)" />
      <span className="gl-sc-pin-user">{m.user}</span>
    </button>
  );
}

function PinnedSuperChatBubble({ m, locale }: { m: SuperChatMessage; locale: string }) {
  const { t } = useTranslation('pages');
  const spec = tierSpec(m.tier);
  const style = {
    '--sc-bg': spec.bg,
    '--sc-soft': spec.soft,
  } as CSSProperties & { '--sc-bg': string; '--sc-soft': string };

  return (
    <div className="gl-sc-pin-popover" role="dialog" style={style} data-sc-pin-popover-id={m.id}>
      <div className="gl-sc-pin-popover-head">
        <Avatar
          name={m.user}
          src={m.avatar}
          size={28}
          ring="rgba(255, 255, 255, 0.8)"
          className="gl-sc-pin-popover-avatar"
        />
        <div className="gl-sc-pin-popover-meta">
          <strong>{m.user}</strong>
          <span>
            {t('account.coins', {
              ns: 'common',
              amount: formatNumber(parseAmountValue(m.amount), locale),
              defaultValue: '{{amount}} coins',
            })}
          </span>
        </div>
      </div>
      <p>{m.text || t('liveRoom.chatPanel.noSuperChatMessage')}</p>
    </div>
  );
}

const SystemNotice = memo(function SystemNotice({ m }: { m: SystemMessage }) {
  return (
    <div className="gl-chat-notice">
      <div className="gl-chat-notice-body">{m.text}</div>
    </div>
  );
});

const GiftNotice = memo(function GiftNotice({ m }: { m: GiftMessage }) {
  const { t } = useTranslation('pages');
  const count = m.count ?? 1;
  const gift = { id: m.giftId, name: m.giftName, icon: m.giftIcon };
  const builtIn = builtInGiftKey(gift);
  const tier = m.tier ?? (builtIn ? GIFT_ART[builtIn].tier : 0);
  return (
    <div className={cn('gl-gift-notice', `tier-${tier}`)}>
      <div className="gl-gift-notice-icon" aria-hidden="true">
        <GiftArt gift={gift} size={36} />
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
});

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

const VirtualMessageItem = memo(function VirtualMessageItem({
  message,
  offsetTop,
  height,
  ownerId,
  ownerName,
  roomId,
  canModerate,
  menuOpen,
  onToggleMenu,
  onCloseMenu,
  onOpenModeration,
  onReportMessage,
}: {
  message: Message;
  offsetTop: number;
  height: number;
  ownerId?: string;
  ownerName?: string;
  roomId?: string;
  canModerate?: boolean;
  menuOpen: boolean;
  onToggleMenu: (id: string) => void;
  onCloseMenu: () => void;
  onOpenModeration?: (target: ChatModerationTarget) => void;
  onReportMessage?: (target: ReportTargetDraft) => void;
}) {
  const style: CSSProperties = {
    position: 'absolute',
    top: offsetTop,
    left: 0,
    right: 0,
    minHeight: height,
  };

  if (message.kind === 'system') {
    return (
      <div className="gl-chat-virtual-row" style={style}>
        <SystemNotice m={message} />
      </div>
    );
  }

  if (message.kind === 'gift') {
    return (
      <div className="gl-chat-virtual-row" style={style}>
        <GiftNotice m={message} />
      </div>
    );
  }

  if (message.kind === 'super_chat') {
    const menuId = `super_chat:${message.id}`;
    return (
      <div className="gl-chat-virtual-row" style={style}>
        <SuperChatCard
          m={message}
          menuOpen={menuOpen}
          onToggleMenu={() => onToggleMenu(menuId)}
          onCloseMenu={onCloseMenu}
          onReport={(target) =>
            onReportMessage?.({
              targetType: 'super_chat',
              targetId: target.id,
              targetUrl: typeof window !== 'undefined' ? window.location.href : undefined,
              roomId,
              channelId: ownerId ? `ch-${ownerId}` : undefined,
              targetOwnerId: ownerId,
              targetOwnerName: ownerName,
              targetUserId: target.userId,
              targetUserName: target.user,
              targetTitle: ownerName,
              targetText: target.text,
            })
          }
        />
      </div>
    );
  }

  const chat = message as ChatMessage;
  const isOwner = isOwnerMessage(chat, ownerId, ownerName);
  const isFan =
    !isOwner && Boolean(ownerId && chat.fanBadge && chat.fanBadge.creatorId === ownerId);
  const menuId = `chat:${chat.id}`;

  return (
    <div className="gl-chat-virtual-row" style={style}>
      <ChatRow
        m={chat}
        isOwner={isOwner}
        isFan={isFan}
        canModerate={canModerate}
        menuOpen={menuOpen}
        onToggleMenu={() => onToggleMenu(menuId)}
        onCloseMenu={onCloseMenu}
        onOpenModeration={onOpenModeration}
        onReportMessage={(target) =>
          onReportMessage?.({
            targetType: 'danmu',
            targetId: target.messageId,
            targetUrl: typeof window !== 'undefined' ? window.location.href : undefined,
            roomId,
            channelId: ownerId ? `ch-${ownerId}` : undefined,
            targetOwnerId: ownerId,
            targetOwnerName: ownerName,
            targetUserId: target.userId,
            targetUserName: target.user,
            targetTitle: ownerName,
            targetText: target.text,
          })
        }
      />
    </div>
  );
});

export const Chat = memo(function Chat({
  messages,
  viewers = [],
  viewerTotal,
  ownerId,
  ownerName,
  roomId,
  onSendSuperChat,
  sheetMode,
  reconnecting,
  reconnectingLabel,
  onSendChat,
  onComposerFocusChange,
  canModerate,
  chatMuted,
  onOpenModeration,
  onReportMessage,
  readOnly,
  readOnlyLabel,
  showViewersTab = true,
}: ChatProps) {
  const { t, i18n } = useTranslation('pages');
  const locale = i18n.resolvedLanguage ?? i18n.language;
  const listRef = useRef<HTMLDivElement | null>(null);
  const inputRef = useRef<HTMLInputElement | null>(null);
  const emojiWrapRef = useRef<HTMLDivElement | null>(null);
  const pinStackRef = useRef<HTMLDivElement | null>(null);
  const pinRowRef = useRef<HTMLDivElement | null>(null);
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
  const [pinnedScrollState, setPinnedScrollState] = useState({
    hasOverflow: false,
    canScrollLeft: false,
    canScrollRight: false,
  });
  const [newMessageCount, setNewMessageCount] = useState(0);
  const [openActionMenuId, setOpenActionMenuId] = useState<string | null>(null);
  const [listViewport, setListViewport] = useState({ scrollTop: 0, height: 0 });
  // Track IME composition so Enter during candidate selection (CJK input
  // methods) does not submit a half-finished message.
  const composingRef = useRef(false);
  const isAuthed = useIsAuthed();
  const currentUser = useAuthStore((s) => s.user);
  const openLogin = useAuthModalStore((s) => s.openLogin);
  const effectiveTab = showViewersTab ? activeTab : 'chat';
  const closeActionMenu = useCallback(() => setOpenActionMenuId(null), []);
  const toggleActionMenu = useCallback((id: string) => {
    setExpandedPinnedId(null);
    setOpenActionMenuId((current) => (current === id ? null : id));
  }, []);

  const trySend = () => {
    if (readOnly) return;
    const text = inputValueRef.current.trim();
    if (!text) return;
    if (!isAuthed) {
      openLogin();
      return;
    }
    if (chatMuted) {
      toast.error(
        t('liveRoom.moderation.youAreMuted', { defaultValue: '你当前已被禁言，暂时不能发言。' }),
      );
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
    if (readOnly) return;
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

  const updateListViewport = useCallback(() => {
    const el = listRef.current;
    if (!el) return;
    const next = {
      scrollTop: el.scrollTop,
      height: el.clientHeight,
    };
    setListViewport((current) =>
      current.scrollTop === next.scrollTop && current.height === next.height ? current : next,
    );
  }, []);

  const scrollChatToBottom = useCallback(
    (behavior: ScrollBehavior = 'auto') => {
      const el = listRef.current;
      if (!el) return;
      el.scrollTo({ top: el.scrollHeight, behavior });
      stickToBottomRef.current = true;
      setNewMessageCount(0);
      window.requestAnimationFrame(updateListViewport);
    },
    [updateListViewport],
  );

  const handleChatScroll = () => {
    const el = listRef.current;
    if (!el) return;
    const nearBottom = isNearChatBottom(el);
    stickToBottomRef.current = nearBottom;
    if (nearBottom) setNewMessageCount(0);
    updateListViewport();
  };

  const updatePinnedScrollState = useCallback(() => {
    const row = pinRowRef.current;
    if (!row) {
      setPinnedScrollState({
        hasOverflow: false,
        canScrollLeft: false,
        canScrollRight: false,
      });
      return;
    }

    const maxScrollLeft = Math.max(0, row.scrollWidth - row.clientWidth);
    setPinnedScrollState({
      hasOverflow: maxScrollLeft > 2,
      canScrollLeft: row.scrollLeft > 2,
      canScrollRight: row.scrollLeft < maxScrollLeft - 2,
    });
  }, []);

  const handlePinnedRowScroll = () => {
    updatePinnedScrollState();
  };

  const handlePinnedWheel = (event: WheelEvent<HTMLDivElement>) => {
    const row = pinRowRef.current;
    if (!row || row.scrollWidth <= row.clientWidth) return;
    if (Math.abs(event.deltaY) <= Math.abs(event.deltaX)) return;

    event.preventDefault();
    row.scrollLeft += event.deltaY;
    updatePinnedScrollState();
  };

  const scrollPinnedRow = (direction: -1 | 1) => {
    const row = pinRowRef.current;
    if (!row) return;
    const distance = Math.max(148, row.clientWidth * 0.66);
    const left = direction * distance;

    if (typeof row.scrollBy === 'function') {
      row.scrollBy({ left, behavior: 'smooth' });
    } else {
      row.scrollLeft += left;
    }

    window.setTimeout(updatePinnedScrollState, 220);
  };

  useLayoutEffect(() => {
    const previousCount = lastMessageCountRef.current;
    const nextCount = messages.length;
    const newCount = Math.max(0, nextCount - previousCount);
    lastMessageCountRef.current = nextCount;

    if (effectiveTab !== 'chat') return;

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
  }, [effectiveTab, messages.length, scrollChatToBottom]);

  useLayoutEffect(() => {
    if (effectiveTab !== 'chat') return undefined;
    updateListViewport();
    const el = listRef.current;
    if (!el || typeof ResizeObserver === 'undefined') return undefined;
    const observer = new ResizeObserver(updateListViewport);
    observer.observe(el);
    return () => observer.disconnect();
  }, [effectiveTab, updateListViewport]);

  useEffect(() => {
    if (!showViewersTab && activeTab !== 'chat') {
      setActiveTab('chat');
    }
  }, [activeTab, showViewersTab]);

  useEffect(() => {
    setOpenActionMenuId(null);
  }, [effectiveTab]);

  useEffect(() => {
    if (!openActionMenuId) return;
    const stillExists = messages.some((message) => {
      if (message.kind === 'system' || message.kind === 'gift') return false;
      const menuId =
        message.kind === 'super_chat' ? `super_chat:${message.id}` : `chat:${message.id}`;
      return menuId === openActionMenuId;
    });
    if (!stillExists) setOpenActionMenuId(null);
  }, [messages, openActionMenuId]);

  useEffect(() => {
    if (!openActionMenuId) return;
    const handlePointerDown = (event: PointerEvent) => {
      const target = event.target;
      if (!(target instanceof Element)) {
        setOpenActionMenuId(null);
        return;
      }
      if (target.closest('.gl-chat-line.is-menu-open, .gl-sc-menu-wrap')) return;
      setOpenActionMenuId(null);
    };
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setOpenActionMenuId(null);
    };

    document.addEventListener('pointerdown', handlePointerDown, true);
    document.addEventListener('keydown', handleKeyDown);
    return () => {
      document.removeEventListener('pointerdown', handlePointerDown, true);
      document.removeEventListener('keydown', handleKeyDown);
    };
  }, [openActionMenuId]);

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
    .sort(comparePinnedSuperChats);
  const virtualLayout = useMemo(() => {
    let offsetTop = 0;
    const rows = messages.map((message) => {
      const height = estimatedMessageHeight(message, ownerId, ownerName);
      const row = { message, offsetTop, height };
      offsetTop += height;
      return row;
    });
    return {
      rows,
      totalHeight: offsetTop,
    };
  }, [messages, ownerId, ownerName]);
  const visibleVirtualRows = useMemo(() => {
    if (listViewport.height <= 0) return virtualLayout.rows;
    const start = Math.max(0, listViewport.scrollTop - CHAT_VIRTUAL_OVERSCAN_PX);
    const end = listViewport.scrollTop + listViewport.height + CHAT_VIRTUAL_OVERSCAN_PX;
    return virtualLayout.rows.filter(
      (row) => row.offsetTop + row.height >= start && row.offsetTop <= end,
    );
  }, [listViewport.height, listViewport.scrollTop, virtualLayout]);

  useLayoutEffect(() => {
    updatePinnedScrollState();
    const row = pinRowRef.current;
    if (!row) return;

    if (typeof ResizeObserver === 'undefined') return;
    const observer = new ResizeObserver(() => updatePinnedScrollState());
    observer.observe(row);
    return () => observer.disconnect();
  }, [pinnedSuperChats.length, updatePinnedScrollState]);

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

  useEffect(() => {
    if (!expandedPinnedId) return;
    const handlePointerDown = (event: PointerEvent) => {
      const target = event.target;
      if (target instanceof Element) {
        const pin = target.closest('[data-sc-pin-id]') as HTMLElement | null;
        const popover = target.closest('[data-sc-pin-popover-id]') as HTMLElement | null;
        if (pin?.dataset.scPinId === expandedPinnedId) return;
        if (popover?.dataset.scPinPopoverId === expandedPinnedId) return;
      }
      setExpandedPinnedId(null);
    };
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setExpandedPinnedId(null);
    };

    document.addEventListener('pointerdown', handlePointerDown, true);
    document.addEventListener('keydown', handleKeyDown);
    return () => {
      document.removeEventListener('pointerdown', handlePointerDown, true);
      document.removeEventListener('keydown', handleKeyDown);
    };
  }, [expandedPinnedId]);

  return (
    <aside
      className={cn('gl-chat', sheetMode && 'is-sheet', readOnly && 'is-readonly')}
      aria-label={t('liveRoom.chat')}
    >
      <div
        className={cn('gl-chat-tabs', !showViewersTab && 'is-single')}
        role="tablist"
        aria-label={t('liveRoom.chat')}
      >
        <button
          type="button"
          role="tab"
          aria-selected={effectiveTab === 'chat'}
          className={cn('gl-chat-tab-btn', effectiveTab === 'chat' && 'is-active')}
          onClick={() => {
            forceScrollOnChatOpenRef.current = true;
            setActiveTab('chat');
          }}
        >
          <MessageCircle size={16} />
          <span>{t('liveRoom.chatPanel.chatTab')}</span>
        </button>
        {showViewersTab && (
          <button
            type="button"
            role="tab"
            aria-selected={effectiveTab === 'viewers'}
            className={cn('gl-chat-tab-btn', effectiveTab === 'viewers' && 'is-active')}
            onClick={() => setActiveTab('viewers')}
          >
            <Users size={16} />
            <span>{t('liveRoom.chatPanel.viewersTab')}</span>
            <span className="gl-chat-tab-count">
              {(viewerTotal ?? viewers.length).toLocaleString(locale)}
            </span>
          </button>
        )}
      </div>
      {effectiveTab === 'chat' && pinnedSuperChats.length > 0 && (
        <div
          className={cn('gl-sc-pin-stack', pinnedScrollState.hasOverflow && 'has-overflow')}
          ref={pinStackRef}
          aria-label={t('liveRoom.chatPanel.pinnedSuperChats')}
        >
          <button
            type="button"
            className="gl-sc-pin-scroll is-left"
            aria-label={t('liveRoom.chatPanel.scrollSuperChatsLeft', {
              defaultValue: 'Scroll SuperChats left',
            })}
            disabled={!pinnedScrollState.canScrollLeft}
            onClick={() => scrollPinnedRow(-1)}
          >
            <ChevronLeft size={18} strokeWidth={2.6} />
          </button>
          <div
            ref={pinRowRef}
            className="gl-sc-pin-row"
            onScroll={handlePinnedRowScroll}
            onWheel={handlePinnedWheel}
          >
            {pinnedSuperChats.map(({ message, remainingMs, durationMs }) => (
              <PinnedSuperChatPill
                key={message.id}
                m={message}
                remainingMs={remainingMs}
                durationMs={durationMs}
                active={expandedPinnedId === message.id}
                onToggle={() => {
                  setOpenActionMenuId(null);
                  setExpandedPinnedId((current) => {
                    const next = current === message.id ? null : message.id;
                    return next;
                  });
                }}
              />
            ))}
          </div>
          <button
            type="button"
            className="gl-sc-pin-scroll is-right"
            aria-label={t('liveRoom.chatPanel.scrollSuperChatsRight', {
              defaultValue: 'Scroll SuperChats right',
            })}
            disabled={!pinnedScrollState.canScrollRight}
            onClick={() => scrollPinnedRow(1)}
          >
            <ChevronRight size={18} strokeWidth={2.6} />
          </button>
          {expandedPinnedId && (
            <PinnedSuperChatBubble
              m={
                pinnedSuperChats.find((item) => item.message.id === expandedPinnedId)?.message ??
                pinnedSuperChats[0].message
              }
              locale={locale}
            />
          )}
        </div>
      )}
      {effectiveTab === 'chat' ? (
        <div className="gl-chat-list" ref={listRef} onScroll={handleChatScroll}>
          <div className="gl-chat-virtual-space" style={{ height: virtualLayout.totalHeight }}>
            {visibleVirtualRows.map((row) => {
              const message = row.message;
              const menuId =
                message.kind === 'super_chat' ? `super_chat:${message.id}` : `chat:${message.id}`;
              return (
                <VirtualMessageItem
                  key={message.id}
                  message={message}
                  offsetTop={row.offsetTop}
                  height={row.height}
                  ownerId={ownerId}
                  ownerName={ownerName}
                  roomId={roomId}
                  canModerate={canModerate}
                  menuOpen={openActionMenuId === menuId}
                  onToggleMenu={toggleActionMenu}
                  onCloseMenu={closeActionMenu}
                  onOpenModeration={onOpenModeration}
                  onReportMessage={onReportMessage}
                />
              );
            })}
          </div>
        </div>
      ) : (
        <ViewerRankList
          viewers={viewers}
          total={viewerTotal ?? viewers.length}
          canModerate={canModerate}
          onOpenModeration={onOpenModeration}
        />
      )}

      {effectiveTab === 'chat' && newMessageCount > 0 && (
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

      {effectiveTab === 'chat' && reconnecting && (
        <div
          className="gl-chat-reconnect-bar flex items-center justify-center gap-2 bg-bg-hover px-3 py-1 text-xs text-text-secondary"
          role="status"
          aria-live="polite"
        >
          <span className="h-2 w-2 animate-pulse rounded-full bg-text-secondary" />
          <span>{reconnectingLabel ?? t('liveRoom.connection.reconnectingShort')}</span>
        </div>
      )}

      {effectiveTab === 'chat' && readOnly && (
        <div className="gl-chat-readonly" role="note">
          {readOnlyLabel ??
            t('liveRoom.replay.chatReadOnly', { defaultValue: 'Replay chat is read-only.' })}
        </div>
      )}

      {effectiveTab === 'chat' && !readOnly && (
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
                  ? t('liveRoom.moderation.chatMutedPlaceholder', {
                      defaultValue: '你当前已被禁言',
                    })
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
              disabled={
                isAuthed && (chatMuted || !input.trim() || charCount(input.trim()) > MAX_CHAT_CHARS)
              }
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
});
