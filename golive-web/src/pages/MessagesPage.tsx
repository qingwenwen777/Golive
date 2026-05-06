import { useEffect, useMemo, useState, type FormEvent } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import {
  Ban,
  Bell,
  BellOff,
  ChevronRight,
  Heart,
  MessageCircle,
  MoreVertical,
  Pin,
  Send,
  Settings,
  ShieldOff,
  UserRound,
} from 'lucide-react';
import { toast } from 'sonner';
import { useNotifications, useMarkNotificationRead, type NotificationItem } from '@/api/room';
import {
  useBlockUser,
  useDirectDraft,
  useDirectMessages,
  useDirectThreads,
  useMessagePreference,
  useSendDirect,
  useSendThreadMessage,
  useUpdateMessagePreference,
  useUpdateThreadOptions,
  type DirectThread,
  type MessagePreference,
} from '@/api/messages';
import { Avatar } from '@/components/Avatar';
import { VerifiedBadge } from '@/components/VerifiedBadge';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { ReportDialog, type ReportTargetDraft } from '@/features/reporting/ReportDialog';
import { cn } from '@/lib/cn';
import { useAuthModalStore } from '@/stores/useAuthModalStore';
import { useAuthStore, useIsAuthed } from '@/stores/useAuthStore';

type MessageSection = 'private' | 'replies' | 'likes' | 'system' | 'settings';

const SECTION_ITEMS: Array<{
  id: MessageSection;
  path: string;
  label: string;
  icon: typeof MessageCircle;
}> = [
  { id: 'private', path: '/messages/private', label: '我的消息', icon: MessageCircle },
  { id: 'replies', path: '/messages/replies', label: '回复我的', icon: Bell },
  { id: 'likes', path: '/messages/likes', label: '收到的赞', icon: Heart },
  { id: 'system', path: '/messages/system', label: '系统通知', icon: ShieldOff },
  { id: 'settings', path: '/messages/settings', label: '消息设置', icon: Settings },
];

export default function MessagesPage() {
  const navigate = useNavigate();
  const { section: rawSection, targetId } = useParams<{ section?: string; targetId?: string }>();
  const isAuthed = useIsAuthed();
  const openLogin = useAuthModalStore((s) => s.openLogin);
  const user = useAuthStore((s) => s.user);
  const section = normalizeSection(rawSection);
  const draftCreatorId = rawSection === 'direct' ? targetId ?? '' : '';

  useEffect(() => {
    if (!rawSection) navigate('/messages/private', { replace: true });
  }, [navigate, rawSection]);

  if (!isAuthed) {
    return (
      <div className="gl-page gl-message-page">
        <div className="gl-message-auth">
          <MessageCircle size={34} />
          <h1>登录后查看消息</h1>
          <p>私信、动态通知和系统通知都会集中在这里。</p>
          <button className="gl-creator-primary" type="button" onClick={() => openLogin()}>
            登录
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="gl-page gl-message-page">
      <div className="gl-message-shell">
        <aside className="gl-message-nav">
          <div className="gl-message-nav-head">
            <MessageCircle size={22} />
            <strong>消息</strong>
          </div>
          {SECTION_ITEMS.map((item) => {
            const Icon = item.icon;
            return (
              <button
                key={item.id}
                type="button"
                className={cn('gl-message-nav-item', section === item.id && 'is-active')}
                onClick={() => navigate(item.path)}
              >
                <Icon size={17} />
                <span>{item.label}</span>
                <ChevronRight size={15} />
              </button>
            );
          })}
        </aside>

        <main className="gl-message-main">
          {section === 'private' ? (
            <PrivateMessages userId={user?.id ?? ''} draftCreatorId={draftCreatorId} />
          ) : section === 'replies' ? (
            <NotificationPanel title="回复我的" box="reply" empty="还没有新的回复。" />
          ) : section === 'likes' ? (
            <NotificationPanel title="收到的赞" box="like" empty="你的帖子、评论或直播间收到赞后会显示在这里。" />
          ) : section === 'system' ? (
            <NotificationPanel title="系统通知" box="system" empty="暂无系统通知。" />
          ) : (
            <MessageSettingsPanel />
          )}
        </main>
      </div>
    </div>
  );
}

function PrivateMessages({ userId, draftCreatorId }: { userId: string; draftCreatorId: string }) {
  const threads = useDirectThreads(true, 1, 50);
  const draft = useDirectDraft(draftCreatorId, Boolean(draftCreatorId));
  const [selectedId, setSelectedId] = useState('');
  const [content, setContent] = useState('');
  const [reportTarget, setReportTarget] = useState<ReportTargetDraft | null>(null);
  const sendDirect = useSendDirect();
  const threadByDraft = threads.data?.items.find((item) => item.creatorId === draftCreatorId);
  const selectedThread =
    (selectedId ? threads.data?.items.find((item) => item.id === selectedId) : undefined) ??
    threadByDraft ??
    threads.data?.items[0] ??
    draft.data;
  const messages = useDirectMessages(selectedThread?.id ?? '', Boolean(selectedThread?.id));
  const sendThread = useSendThreadMessage(selectedThread?.id ?? '');
  const blockUser = useBlockUser();
  const updateOptions = useUpdateThreadOptions(selectedThread?.id ?? '');
  const sortedThreads = useMemo(
    () => [...(threads.data?.items ?? [])].sort((a, b) => Number(b.pinned) - Number(a.pinned)),
    [threads.data?.items],
  );

  useEffect(() => {
    if (!selectedId && threadByDraft?.id) setSelectedId(threadByDraft.id);
  }, [selectedId, threadByDraft?.id]);

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const text = content.trim();
    if (!text || !selectedThread) return;
    const onSuccess = () => setContent('');
    const onError = (err: Error) => toast.error(messageError(err));
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
          <h1>我的消息</h1>
          <span>{threads.data?.total ?? 0} 个会话</span>
        </div>
        {threads.isPending ? (
          <div className="gl-message-empty-soft">正在加载私信...</div>
        ) : sortedThreads.length || draft.data ? (
          <>
            {draft.data && !threadByDraft && (
              <ThreadButton
                thread={draft.data}
                active={!selectedThread?.id}
                onClick={() => setSelectedId('')}
              />
            )}
            {sortedThreads.map((thread) => (
              <ThreadButton
                key={thread.id}
                thread={thread}
                active={selectedThread?.id === thread.id}
                onClick={() => setSelectedId(thread.id)}
              />
            ))}
          </>
        ) : (
          <div className="gl-message-empty-soft">关注主播后，可以从频道页发起私信。</div>
        )}
      </aside>

      <section className="gl-direct-chat">
        {selectedThread ? (
          <>
            <div className="gl-direct-chat-head">
              <Avatar name={selectedThread.peer.name} src={selectedThread.peer.avatar} size={42} />
              <div>
                <strong>
                  {selectedThread.peer.name}
                  {selectedThread.peer.verified && <VerifiedBadge size={14} />}
                </strong>
                <span>
                  {selectedThread.awaitingReply
                    ? '等待主播回复后才能继续发送'
                    : selectedThread.muted
                      ? '已开启免打扰'
                      : '通知正常接收'}
                </span>
              </div>
              {selectedThread.id && (
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <button className="gl-message-icon-btn" type="button" aria-label="会话操作">
                      <MoreVertical size={19} />
                    </button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end" className="w-44">
                    <DropdownMenuItem
                      onSelect={() =>
                        updateOptions.mutate({ pinned: !selectedThread.pinned })
                      }
                    >
                      <Pin size={15} />
                      {selectedThread.pinned ? '取消置顶' : '置顶聊天'}
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      onSelect={() => updateOptions.mutate({ muted: !selectedThread.muted })}
                    >
                      {selectedThread.muted ? <Bell size={15} /> : <BellOff size={15} />}
                      {selectedThread.muted ? '关闭免打扰' : '开启免打扰'}
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      onSelect={() =>
                        updateOptions.mutate({ pushDisabled: !selectedThread.pushDisabled })
                      }
                    >
                      <BellOff size={15} />
                      {selectedThread.pushDisabled ? '接收推送' : '不接收推送'}
                    </DropdownMenuItem>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem onSelect={() => openReport(selectedThread)}>
                      <ShieldOff size={15} />
                      举报该用户
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      className="gl-menu-danger"
                      onSelect={() =>
                        blockUser.mutate(
                          { userId: selectedThread.peer.id, reason: 'direct_message' },
                          { onSuccess: () => toast.success('已加入黑名单') },
                        )
                      }
                    >
                      <Ban size={15} />
                      加入黑名单
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              )}
            </div>

            <div className="gl-direct-message-list">
              {messages.isPending && selectedThread.id ? (
                <div className="gl-message-empty-soft">正在加载聊天记录...</div>
              ) : messages.data?.items.length ? (
                messages.data.items.map((item) => (
                  <div
                    key={item.id}
                    className={cn('gl-direct-bubble-row', item.senderId === userId && 'is-me')}
                  >
                    <div className="gl-direct-bubble">
                      <p>{item.body}</p>
                      <time>{formatMessageTime(item.createdAt)}</time>
                    </div>
                  </div>
                ))
              ) : (
                <div className="gl-direct-empty">
                  <UserRound size={32} />
                  <strong>开始和主播私信</strong>
                  <span>在主播回复前，你只能先发送一条消息。</span>
                </div>
              )}
            </div>

            <form className="gl-direct-compose" onSubmit={submit}>
              <input
                value={content}
                onChange={(event) => setContent(event.target.value)}
                maxLength={1000}
                disabled={!selectedThread.canSend || sendDirect.isPending || sendThread.isPending}
                placeholder={
                  selectedThread.canSend ? '输入私信内容' : '等待主播回复后才能继续发送'
                }
              />
              <button
                className="gl-creator-primary"
                type="submit"
                disabled={!content.trim() || !selectedThread.canSend}
              >
                <Send size={16} />
                发送
              </button>
            </form>
          </>
        ) : (
          <div className="gl-direct-empty is-full">
            <MessageCircle size={38} />
            <strong>暂无私信</strong>
            <span>进入主播频道页，点击私信图标就能发起会话。</span>
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

function ThreadButton({
  thread,
  active,
  onClick,
}: {
  thread: DirectThread;
  active: boolean;
  onClick: () => void;
}) {
  return (
    <button type="button" className={cn('gl-direct-thread', active && 'is-active')} onClick={onClick}>
      <Avatar name={thread.peer.name} src={thread.peer.avatar} size={44} />
      <span>
        <strong>
          {thread.peer.name}
          {thread.peer.verified && <VerifiedBadge size={12} />}
        </strong>
        <small>{thread.lastMessagePreview || '还没有聊天记录'}</small>
      </span>
      {thread.unread > 0 && <em>{thread.unread > 99 ? '99+' : thread.unread}</em>}
    </button>
  );
}

function NotificationPanel({ title, box, empty }: { title: string; box: string; empty: string }) {
  const navigate = useNavigate();
  const notifications = useNotifications(true, 1, 40, box);
  const markRead = useMarkNotificationRead();
  const items = notifications.data?.items ?? [];

  const openItem = (item: NotificationItem) => {
    if (!item.readAt) markRead.mutate(item.id);
    if (item.link) navigate(item.link);
  };

  return (
    <section className="gl-message-card">
      <div className="gl-message-card-head">
        <h1>{title}</h1>
        <span>{notifications.data?.unread ?? 0} 条未读</span>
      </div>
      {notifications.isPending ? (
        <div className="gl-message-empty-soft">正在加载通知...</div>
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
                <time>{formatMessageTime(item.createdAt)}</time>
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
    </section>
  );
}

function MessageSettingsPanel() {
  const pref = useMessagePreference(true);
  const update = useUpdateMessagePreference();
  const current = pref.data ?? defaultPreference;

  const patch = (value: Partial<MessagePreference>) => {
    update.mutate(
      { ...current, ...value },
      { onSuccess: () => toast.success('消息设置已保存') },
    );
  };

  return (
    <section className="gl-message-card gl-message-settings-panel">
      <div className="gl-message-card-head">
        <h1>消息设置</h1>
        <span>提醒、回复、点赞和收纳规则</span>
      </div>
      <SettingSwitch
        title="消息提醒"
        sub="关闭后，消息将不再进行提醒"
        checked={current.messageReminderEnabled}
        onChange={() => patch({ messageReminderEnabled: !current.messageReminderEnabled })}
      />
      <SettingRadioGroup
        title="回复我的消息提醒"
        sub="接收谁的评论消息提醒"
        value={current.replyReminderScope}
        onChange={(replyReminderScope) => patch({ replyReminderScope })}
      />
      <SettingRadioGroup
        title="@我的消息提醒"
        sub="接收谁的 @ 消息提醒"
        value={current.mentionReminderScope}
        onChange={(mentionReminderScope) => patch({ mentionReminderScope })}
      />
      <SettingSwitch
        title="收到的赞消息提醒"
        checked={current.likeReminderEnabled}
        onChange={() => patch({ likeReminderEnabled: !current.likeReminderEnabled })}
      />
      <SettingSwitch
        title="收起未关注人消息"
        sub="开启后，未关注人消息将被折叠起来"
        checked={current.foldUnfollowedMessages}
        onChange={() => patch({ foldUnfollowedMessages: !current.foldUnfollowedMessages })}
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
  onChange: () => void;
}) {
  return (
    <div className="gl-message-setting-row">
      <span>
        <strong>{title}</strong>
        {sub && <small>{sub}</small>}
      </span>
      <button
        type="button"
        role="switch"
        aria-checked={checked}
        className={cn('gl-yt-switch', checked && 'is-on')}
        onClick={onChange}
      >
        <span className="gl-yt-switch-thumb" />
      </button>
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
  return (
    <div className="gl-message-setting-row is-radio">
      <span>
        <strong>{title}</strong>
        <small>{sub}</small>
      </span>
      <div className="gl-message-radio-row">
        {[
          ['all', '所有人'],
          ['following', '关注的人'],
          ['none', '不接收任何消息提醒'],
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

function formatMessageTime(value: string): string {
  return new Intl.DateTimeFormat('zh-CN', {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(value));
}

function messageError(err: Error): string {
  const reason = (err as Error & { response?: { data?: { reason?: string; message?: string } } })
    .response?.data?.reason;
  if (reason === 'awaiting_creator_reply') return '主播回复前，你最多只能发送一条消息。';
  if (reason === 'follow_required') return '关注该主播后才能发送私信。';
  if (reason === 'user_blocked') return '你们之间存在黑名单关系，暂时无法私信。';
  return err.message || '消息发送失败。';
}
