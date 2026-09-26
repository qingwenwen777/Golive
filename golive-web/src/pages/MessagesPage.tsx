import {
  Fragment,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type FormEvent,
  type ReactNode,
  type Ref,
} from 'react';
import { useTranslation } from 'react-i18next';
import { useLocation, useNavigate, useParams } from 'react-router-dom';
import {
  Ban,
  Bell,
  BellOff,
  ChevronsUp,
  Crown,
  MessageCircle,
  MoreVertical,
  Pin,
  Send,
  ShieldCheck,
  ShieldOff,
  UserRound,
  Users,
} from 'lucide-react';
import { toast } from 'sonner';
import {
  useMarkAllNotificationsRead,
  useMarkNotificationRead,
  useNotifications,
  type NotificationItem,
} from '@/api/room';
import {
  useBlockUser,
  useDirectDraft,
  useDirectMessages,
  useDirectThreads,
  useFanGroupMessages,
  useJoinedFanGroups,
  useMarkDirectThreadReadLocal,
  useMarkFanGroupReadLocal,
  useMessagePreference,
  useRequestFanGroupRejoin,
  useSendDirect,
  useSendFanGroupMessage,
  useSendThreadMessage,
  useUpdateMessagePreference,
  useUpdateFanGroupMember,
  useUpdateThreadOptions,
  type DirectMessage,
  type DirectThread,
  type FanGroup,
  type FanGroupMember,
  type MessageFanBadge,
  type MessagePreference,
  type MessageUser,
} from '@/api/messages';
import { Avatar } from '@/components/Avatar';
import { VerifiedBadge } from '@/components/VerifiedBadge';
import { Dialog, DialogContent, DialogTitle } from '@/components/ui/dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { ReportDialog, type ReportTargetDraft } from '@/features/reporting/ReportDialog';
import { cn } from '@/lib/cn';
import { fanBadgeToneClass } from '@/lib/fanBadgeTone';
import { useAuthModalStore } from '@/stores/useAuthModalStore';
import { useAuthStore, useIsAuthed } from '@/stores/useAuthStore';

type MessageSection = 'private' | 'replies' | 'likes' | 'system' | 'settings';
type Translate = ReturnType<typeof useTranslation>['t'];
const CHAT_PAGE_SIZE = 40;
const CHAT_NEW_MESSAGE_NOTICE_MIN_COUNT = 10;
const CHAT_TIME_SEPARATOR_GAP_MS = 5 * 60 * 1000;
type ChatNewNoticeMode = 'entry' | 'live';
type ChatNewNotice = {
  count: number;
  targetId: string;
  mode: ChatNewNoticeMode;
};

type ChatAvatarAction = {
  key: string;
  label: string;
  icon?: ReactNode;
  danger?: boolean;
  onSelect: () => void;
};

export default function MessagesPage() {
  const navigate = useNavigate();
  const { t } = useTranslation('pages');
  const { section: rawSection, targetId } = useParams<{ section?: string; targetId?: string }>();
  const isAuthed = useIsAuthed();
  const openLogin = useAuthModalStore((s) => s.openLogin);
  const user = useAuthStore((s) => s.user);
  const section = normalizeSection(rawSection);
  const draftCreatorId = rawSection === 'direct' ? (targetId ?? '') : '';

  useEffect(() => {
    if (!rawSection) navigate('/messages/private', { replace: true });
  }, [navigate, rawSection]);

  if (!isAuthed) {
    return (
      <div className="gl-page gl-message-page">
        <div className="gl-message-auth">
          <MessageCircle size={34} />
          <h1>{t('messages.auth.title', { defaultValue: '登录后查看消息' })}</h1>
          <p>
            {t('messages.auth.body', {
              defaultValue: '私信、动态通知和系统通知都会集中在这里。',
            })}
          </p>
          <button className="gl-creator-primary" type="button" onClick={() => openLogin()}>
            {t('messages.auth.login', { defaultValue: '登录' })}
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="gl-page gl-message-page">
      <div className="gl-message-shell">
        <main className="gl-message-main">
          {section === 'private' ? (
            <PrivateMessages
              currentUser={{
                id: user?.id ?? '',
                username: user?.username,
                displayName: user?.displayName,
                name: user?.displayName || user?.username || user?.id || '我',
                avatar: user?.avatar,
                verified: Boolean(user?.verified),
                livePermissionStatus: user?.livePermissionStatus,
              }}
              draftCreatorId={draftCreatorId}
            />
          ) : section === 'replies' ? (
            <NotificationPanel
              title={t('messages.sections.replies', { defaultValue: '回复我的' })}
              box="reply"
              empty={t('messages.notifications.repliesEmpty', { defaultValue: '还没有新的回复。' })}
            />
          ) : section === 'likes' ? (
            <NotificationPanel
              title={t('messages.sections.likes', { defaultValue: '收到的赞' })}
              box="like"
              empty={t('messages.notifications.likesEmpty', {
                defaultValue: '你的帖子、评论或直播间收到赞后会显示在这里。',
              })}
            />
          ) : section === 'system' ? (
            <NotificationPanel
              title={t('messages.sections.system', { defaultValue: '系统通知' })}
              box="system"
              empty={t('messages.notifications.systemEmpty', { defaultValue: '暂无系统通知。' })}
            />
          ) : (
            <MessageSettingsPanel />
          )}
        </main>
      </div>
    </div>
  );
}

function PrivateMessages({
  currentUser,
  draftCreatorId,
}: {
  currentUser: MessageUser;
  draftCreatorId: string;
}) {
  const navigate = useNavigate();
  const location = useLocation();
  const { t, i18n } = useTranslation('pages');
  const userId = currentUser.id;
  const threads = useDirectThreads(true, 1, 50);
  const fanGroups = useJoinedFanGroups(true);
  const draft = useDirectDraft(draftCreatorId, Boolean(draftCreatorId));
  const draftErrorReason = apiErrorReason(draft.error);
  const draftBlockedByFollow = Boolean(draftCreatorId && draftErrorReason === 'follow_required');
  const [selectedId, setSelectedId] = useState('');
  const [selectedGroupId, setSelectedGroupId] = useState('');
  const [content, setContent] = useState('');
  const [reportTarget, setReportTarget] = useState<ReportTargetDraft | null>(null);
  const [entryUnread, setEntryUnread] = useState(0);
  const directInputRef = useRef<HTMLInputElement | null>(null);
  const noticeEntry = useMemo(() => parseNoticeEntry(location.search), [location.search]);
  const sendDirect = useSendDirect();
  const markDirectThreadReadLocal = useMarkDirectThreadReadLocal();
  const markFanGroupReadLocal = useMarkFanGroupReadLocal();
  const joinedGroups = fanGroups.data?.items ?? [];
  const selectedGroup = selectedGroupId
    ? joinedGroups.find((item) => item.id === selectedGroupId)
    : undefined;
  const threadByDraft = threads.data?.items.find((item) => item.creatorId === draftCreatorId);
  const selectedDirectThread = selectedId
    ? threads.data?.items.find((item) => item.id === selectedId)
    : undefined;
  const routeDraftThread = draftCreatorId ? (threadByDraft ?? draft.data) : undefined;
  const followRequiredActive = draftBlockedByFollow && !selectedId && !selectedGroupId;
  const selectedThread =
    selectedGroup || followRequiredActive ? undefined : (selectedDirectThread ?? routeDraftThread);
  const conversationCount = (threads.data?.total ?? 0) + joinedGroups.length;
  const messages = useDirectMessages(
    selectedThread?.id ?? '',
    Boolean(selectedThread?.id),
    CHAT_PAGE_SIZE,
  );
  const directMessageItems = useMemo(
    () => mergeMessagePages(messages.data?.pages),
    [messages.data?.pages],
  );
  const directScroll = useChatScroll(directMessageItems, {
    activeKey: selectedThread?.id ? `direct:${selectedThread.id}` : 'direct:none',
    hasNextPage: Boolean(messages.hasNextPage),
    isFetchingNextPage: messages.isFetchingNextPage,
    entryUnread: Math.max(entryUnread, selectedThread?.unread ?? 0),
    onEntryNoticeConsumed: () => setEntryUnread(0),
    fetchNextPage: () => messages.fetchNextPage(),
  });
  const sendThread = useSendThreadMessage(selectedThread?.id ?? '');
  const blockUser = useBlockUser();
  const updateOptions = useUpdateThreadOptions(selectedThread?.id ?? '');
  const sortedThreads = useMemo(
    () => [...(threads.data?.items ?? [])].sort((a, b) => Number(b.pinned) - Number(a.pinned)),
    [threads.data?.items],
  );

  useEffect(() => {
    setSelectedGroupId(noticeEntry.groupId);
    setSelectedId('');
    setContent('');
    setEntryUnread(noticeEntry.unread);
  }, [draftCreatorId, noticeEntry.groupId, noticeEntry.unread]);

  useEffect(() => {
    if (!selectedId && !selectedGroupId && threadByDraft?.id) setSelectedId(threadByDraft.id);
  }, [selectedGroupId, selectedId, threadByDraft?.id]);

  useEffect(() => {
    if (draftBlockedByFollow) {
      toast.error(
        t('messages.private.followRequiredToast', {
          defaultValue: '关注该主播后才能发送私信。',
        }),
      );
    }
  }, [draftBlockedByFollow, t]);

  useEffect(() => {
    if (selectedThread?.id) markDirectThreadReadLocal(selectedThread.id);
  }, [markDirectThreadReadLocal, selectedThread?.id]);

  useEffect(() => {
    if (selectedThread?.id && selectedThread.unread > 0) {
      setEntryUnread(selectedThread.unread);
    }
  }, [selectedThread?.id, selectedThread?.unread]);

  useEffect(() => {
    if (selectedGroup?.id && selectedGroup.unread > 0) {
      setEntryUnread(selectedGroup.unread);
    }
  }, [selectedGroup?.id, selectedGroup?.unread]);

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const text = content.trim();
    if (!text || !selectedThread) return;
    const onSuccess = () => {
      setContent('');
      focusInputSoon(directInputRef);
    };
    const onError = (err: Error) => toast.error(messageError(err, t));
    if (selectedThread.id) {
      sendThread.mutate(text, { onSuccess, onError });
      return;
    }
    sendDirect.mutate(
      { creatorId: selectedThread.creatorId, channelId: selectedThread.channelId, content: text },
      {
        onSuccess: (thread) => {
          setSelectedId(thread.id);
          onSuccess();
        },
        onError,
      },
    );
  };

  const openReport = (thread: DirectThread) => {
    setReportTarget({
      targetType: 'channel',
      targetId: thread.channelId || thread.creatorId,
      targetUrl: `/channel/${thread.creatorId}`,
      channelId: thread.channelId,
      targetOwnerId: thread.creatorId,
      targetOwnerName: thread.peer.name,
      targetTitle: thread.peer.name,
      targetText: thread.lastMessagePreview ?? '',
    });
  };

  return (
    <div className="gl-direct-layout">
      <aside className="gl-direct-list">
        <div className="gl-direct-list-head">
          <h1>{t('messages.private.title', { defaultValue: '我的消息' })}</h1>
          <span>
            {t('messages.private.conversationCount', {
              count: conversationCount,
              defaultValue: '{{count}} 个会话',
            })}
          </span>
        </div>
        {fanGroups.isPending ? null : joinedGroups.length ? (
          <div className="gl-fan-chat-section">
            <span className="gl-fan-chat-section-title">
              {t('messages.private.fanGroups', { defaultValue: '粉丝团群聊' })}
            </span>
            {joinedGroups.map((group) => (
              <FanGroupThreadButton
                key={group.id}
                group={group}
                active={selectedGroup?.id === group.id}
                onClick={() => {
                  if (group.unread > 0) setEntryUnread(group.unread);
                  markFanGroupReadLocal(group.id);
                  setSelectedId('');
                  setSelectedGroupId(group.id);
                }}
              />
            ))}
          </div>
        ) : null}
        {threads.isPending ? (
          <div className="gl-message-empty-soft">
            {t('messages.private.loading', { defaultValue: '正在加载私信...' })}
          </div>
        ) : sortedThreads.length || draft.data ? (
          <>
            {draft.data && !threadByDraft && (
              <ThreadButton
                thread={draft.data}
                active={!selectedGroup && !selectedThread?.id}
                onClick={() => {
                  if (draft.data?.unread && draft.data.unread > 0)
                    setEntryUnread(draft.data.unread);
                  setSelectedGroupId('');
                  setSelectedId('');
                  if (draft.data?.id) markDirectThreadReadLocal(draft.data.id);
                }}
              />
            )}
            {sortedThreads.map((thread) => (
              <ThreadButton
                key={thread.id}
                thread={thread}
                active={selectedThread?.id === thread.id}
                onClick={() => {
                  if (thread.unread > 0) setEntryUnread(thread.unread);
                  markDirectThreadReadLocal(thread.id);
                  setSelectedGroupId('');
                  setSelectedId(thread.id);
                }}
              />
            ))}
          </>
        ) : joinedGroups.length ? null : (
          <div className="gl-message-empty-soft">
            {t('messages.private.emptyHint', {
              defaultValue: '关注主播后，可以从频道页发起私信。',
            })}
          </div>
        )}
      </aside>

      <section className="gl-direct-chat">
        {selectedGroup ? (
          <FanGroupChatView
            group={selectedGroup}
            userId={userId}
            entryUnread={entryUnread}
            onEntryNoticeConsumed={() => setEntryUnread(0)}
          />
        ) : followRequiredActive ? (
          <FollowRequiredDirectState onOpenChannel={() => navigate(`/channel/${draftCreatorId}`)} />
        ) : selectedThread ? (
          <>
            <div className="gl-direct-chat-head">
              <CreatorAvatarButton
                user={selectedThread.peer}
                creatorId={creatorChannelIdForUser(selectedThread.peer, selectedThread.creatorId)}
                enabled={Boolean(
                  creatorChannelIdForUser(selectedThread.peer, selectedThread.creatorId),
                )}
              />
              <div className="gl-direct-chat-meta">
                <strong>
                  {selectedThread.peer.name}
                  {selectedThread.peer.verified && <VerifiedBadge size={14} />}
                </strong>
                <span>
                  {selectedThread.awaitingReply
                    ? t('messages.private.waitingReply', {
                        defaultValue: '等待主播首次回复后才能继续发送',
                      })
                    : selectedThread.muted
                      ? t('messages.private.muted', { defaultValue: '已开启免打扰' })
                      : t('messages.private.normal', { defaultValue: '通知正常接收' })}
                </span>
              </div>
              {selectedThread.id && (
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <button
                      className="gl-message-icon-btn"
                      type="button"
                      aria-label={t('messages.private.actions', { defaultValue: '会话操作' })}
                    >
                      <MoreVertical size={19} />
                    </button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end" className="w-44">
                    <DropdownMenuItem
                      onSelect={() => updateOptions.mutate({ pinned: !selectedThread.pinned })}
                    >
                      <Pin size={15} />
                      {selectedThread.pinned
                        ? t('messages.private.unpin', { defaultValue: '取消置顶' })
                        : t('messages.private.pin', { defaultValue: '置顶聊天' })}
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      onSelect={() => updateOptions.mutate({ muted: !selectedThread.muted })}
                    >
                      {selectedThread.muted ? <Bell size={15} /> : <BellOff size={15} />}
                      {selectedThread.muted
                        ? t('messages.private.unmute', { defaultValue: '关闭免打扰' })
                        : t('messages.private.mute', { defaultValue: '开启免打扰' })}
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      onSelect={() =>
                        updateOptions.mutate({ pushDisabled: !selectedThread.pushDisabled })
                      }
                    >
                      <BellOff size={15} />
                      {selectedThread.pushDisabled
                        ? t('messages.private.enablePush', { defaultValue: '接收推送' })
                        : t('messages.private.disablePush', { defaultValue: '不接收推送' })}
                    </DropdownMenuItem>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem onSelect={() => openReport(selectedThread)}>
                      <ShieldOff size={15} />
                      {t('messages.private.report', { defaultValue: '举报该用户' })}
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      className="gl-menu-danger"
                      onSelect={() =>
                        blockUser.mutate(
                          { userId: selectedThread.peer.id, reason: 'direct_message' },
                          {
                            onSuccess: () =>
                              toast.success(
                                t('messages.private.blockedToast', {
                                  defaultValue: '已加入黑名单',
                                }),
                              ),
                          },
                        )
                      }
                    >
                      <Ban size={15} />
                      {t('messages.private.block', { defaultValue: '加入黑名单' })}
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              )}
            </div>

            <div
              className="gl-direct-message-list"
              ref={directScroll.listRef}
              onScroll={directScroll.onScroll}
            >
              {messages.isFetchingNextPage && (
                <div className="gl-chat-loading-older">
                  {t('messages.chat.loadingOlder', { defaultValue: '正在加载更早消息...' })}
                </div>
              )}
              {directScroll.newNotice && (
                <NewMessagesJump notice={directScroll.newNotice} onClick={directScroll.jumpToNew} />
              )}
              {messages.isPending && selectedThread.id ? (
                <div className="gl-message-empty-soft">
                  {t('messages.private.loadingMessages', {
                    defaultValue: '正在加载聊天记录...',
                  })}
                </div>
              ) : directMessageItems.length ? (
                directMessageItems.map((item, index) => {
                  const sender = directMessageSender(item, selectedThread, currentUser);
                  const senderCreatorId = creatorChannelIdForUser(sender, selectedThread.creatorId);
                  const previous = directMessageItems[index - 1];
                  return (
                    <Fragment key={item.id}>
                      {shouldShowChatTimeSeparator(item.createdAt, previous?.createdAt) && (
                        <ChatTimeSeparator value={item.createdAt} locale={i18n.language} />
                      )}
                      <ChatMessageRow
                        sender={sender}
                        messageId={item.id}
                        body={item.body}
                        currentUserId={userId}
                        fanBadge={sender.fanBadge}
                        highlighted={directScroll.highlightedId === item.id}
                        anchorRef={
                          directScroll.newNotice?.targetId === item.id
                            ? directScroll.firstNewRef
                            : undefined
                        }
                        onAvatarClick={
                          senderCreatorId
                            ? () => navigate(`/channel/${senderCreatorId}`)
                            : undefined
                        }
                        avatarTitle={
                          senderCreatorId
                            ? t('messages.actions.openCreatorChannel', {
                                defaultValue: '进入主播频道',
                              })
                            : undefined
                        }
                      />
                    </Fragment>
                  );
                })
              ) : (
                <div className="gl-direct-empty">
                  <UserRound size={32} />
                  <strong>
                    {t('messages.private.startTitle', { defaultValue: '开始和主播私信' })}
                  </strong>
                  <span>
                    {t('messages.private.startBody', {
                      defaultValue: '在主播首次回复前，你只能先发送一条消息。',
                    })}
                  </span>
                </div>
              )}
            </div>

            <form className="gl-direct-compose" onSubmit={submit}>
              <input
                ref={directInputRef}
                value={content}
                onChange={(event) => setContent(event.target.value)}
                maxLength={1000}
                disabled={!selectedThread.canSend || sendDirect.isPending || sendThread.isPending}
                placeholder={
                  selectedThread.canSend
                    ? t('messages.private.input', { defaultValue: '输入私信内容' })
                    : t('messages.private.inputWaiting', {
                        defaultValue: '等待主播首次回复后才能继续发送',
                      })
                }
              />
              <button
                className="gl-creator-primary"
                type="submit"
                onMouseDown={(event) => event.preventDefault()}
                disabled={
                  !content.trim() ||
                  !selectedThread.canSend ||
                  sendDirect.isPending ||
                  sendThread.isPending
                }
              >
                <Send size={16} />
                {t('messages.private.send', { defaultValue: '发送' })}
              </button>
            </form>
          </>
        ) : (
          <div className="gl-direct-empty is-full">
            <MessageCircle size={38} />
            <strong>
              {conversationCount > 0
                ? t('messages.private.selectTitle', { defaultValue: '选择一个会话' })
                : t('messages.private.emptyTitle', { defaultValue: '暂无私信' })}
            </strong>
            <span>
              {conversationCount > 0
                ? t('messages.private.selectBody', {
                    defaultValue: '从左侧选择私信或粉丝团群聊后开始查看消息。',
                  })
                : t('messages.private.emptyBody', {
                    defaultValue: '进入主播频道页，点击私信图标就能发起会话。',
                  })}
            </span>
          </div>
        )}
      </section>
      <ReportDialog
        open={Boolean(reportTarget)}
        target={reportTarget}
        onOpenChange={(open) => {
          if (!open) setReportTarget(null);
        }}
      />
    </div>
  );
}

function NewMessagesJump({ notice, onClick }: { notice: ChatNewNotice; onClick: () => void }) {
  const { t } = useTranslation('pages');
  return (
    <button className="gl-chat-new-message-pill" type="button" onClick={onClick}>
      <ChevronsUp size={15} />
      {t('messages.chat.newMessages', {
        count: notice.count,
        defaultValue: '{{count}} 条新消息',
      })}
    </button>
  );
}

function ChatTimeSeparator({ value, locale }: { value: string; locale?: string }) {
  return <div className="gl-chat-time-separator">{formatChatSeparatorTime(value, locale)}</div>;
}

function mergeMessagePages<T extends { id: string }>(pages?: Array<{ items: T[] }>): T[] {
  if (!pages?.length) return [];
  const seen = new Set<string>();
  const merged: T[] = [];
  for (const page of [...pages].reverse()) {
    for (const item of page.items) {
      if (seen.has(item.id)) continue;
      seen.add(item.id);
      merged.push(item);
    }
  }
  return merged;
}

function focusInputSoon(ref: { current: HTMLInputElement | null }) {
  window.requestAnimationFrame(() => ref.current?.focus());
  window.setTimeout(() => ref.current?.focus(), 0);
}

function useChatScroll<T extends { id: string }>(
  items: T[],
  options: {
    activeKey: string;
    hasNextPage: boolean;
    isFetchingNextPage: boolean;
    entryUnread?: number;
    onEntryNoticeConsumed?: () => void;
    fetchNextPage: () => Promise<unknown>;
  },
) {
  const listRef = useRef<HTMLDivElement | null>(null);
  const firstNewRef = useRef<HTMLDivElement | null>(null);
  const itemsRef = useRef(items);
  const previousLastIdRef = useRef('');
  const nearBottomRef = useRef(true);
  const loadingOlderRef = useRef(false);
  const previousScrollHeightRef = useRef(0);
  const requestedOlderRef = useRef(false);
  const newNoticeModeRef = useRef<ChatNewNoticeMode | null>(null);
  const newNoticeRef = useRef<ChatNewNotice | null>(null);
  const pendingJumpRef = useRef(false);
  const highlightTimerRef = useRef<number | null>(null);
  const consumedEntryNoticeKeyRef = useRef('');
  const pendingLiveNewCountRef = useRef(0);
  const pendingLiveTargetIdRef = useRef('');
  const [newNotice, setNewNotice] = useState<ChatNewNotice | null>(null);
  const [highlightedId, setHighlightedId] = useState('');
  const {
    activeKey,
    entryUnread,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
    onEntryNoticeConsumed,
  } = options;
  const setChatNewNotice = useCallback((notice: ChatNewNotice | null) => {
    newNoticeModeRef.current = notice?.mode ?? null;
    newNoticeRef.current = notice;
    setNewNotice(notice);
  }, []);
  const updateChatNewNotice = useCallback(
    (updater: (current: ChatNewNotice | null) => ChatNewNotice | null) => {
      setNewNotice((current) => {
        const next = updater(current);
        newNoticeModeRef.current = next?.mode ?? null;
        newNoticeRef.current = next;
        return next;
      });
    },
    [],
  );
  const resetPendingLiveNewNotice = useCallback(() => {
    pendingLiveNewCountRef.current = 0;
    pendingLiveTargetIdRef.current = '';
  }, []);
  const highlightMessage = useCallback((id: string) => {
    if (!id) return;
    if (highlightTimerRef.current) window.clearTimeout(highlightTimerRef.current);
    setHighlightedId(id);
    highlightTimerRef.current = window.setTimeout(() => {
      setHighlightedId('');
      highlightTimerRef.current = null;
    }, 1700);
  }, []);
  const syncEntryNoticeTarget = useCallback(
    (nextItems = itemsRef.current) => {
      const notice = newNoticeRef.current;
      if (!notice || notice.mode !== 'entry') return;
      const targetId = firstUnreadTargetId(nextItems, notice.count) || notice.targetId;
      if (targetId && targetId !== notice.targetId) {
        setChatNewNotice({ ...notice, targetId });
      }
    },
    [setChatNewNotice],
  );
  const needsOlderMessagesForJump = useCallback(() => {
    const notice = newNoticeRef.current;
    return Boolean(
      notice && notice.mode === 'entry' && notice.count > itemsRef.current.length && hasNextPage,
    );
  }, [hasNextPage]);
  const fetchOlderForPendingJump = useCallback(() => {
    if (!needsOlderMessagesForJump() || isFetchingNextPage || requestedOlderRef.current) {
      return false;
    }
    requestedOlderRef.current = true;
    void fetchNextPage().finally(() => {
      requestedOlderRef.current = false;
    });
    return true;
  }, [fetchNextPage, isFetchingNextPage, needsOlderMessagesForJump]);
  const scrollToNewTarget = useCallback(() => {
    const notice = newNoticeRef.current;
    if (!notice) return;
    syncEntryNoticeTarget();
    const targetId = newNoticeRef.current?.targetId || notice.targetId;
    window.requestAnimationFrame(() => {
      const targetNode =
        listRef.current?.querySelector<HTMLElement>(`[data-chat-message-id="${targetId}"]`) ??
        firstNewRef.current;
      targetNode?.scrollIntoView({ block: 'start', behavior: 'smooth' });
      highlightMessage(targetId);
      pendingJumpRef.current = false;
      resetPendingLiveNewNotice();
      setChatNewNotice(null);
    });
  }, [highlightMessage, resetPendingLiveNewNotice, setChatNewNotice, syncEntryNoticeTarget]);
  const continuePendingJump = useCallback(
    (nextItems = itemsRef.current) => {
      if (!pendingJumpRef.current) return;
      syncEntryNoticeTarget(nextItems);
      if (needsOlderMessagesForJump()) {
        void fetchOlderForPendingJump();
        return;
      }
      scrollToNewTarget();
    },
    [fetchOlderForPendingJump, needsOlderMessagesForJump, scrollToNewTarget, syncEntryNoticeTarget],
  );
  const showEntryNoticeIfNeeded = useCallback(
    (nextItems = itemsRef.current) => {
      const node = listRef.current;
      const lastId = nextItems.at(-1)?.id ?? '';
      const normalizedEntryUnread = Math.max(0, entryUnread ?? 0);
      if (!node || !lastId || normalizedEntryUnread <= 0) return false;

      const noticeKey = `${activeKey}:${normalizedEntryUnread}:${lastId}`;
      if (consumedEntryNoticeKeyRef.current === noticeKey) return false;

      const overflowing = node.scrollHeight > node.clientHeight + 12;
      if (overflowing && normalizedEntryUnread >= CHAT_NEW_MESSAGE_NOTICE_MIN_COUNT) {
        setChatNewNotice({
          count: normalizedEntryUnread,
          targetId: firstUnreadTargetId(nextItems, normalizedEntryUnread) || lastId,
          mode: 'entry',
        });
      } else {
        setChatNewNotice(null);
      }
      consumedEntryNoticeKeyRef.current = noticeKey;
      onEntryNoticeConsumed?.();
      return true;
    },
    [activeKey, entryUnread, onEntryNoticeConsumed, setChatNewNotice],
  );

  useEffect(() => {
    previousLastIdRef.current = '';
    nearBottomRef.current = true;
    loadingOlderRef.current = false;
    previousScrollHeightRef.current = 0;
    requestedOlderRef.current = false;
    pendingJumpRef.current = false;
    consumedEntryNoticeKeyRef.current = '';
    resetPendingLiveNewNotice();
    setHighlightedId('');
    setChatNewNotice(null);
  }, [activeKey, resetPendingLiveNewNotice, setChatNewNotice]);

  useEffect(() => {
    return () => {
      if (highlightTimerRef.current) window.clearTimeout(highlightTimerRef.current);
    };
  }, []);

  useEffect(() => {
    if (!isFetchingNextPage) requestedOlderRef.current = false;
  }, [isFetchingNextPage]);

  const onScroll = () => {
    const node = listRef.current;
    if (!node) return;
    nearBottomRef.current = distanceFromBottom(node) < 88;
    if (nearBottomRef.current && newNoticeModeRef.current !== 'entry') {
      resetPendingLiveNewNotice();
      setChatNewNotice(null);
    }
    if (node.scrollTop <= 72 && hasNextPage && !isFetchingNextPage && !requestedOlderRef.current) {
      requestedOlderRef.current = true;
      loadingOlderRef.current = true;
      previousScrollHeightRef.current = node.scrollHeight;
      void fetchNextPage().finally(() => {
        requestedOlderRef.current = false;
      });
    }
  };

  useEffect(() => {
    itemsRef.current = items;
    const node = listRef.current;
    const lastId = items.at(-1)?.id ?? '';
    if (!node || !lastId) {
      previousLastIdRef.current = lastId;
      resetPendingLiveNewNotice();
      setChatNewNotice(null);
      return;
    }

    if (loadingOlderRef.current) {
      const previousHeight = previousScrollHeightRef.current;
      window.requestAnimationFrame(() => {
        const nextNode = listRef.current;
        if (nextNode && previousHeight > 0) {
          nextNode.scrollTop = nextNode.scrollHeight - previousHeight + nextNode.scrollTop;
        }
      });
      loadingOlderRef.current = false;
      previousScrollHeightRef.current = 0;
      previousLastIdRef.current = lastId;
      syncEntryNoticeTarget(items);
      continuePendingJump(items);
      return;
    }

    const previousLastId = previousLastIdRef.current;
    if (!previousLastId) {
      showEntryNoticeIfNeeded(items);
      window.requestAnimationFrame(() => {
        const nextNode = listRef.current;
        if (nextNode) nextNode.scrollTop = nextNode.scrollHeight;
      });
      previousLastIdRef.current = lastId;
      return;
    }

    if (lastId !== previousLastId) {
      const previousIndex = items.findIndex((item) => item.id === previousLastId);
      const addedItems = previousIndex >= 0 ? items.slice(previousIndex + 1) : items.slice(-1);
      if (nearBottomRef.current || distanceFromBottom(node) < 120) {
        window.requestAnimationFrame(() => {
          const nextNode = listRef.current;
          if (nextNode) nextNode.scrollTop = nextNode.scrollHeight;
        });
        if (newNoticeModeRef.current !== 'entry') {
          resetPendingLiveNewNotice();
          setChatNewNotice(null);
        }
      } else if (addedItems.length) {
        updateChatNewNotice((current) => {
          const targetId =
            current?.mode === 'live'
              ? current.targetId
              : pendingLiveTargetIdRef.current || addedItems[0].id;
          const nextCount =
            (current?.mode === 'live' ? current.count : pendingLiveNewCountRef.current) +
            addedItems.length;
          pendingLiveTargetIdRef.current = targetId;
          pendingLiveNewCountRef.current = nextCount;
          if (nextCount < CHAT_NEW_MESSAGE_NOTICE_MIN_COUNT) return null;
          return {
            count: nextCount,
            targetId,
            mode: 'live',
          };
        });
      }
    }
    syncEntryNoticeTarget(items);
    continuePendingJump(items);
    previousLastIdRef.current = lastId;
  }, [
    activeKey,
    continuePendingJump,
    items,
    resetPendingLiveNewNotice,
    setChatNewNotice,
    showEntryNoticeIfNeeded,
    syncEntryNoticeTarget,
    updateChatNewNotice,
  ]);

  useEffect(() => {
    if (!previousLastIdRef.current || Math.max(0, entryUnread ?? 0) <= 0) return;
    window.requestAnimationFrame(() => {
      showEntryNoticeIfNeeded(itemsRef.current);
    });
  }, [activeKey, entryUnread, showEntryNoticeIfNeeded]);

  const jumpToNew = () => {
    if (!newNoticeRef.current) return;
    pendingJumpRef.current = true;
    if (needsOlderMessagesForJump()) {
      void fetchOlderForPendingJump();
      return;
    }
    scrollToNewTarget();
  };

  return { listRef, firstNewRef, highlightedId, newNotice, onScroll, jumpToNew };
}

function distanceFromBottom(node: HTMLElement) {
  return node.scrollHeight - node.scrollTop - node.clientHeight;
}

function firstUnreadTargetId<T extends { id: string }>(items: T[], unreadCount: number): string {
  if (!items.length) return '';
  const firstUnreadIndex = Math.max(0, items.length - unreadCount);
  return items[firstUnreadIndex]?.id ?? items[0].id;
}

function ThreadButton({
  thread,
  active,
  onClick,
}: {
  thread: DirectThread;
  active: boolean;
  onClick: () => void;
}) {
  const { t } = useTranslation('pages');
  return (
    <button
      type="button"
      className={cn('gl-direct-thread', active && 'is-active')}
      onClick={onClick}
    >
      <Avatar name={thread.peer.name} src={thread.peer.avatar} size={44} />
      <span>
        <strong>
          {thread.peer.name}
          {thread.peer.verified && <VerifiedBadge size={12} />}
        </strong>
        <small>
          {thread.lastMessagePreview ||
            t('messages.private.threadEmpty', { defaultValue: '还没有聊天记录' })}
        </small>
      </span>
      {thread.unread > 0 && <em>{thread.unread > 99 ? '99+' : thread.unread}</em>}
    </button>
  );
}

function FanGroupThreadButton({
  group,
  active,
  onClick,
}: {
  group: FanGroup;
  active: boolean;
  onClick: () => void;
}) {
  const { t } = useTranslation('pages');
  const previewMembers = group.members.slice(0, 3);
  return (
    <button
      type="button"
      className={cn('gl-direct-thread gl-fan-chat-thread', active && 'is-active')}
      onClick={onClick}
    >
      <span className="gl-fan-chat-avatars">
        {previewMembers.length ? (
          previewMembers.map((member) => (
            <Avatar
              key={member.user.id}
              name={member.user.name}
              src={member.user.avatar}
              size={26}
            />
          ))
        ) : (
          <Users size={22} />
        )}
      </span>
      <span>
        <strong>{group.name}</strong>
        <small>
          {t('messages.fanGroupChat.memberCount', {
            count: group.memberCount,
            defaultValue: '{{count}}/200 人',
          })}
          {' · '}
          {t('messages.private.fanGroups', { defaultValue: '粉丝团群聊' })}
        </small>
      </span>
      {group.unread > 0 && <em>{group.unread > 99 ? '99+' : group.unread}</em>}
    </button>
  );
}

function FanGroupChatView({
  group,
  userId,
  entryUnread,
  onEntryNoticeConsumed,
}: {
  group: FanGroup;
  userId: string;
  entryUnread: number;
  onEntryNoticeConsumed: () => void;
}) {
  const navigate = useNavigate();
  const { t, i18n } = useTranslation('pages');
  const owner = group.members.find((member) => member.role === 'owner');
  const currentMember = group.members.find((member) => member.user.id === userId);
  const [content, setContent] = useState('');
  const [muteTarget, setMuteTarget] = useState<FanGroupMember | null>(null);
  const groupInputRef = useRef<HTMLInputElement | null>(null);
  const messages = useFanGroupMessages(
    group.id,
    Boolean(group.id) && !currentMember?.kicked,
    CHAT_PAGE_SIZE,
  );
  const groupMessageItems = useMemo(
    () => mergeMessagePages(messages.data?.pages),
    [messages.data?.pages],
  );
  const groupScroll = useChatScroll(groupMessageItems, {
    activeKey: group.id ? `fan:${group.id}` : 'fan:none',
    hasNextPage: Boolean(messages.hasNextPage),
    isFetchingNextPage: messages.isFetchingNextPage,
    entryUnread: Math.max(entryUnread, group.unread ?? 0),
    onEntryNoticeConsumed,
    fetchNextPage: () => messages.fetchNextPage(),
  });
  const sendMessage = useSendFanGroupMessage(group.id);
  const updateMember = useUpdateFanGroupMember();
  const requestRejoinMutation = useRequestFanGroupRejoin();
  const canSend = !currentMember?.muted && !currentMember?.kicked;
  const canModerate = currentMember?.role === 'owner' || currentMember?.role === 'admin';
  const memberById = useMemo(
    () => new Map(group.members.map((member) => [member.user.id, member])),
    [group.members],
  );

  const openMemberMuteDialog = (member: FanGroupMember | undefined) => {
    if (!member || !canManageFanGroupMember(currentMember, member, userId)) return;
    setMuteTarget(member);
  };

  const updateMemberMute = (member: FanGroupMember, muteMinutes: number) => {
    updateMember.mutate(
      { groupId: group.id, userId: member.user.id, muteMinutes },
      {
        onSuccess: () => {
          setMuteTarget(null);
          toast.success(
            muteMinutes > 0
              ? t('messages.fanGroupChat.muteSuccess', {
                  minutes: muteMinutes,
                  defaultValue: '已禁言 {{minutes}} 分钟',
                })
              : t('messages.fanGroupChat.unmuteSuccess', { defaultValue: '已解除禁言' }),
          );
        },
        onError: (err) =>
          toast.error(
            err.message ||
              t('messages.fanGroupChat.updateFailed', { defaultValue: '更新群成员失败。' }),
          ),
      },
    );
  };

  const submitRejoinRequest = () => {
    requestRejoinMutation.mutate(group.id, {
      onSuccess: () =>
        toast.success(
          t('messages.fanGroupChat.requestSuccess', {
            defaultValue: '已提交重新加入申请，等待主播审批。',
          }),
        ),
      onError: (err) => toast.error(fanGroupMessageError(err, t)),
    });
  };

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const text = content.trim();
    if (!text || !canSend) return;
    sendMessage.mutate(text, {
      onSuccess: () => {
        setContent('');
        focusInputSoon(groupInputRef);
      },
      onError: (err) => toast.error(fanGroupMessageError(err, t)),
    });
  };

  return (
    <>
      <div className="gl-direct-chat-head">
        {owner ? (
          <CreatorAvatarButton user={owner.user} creatorId={group.creatorId} enabled />
        ) : (
          <span className="gl-fan-chat-head-icon">
            <Users size={22} />
          </span>
        )}
        <div className="gl-direct-chat-meta">
          <strong>{group.name}</strong>
          <span>
            {t('messages.fanGroupChat.memberCount', {
              count: group.memberCount,
              defaultValue: '{{count}}/200 人',
            })}
            {owner
              ? t('messages.fanGroupChat.ownerSuffix', {
                  name: owner.user.name,
                  defaultValue: ' · 群主 {{name}}',
                })
              : ''}
          </span>
        </div>
      </div>
      <div
        className="gl-direct-message-list"
        ref={groupScroll.listRef}
        onScroll={groupScroll.onScroll}
      >
        {messages.isFetchingNextPage && !currentMember?.kicked && (
          <div className="gl-chat-loading-older">
            {t('messages.chat.loadingOlder', { defaultValue: '正在加载更早消息...' })}
          </div>
        )}
        {groupScroll.newNotice && !currentMember?.kicked && (
          <NewMessagesJump notice={groupScroll.newNotice} onClick={groupScroll.jumpToNew} />
        )}
        {currentMember?.kicked ? (
          <FanGroupKickedState
            member={currentMember}
            pending={requestRejoinMutation.isPending}
            onRequest={submitRejoinRequest}
          />
        ) : messages.isPending ? (
          <div className="gl-message-empty-soft">
            {t('messages.fanGroupChat.loading', { defaultValue: '正在加载群聊...' })}
          </div>
        ) : groupMessageItems.length ? (
          groupMessageItems.map((item, index) => {
            const member = memberById.get(item.sender.id);
            const previous = groupMessageItems[index - 1];
            const isCreatorMessage = item.sender.id === group.creatorId || item.role === 'owner';
            const canManageMember =
              canModerate && canManageFanGroupMember(currentMember, member, userId);
            const memberCreatorId = creatorChannelIdForUser(item.sender, group.creatorId);
            const avatarActions: ChatAvatarAction[] = [];
            if (memberCreatorId) {
              avatarActions.push({
                key: 'channel',
                label: t('messages.actions.openCreatorChannel', { defaultValue: '进入主播频道' }),
                icon: <UserRound size={15} />,
                onSelect: () => navigate(`/channel/${memberCreatorId}`),
              });
            }
            if (canManageMember) {
              avatarActions.push({
                key: 'mute',
                label: member?.muted
                  ? t('messages.fanGroupChat.avatarUnmute', { defaultValue: '解除禁言' })
                  : t('messages.fanGroupChat.avatarMute', { defaultValue: '禁言该用户' }),
                icon: <Ban size={15} />,
                danger: !member?.muted,
                onSelect: () => openMemberMuteDialog(member),
              });
            }
            return (
              <Fragment key={item.id}>
                {shouldShowChatTimeSeparator(item.createdAt, previous?.createdAt) && (
                  <ChatTimeSeparator value={item.createdAt} locale={i18n.language} />
                )}
                <ChatMessageRow
                  sender={item.sender}
                  messageId={item.id}
                  body={item.body}
                  currentUserId={userId}
                  role={item.role}
                  fanBadge={item.fanBadge ?? item.sender.fanBadge}
                  muted={member?.muted}
                  canManage={canManageMember}
                  highlighted={groupScroll.highlightedId === item.id}
                  anchorRef={
                    groupScroll.newNotice?.targetId === item.id
                      ? groupScroll.firstNewRef
                      : undefined
                  }
                  avatarActions={avatarActions}
                  avatarTitle={
                    avatarActions.length
                      ? item.sender.name
                      : isCreatorMessage
                        ? t('messages.actions.openCreatorChannel', { defaultValue: '进入主播频道' })
                        : undefined
                  }
                  onAvatarClick={
                    !avatarActions.length && isCreatorMessage
                      ? () => navigate(`/channel/${group.creatorId}`)
                      : undefined
                  }
                />
              </Fragment>
            );
          })
        ) : (
          <div className="gl-direct-empty">
            <Users size={32} />
            <strong>{t('messages.fanGroupChat.emptyTitle', { defaultValue: '粉丝团群聊' })}</strong>
            <span>
              {t('messages.fanGroupChat.emptyBody', {
                defaultValue: '所有加入粉丝团的成员会显示在这个群里。',
              })}
            </span>
          </div>
        )}
      </div>
      {!currentMember?.kicked && (
        <form className="gl-direct-compose" onSubmit={submit}>
          <input
            ref={groupInputRef}
            value={content}
            onChange={(event) => setContent(event.target.value)}
            maxLength={1000}
            disabled={!canSend || sendMessage.isPending}
            placeholder={
              canSend
                ? t('messages.fanGroupChat.input', { defaultValue: '输入群聊内容' })
                : t('messages.fanGroupChat.mutedInput', {
                    defaultValue: '你已被禁言，暂时不能发言',
                  })
            }
          />
          <button
            className="gl-creator-primary"
            type="submit"
            onMouseDown={(event) => event.preventDefault()}
            disabled={!content.trim() || !canSend || sendMessage.isPending}
          >
            <Send size={16} />
            {t('messages.private.send', { defaultValue: '发送' })}
          </button>
        </form>
      )}
      <FanGroupMuteDialog
        target={muteTarget}
        actor={currentMember}
        currentUserId={userId}
        pending={updateMember.isPending}
        onOpenChange={(open) => {
          if (!open) setMuteTarget(null);
        }}
        onMute={(duration) => {
          if (muteTarget) updateMemberMute(muteTarget, duration);
        }}
        onUnmute={() => {
          if (muteTarget) updateMemberMute(muteTarget, 0);
        }}
      />
    </>
  );
}

function ChatMessageRow({
  sender,
  messageId,
  body,
  currentUserId,
  role,
  fanBadge,
  muted,
  canManage,
  highlighted,
  anchorRef,
  onAvatarClick,
  avatarTitle,
  avatarActions,
}: {
  sender: MessageUser;
  messageId: string;
  body: string;
  currentUserId: string;
  role?: string;
  fanBadge?: MessageFanBadge;
  muted?: boolean;
  canManage?: boolean;
  highlighted?: boolean;
  anchorRef?: Ref<HTMLDivElement>;
  onAvatarClick?: () => void;
  avatarTitle?: string;
  avatarActions?: ChatAvatarAction[];
}) {
  const { t } = useTranslation('pages');
  const isMine = sender.id === currentUserId;
  const hasAvatarActions = Boolean(avatarActions?.length);
  const actionable = Boolean(onAvatarClick || hasAvatarActions);
  const avatarButton = (
    <button
      type="button"
      className={cn(
        'gl-chat-message-avatar-btn',
        actionable && 'is-actionable',
        muted && 'is-muted',
      )}
      disabled={!actionable}
      onClick={hasAvatarActions ? undefined : onAvatarClick}
      title={
        avatarTitle ||
        (canManage
          ? muted
            ? t('messages.fanGroupChat.avatarUnmute', { defaultValue: '解除禁言' })
            : t('messages.fanGroupChat.avatarMute', { defaultValue: '禁言该用户' })
          : sender.name)
      }
    >
      <Avatar name={sender.name} src={sender.avatar} size={38} />
      {muted && <span>{t('messages.fanGroupChat.mutedMark', { defaultValue: '禁' })}</span>}
    </button>
  );
  const avatar = hasAvatarActions ? (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>{avatarButton}</DropdownMenuTrigger>
      <DropdownMenuContent align={isMine ? 'end' : 'start'} className="w-40">
        {avatarActions?.map((action) => (
          <DropdownMenuItem
            key={action.key}
            className={action.danger ? 'gl-menu-danger' : undefined}
            onSelect={action.onSelect}
          >
            {action.icon}
            {action.label}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  ) : (
    avatarButton
  );

  return (
    <div
      ref={anchorRef}
      data-chat-message-id={messageId}
      className={cn('gl-chat-message-row', isMine && 'is-me', highlighted && 'is-highlighted')}
    >
      {!isMine && avatar}
      <div className="gl-chat-message-stack">
        <div className="gl-chat-message-meta">
          <strong>{sender.name}</strong>
          <FanBadgePill badge={fanBadge ?? sender.fanBadge} />
          <RoleBadge role={role} />
        </div>
        <div className="gl-chat-message-bubble">
          <p>{body}</p>
        </div>
      </div>
      {isMine && avatar}
    </div>
  );
}

function FanBadgePill({ badge }: { badge?: MessageFanBadge }) {
  if (!badge || badge.level <= 0) return null;
  return (
    <span className={cn('gl-chat-fan-badge', fanBadgeToneClass(badge.level))}>
      LV{Math.min(99, Math.max(1, Math.floor(badge.level)))}
    </span>
  );
}

function RoleBadge({ role }: { role?: string }) {
  const { t } = useTranslation('pages');
  if (role === 'owner') {
    return (
      <span className="gl-chat-role-badge is-owner">
        <Crown size={10} />
        {t('messages.fanGroupChat.owner', { defaultValue: '群主' })}
      </span>
    );
  }
  if (role === 'admin') {
    return (
      <span className="gl-chat-role-badge is-admin">
        <ShieldCheck size={10} />
        {t('messages.fanGroupChat.admin', { defaultValue: '管理员' })}
      </span>
    );
  }
  return null;
}

function CreatorAvatarButton({
  user,
  creatorId,
  enabled,
}: {
  user: MessageUser;
  creatorId: string;
  enabled?: boolean;
}) {
  const navigate = useNavigate();
  const { t } = useTranslation('pages');
  if (!enabled || !creatorId) {
    return <Avatar name={user.name} src={user.avatar} size={42} />;
  }
  return (
    <button
      type="button"
      className="gl-creator-avatar-link"
      onClick={() => navigate(`/channel/${creatorId}`)}
      title={t('messages.actions.openCreatorChannel', { defaultValue: '进入主播频道' })}
    >
      <Avatar name={user.name} src={user.avatar} size={42} />
    </button>
  );
}

function FollowRequiredDirectState({ onOpenChannel }: { onOpenChannel: () => void }) {
  const { t } = useTranslation('pages');
  return (
    <div className="gl-direct-empty is-full">
      <MessageCircle size={38} />
      <strong>
        {t('messages.private.followRequiredTitle', { defaultValue: '关注主播后才能私信' })}
      </strong>
      <span>
        {t('messages.private.followRequiredBody', {
          defaultValue: '先进入主播频道关注 TA，然后就可以从这里发起私信。',
        })}
      </span>
      <button type="button" className="gl-creator-primary" onClick={onOpenChannel}>
        {t('messages.private.openChannel', { defaultValue: '去主播频道' })}
      </button>
    </div>
  );
}

function FanGroupMuteDialog({
  target,
  actor,
  currentUserId,
  pending,
  onOpenChange,
  onMute,
  onUnmute,
}: {
  target: FanGroupMember | null;
  actor: FanGroupMember | undefined;
  currentUserId: string;
  pending: boolean;
  onOpenChange: (open: boolean) => void;
  onMute: (duration: number) => void;
  onUnmute: () => void;
}) {
  const { t } = useTranslation('pages');
  const blocked = !canManageFanGroupMember(actor, target ?? undefined, currentUserId);
  const durations = [5, 10, 30, 60];
  const muted = Boolean(target?.muted);
  const statusText = blocked
    ? t('messages.fanGroupChat.muteBlocked', {
        defaultValue: '群主、管理员或你自己不能被禁言。',
      })
    : muted
      ? t('messages.fanGroupChat.alreadyMuted', {
          defaultValue: '该成员当前已被禁言，可解除禁言。',
        })
      : t('messages.fanGroupChat.mutePrompt', { defaultValue: '选择禁言时长' });

  return (
    <Dialog open={Boolean(target)} onOpenChange={onOpenChange}>
      <DialogContent className="gl-mute-dialog">
        <DialogTitle>
          {muted
            ? t('messages.fanGroupChat.unmuteTitle', { defaultValue: '解除禁言' })
            : t('messages.fanGroupChat.muteTitle', { defaultValue: '禁言用户' })}
        </DialogTitle>
        <div className="gl-mute-target">
          <ShieldCheck size={22} />
          <div>
            <strong>{target?.user.name ?? ''}</strong>
            <span>{statusText}</span>
          </div>
        </div>
        {muted && !blocked ? (
          <button
            type="button"
            className="gl-mute-unmute-btn"
            disabled={pending}
            onClick={onUnmute}
          >
            {t('messages.fanGroupChat.unmuteAction', { defaultValue: '解除禁言' })}
          </button>
        ) : (
          <div className="gl-mute-duration-grid">
            {durations.map((duration) => (
              <button
                key={duration}
                type="button"
                disabled={blocked || pending}
                onClick={() => onMute(duration)}
              >
                {t('messages.fanGroupChat.muteMinutes', {
                  count: duration,
                  defaultValue: '{{count}} 分钟',
                })}
              </button>
            ))}
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}

function FanGroupKickedState({
  member,
  pending,
  onRequest,
}: {
  member: FanGroupMember;
  pending: boolean;
  onRequest: () => void;
}) {
  const { t } = useTranslation('pages');
  const waiting = Boolean(member.rejoinRequestedAt);
  return (
    <div className="gl-direct-empty is-full gl-fan-chat-kicked">
      <Users size={34} />
      <strong>
        {t('messages.fanGroupChat.kickedTitle', { defaultValue: '你已被踢出粉丝群' })}
      </strong>
      <span>
        {waiting
          ? t('messages.fanGroupChat.kickedWaiting', {
              defaultValue: '重新加入申请已提交，等待主播在群聊管理中审批。',
            })
          : member.rejoinRejectedAt
            ? t('messages.fanGroupChat.kickedRejected', {
                defaultValue: '你的重新加入申请已被驳回，可以再次提交申请。',
              })
            : t('messages.fanGroupChat.kickedBody', {
                defaultValue: '你暂时不能查看群消息，需要重新申请加入。',
              })}
      </span>
      <button
        type="button"
        className="gl-creator-primary"
        disabled={waiting || pending}
        onClick={onRequest}
      >
        {waiting
          ? t('messages.fanGroupChat.waitingApproval', { defaultValue: '等待审批' })
          : pending
            ? t('messages.fanGroupChat.submitting', { defaultValue: '提交中...' })
            : t('messages.fanGroupChat.requestRejoin', { defaultValue: '申请重新加入' })}
      </button>
    </div>
  );
}

function directMessageSender(
  message: DirectMessage,
  thread: DirectThread,
  currentUser: MessageUser,
): MessageUser {
  if (message.sender) return message.sender;
  if (message.senderId === thread.peer.id) return thread.peer;
  return currentUser;
}

function creatorChannelIdForUser(user: MessageUser, threadCreatorId?: string): string {
  if (user.id && user.id === threadCreatorId) return user.id;
  if (user.id && user.livePermissionStatus === 'approved') return user.id;
  return '';
}

function canManageFanGroupMember(
  actor: FanGroupMember | undefined,
  target: FanGroupMember | undefined,
  currentUserId: string,
) {
  if (!actor || !target || target.user.id === currentUserId || target.kicked) return false;
  if (target.role === 'owner') return false;
  if (actor.role === 'owner') return true;
  return actor.role === 'admin' && target.role === 'member';
}

function fanGroupMessageError(err: Error, t: Translate): string {
  const reason = apiErrorReason(err);
  if (reason === 'fan_group_muted') {
    return t('messages.fanGroupChat.errorMuted', {
      defaultValue: '你已被禁言，暂时不能在群聊发言。',
    });
  }
  if (reason === 'fan_group_member_required') {
    return t('messages.fanGroupChat.errorMemberRequired', {
      defaultValue: '你不在这个粉丝团群聊里。',
    });
  }
  return (
    err.message ||
    t('messages.fanGroupChat.errorSendFailed', { defaultValue: '群聊消息发送失败。' })
  );
}

function NotificationPanel({ title, box, empty }: { title: string; box: string; empty: string }) {
  const navigate = useNavigate();
  const { t, i18n } = useTranslation('pages');
  const [page, setPage] = useState(1);
  const pageSize = 20;
  const notifications = useNotifications(true, page, pageSize, box);
  const markRead = useMarkNotificationRead();
  const markAllRead = useMarkAllNotificationsRead(box);
  const items = notifications.data?.items ?? [];
  const unread = notifications.data?.unread ?? 0;
  const total = notifications.data?.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(total / pageSize));

  useEffect(() => {
    setPage(1);
  }, [box]);

  useEffect(() => {
    if (page > totalPages) setPage(totalPages);
  }, [page, totalPages]);

  const openItem = (item: NotificationItem) => {
    if (!item.readAt) markRead.mutate(item.id);
    if (item.link) navigate(item.link);
  };

  return (
    <section className="gl-message-card">
      <div className="gl-message-card-head">
        <h1>{title}</h1>
        <div className="gl-message-card-actions">
          <span>
            {t('messages.notifications.unread', {
              count: unread,
              defaultValue: '{{count}} 条未读',
            })}
          </span>
          {unread > 0 && (
            <button
              type="button"
              className="gl-message-read-all"
              disabled={markAllRead.isPending}
              onClick={() => markAllRead.mutate()}
            >
              {t('messages.notifications.markAllRead', { defaultValue: '全部已读' })}
            </button>
          )}
        </div>
      </div>
      {notifications.isPending ? (
        <div className="gl-message-empty-soft">
          {t('messages.notifications.loading', { defaultValue: '正在加载通知...' })}
        </div>
      ) : items.length ? (
        <div className="gl-message-notice-list">
          {items.map((item) => (
            <button
              type="button"
              key={item.id}
              className={cn('gl-message-notice', !item.readAt && 'is-unread')}
              onClick={() => openItem(item)}
            >
              <Avatar name={item.actorName || item.title} src={item.actorAvatar} size={42} />
              <span>
                <strong>{item.title}</strong>
                {item.actorName && <small>@{item.actorUsername || item.actorName}</small>}
                {item.body && <p>{item.body}</p>}
                <time>{formatMessageTime(item.createdAt, i18n.language)}</time>
              </span>
            </button>
          ))}
        </div>
      ) : (
        <div className="gl-direct-empty is-full">
          <Bell size={34} />
          <strong>{empty}</strong>
        </div>
      )}
      {total > pageSize && (
        <div className="gl-message-pagination">
          <span>
            {t('messages.notifications.pageStatus', {
              total,
              page,
              totalPages,
              defaultValue: '共 {{total}} 条 · 第 {{page}} / {{totalPages}} 页',
            })}
          </span>
          <div>
            <button
              type="button"
              disabled={page <= 1 || notifications.isFetching}
              onClick={() => setPage((value) => Math.max(1, value - 1))}
            >
              {t('messages.notifications.prevPage', { defaultValue: '上一页' })}
            </button>
            <button
              type="button"
              disabled={page >= totalPages || notifications.isFetching}
              onClick={() => setPage((value) => Math.min(totalPages, value + 1))}
            >
              {t('messages.notifications.nextPage', { defaultValue: '下一页' })}
            </button>
          </div>
        </div>
      )}
    </section>
  );
}

function MessageSettingsPanel() {
  const { t } = useTranslation('pages');
  const pref = useMessagePreference(true);
  const update = useUpdateMessagePreference();
  const [draft, setDraft] = useState<MessagePreference>(defaultPreference);

  useEffect(() => {
    if (pref.data) setDraft(pref.data);
  }, [pref.data]);

  const patch = (value: Partial<MessagePreference>) => {
    const next = { ...draft, ...value };
    setDraft(next);
    update.mutate(next, {
      onSuccess: () =>
        toast.success(t('messages.settings.saved', { defaultValue: '消息设置已保存' })),
      onError: (err) => {
        if (pref.data) setDraft(pref.data);
        toast.error(
          err.message || t('messages.settings.saveFailed', { defaultValue: '消息设置保存失败。' }),
        );
      },
    });
  };

  return (
    <section className="gl-message-card gl-message-settings-panel">
      <div className="gl-message-card-head">
        <h1>{t('messages.sections.settings', { defaultValue: '消息设置' })}</h1>
        <span>
          {t('messages.settings.subtitle', { defaultValue: '提醒、回复、点赞和收纳规则' })}
        </span>
      </div>
      <SettingSwitch
        title={t('messages.settings.reminder', { defaultValue: '消息提醒' })}
        sub={t('messages.settings.reminderSub', { defaultValue: '关闭后，消息将不再进行提醒' })}
        checked={draft.messageReminderEnabled}
        onChange={(messageReminderEnabled) => patch({ messageReminderEnabled })}
      />
      <SettingRadioGroup
        title={t('messages.settings.replies', { defaultValue: '回复我的消息提醒' })}
        sub={t('messages.settings.repliesSub', { defaultValue: '接收谁的评论消息提醒' })}
        value={draft.replyReminderScope}
        onChange={(replyReminderScope) => patch({ replyReminderScope })}
      />
      <SettingRadioGroup
        title={t('messages.settings.mentions', { defaultValue: '@我的消息提醒' })}
        sub={t('messages.settings.mentionsSub', { defaultValue: '接收谁的 @ 消息提醒' })}
        value={draft.mentionReminderScope}
        onChange={(mentionReminderScope) => patch({ mentionReminderScope })}
      />
      <SettingSwitch
        title={t('messages.settings.likes', { defaultValue: '收到的赞消息提醒' })}
        checked={draft.likeReminderEnabled}
        onChange={(likeReminderEnabled) => patch({ likeReminderEnabled })}
      />
      <SettingSwitch
        title={t('messages.settings.fold', { defaultValue: '收起未关注人消息' })}
        sub={t('messages.settings.foldSub', {
          defaultValue: '开启后，未关注人消息将被折叠起来',
        })}
        checked={draft.foldUnfollowedMessages}
        onChange={(foldUnfollowedMessages) => patch({ foldUnfollowedMessages })}
      />
    </section>
  );
}

function SettingSwitch({
  title,
  sub,
  checked,
  onChange,
}: {
  title: string;
  sub?: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
}) {
  const { t } = useTranslation('pages');
  return (
    <div className="gl-message-setting-row">
      <span>
        <strong>{title}</strong>
        {sub && <small>{sub}</small>}
      </span>
      <div className="gl-message-radio-row is-binary">
        {[
          [true, t('messages.settings.on', { defaultValue: '开启' })],
          [false, t('messages.settings.off', { defaultValue: '关闭' })],
        ].map(([value, label]) => (
          <button
            key={String(value)}
            type="button"
            className={cn('gl-message-radio', checked === value && 'is-active')}
            onClick={() => onChange(Boolean(value))}
          >
            <i />
            {label}
          </button>
        ))}
      </div>
    </div>
  );
}

function SettingRadioGroup({
  title,
  sub,
  value,
  onChange,
}: {
  title: string;
  sub: string;
  value: string;
  onChange: (value: 'all' | 'following' | 'none') => void;
}) {
  const { t } = useTranslation('pages');
  return (
    <div className="gl-message-setting-row is-radio">
      <span>
        <strong>{title}</strong>
        <small>{sub}</small>
      </span>
      <div className="gl-message-radio-row">
        {[
          ['all', t('messages.settings.all', { defaultValue: '所有人' })],
          ['following', t('messages.settings.following', { defaultValue: '关注的人' })],
          ['none', t('messages.settings.none', { defaultValue: '不接收任何消息提醒' })],
        ].map(([id, label]) => (
          <button
            key={id}
            type="button"
            className={cn('gl-message-radio', value === id && 'is-active')}
            onClick={() => onChange(id as 'all' | 'following' | 'none')}
          >
            <i />
            {label}
          </button>
        ))}
      </div>
    </div>
  );
}

const defaultPreference: MessagePreference = {
  messageReminderEnabled: true,
  replyReminderScope: 'all',
  mentionReminderScope: 'all',
  likeReminderEnabled: true,
  foldUnfollowedMessages: false,
};

function normalizeSection(value: string | undefined): MessageSection {
  if (value === 'replies' || value === 'likes' || value === 'system' || value === 'settings') {
    return value;
  }
  return 'private';
}

function parseNoticeEntry(search: string): { groupId: string; unread: number } {
  const params = new URLSearchParams(search);
  const rawUnread = Number(params.get('unread') ?? 0);
  const unread =
    params.get('from') === 'notice' && Number.isFinite(rawUnread) ? Math.max(0, rawUnread) : 0;
  return {
    groupId: params.get('group') ?? '',
    unread,
  };
}

function shouldShowChatTimeSeparator(value: string, previousValue?: string): boolean {
  const currentTime = Date.parse(value);
  if (!Number.isFinite(currentTime)) return !previousValue;
  if (!previousValue) return true;
  const previousTime = Date.parse(previousValue);
  if (!Number.isFinite(previousTime)) return true;
  return (
    !isSameCalendarDay(new Date(currentTime), new Date(previousTime)) ||
    currentTime - previousTime >= CHAT_TIME_SEPARATOR_GAP_MS
  );
}

function formatChatSeparatorTime(value: string, locale = 'en-US'): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  const normalizedLocale = locale || 'en-US';
  const now = new Date();
  const yesterday = new Date(now);
  yesterday.setDate(now.getDate() - 1);
  const time = new Intl.DateTimeFormat(normalizedLocale, {
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(date);
  if (isSameCalendarDay(date, now)) return time;
  if (isSameCalendarDay(date, yesterday)) {
    if (normalizedLocale.startsWith('ja')) return `昨日 ${time}`;
    if (normalizedLocale.startsWith('en')) return `Yesterday ${time}`;
    return `昨天 ${time}`;
  }
  const options: Intl.DateTimeFormatOptions = {
    month: 'numeric',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  };
  if (date.getFullYear() !== now.getFullYear()) options.year = 'numeric';
  return new Intl.DateTimeFormat(normalizedLocale, options).format(date);
}

function isSameCalendarDay(left: Date, right: Date): boolean {
  return (
    left.getFullYear() === right.getFullYear() &&
    left.getMonth() === right.getMonth() &&
    left.getDate() === right.getDate()
  );
}

function formatMessageTime(value: string, locale = 'en-US'): string {
  return new Intl.DateTimeFormat(locale || 'en-US', {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(value));
}

function apiErrorReason(err: unknown): string | undefined {
  return (err as { response?: { data?: { reason?: string } } } | null | undefined)?.response?.data
    ?.reason;
}

function messageError(err: Error, t: Translate): string {
  const reason = apiErrorReason(err);
  if (reason === 'awaiting_creator_reply') {
    return t('messages.errors.awaitingCreatorReply', {
      defaultValue: '主播首次回复前，你最多只能发送一条消息。',
    });
  }
  if (reason === 'follow_required') {
    return t('messages.errors.followRequired', { defaultValue: '关注该主播后才能发送私信。' });
  }
  if (reason === 'user_blocked') {
    return t('messages.errors.userBlocked', {
      defaultValue: '你们之间存在黑名单关系，暂时无法私信。',
    });
  }
  return err.message || t('messages.errors.sendFailed', { defaultValue: '消息发送失败。' });
}
