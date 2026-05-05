import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';

type LegalKind = 'privacy' | 'terms';

type LegalCopy = {
  title: string;
  updated: string;
  sections: Array<{ title: string; body: string[] }>;
};

const LEGAL_COPY: Record<'en' | 'zh' | 'ja', Record<LegalKind, LegalCopy>> = {
  en: {
    privacy: {
      title: 'Privacy Policy',
      updated: 'Last updated: May 5, 2026',
      sections: [
        {
          title: 'Information we collect',
          body: [
            'When you create or use a GoLive account, we collect account details such as username, display name, email address, avatar, login method, invite code usage, and security records needed to keep the service reliable.',
            'If you sign in or link a Google account, we receive your Google account identifier, email address, email verification status, name, and profile picture for registration, login, and account binding.',
          ],
        },
        {
          title: 'How we use information',
          body: [
            'We use account information to provide login, profile, live room, chat, subscription, wallet, moderation, and creator tools.',
            'We use security and operational data to prevent abuse, diagnose incidents, protect invite-only registration, and maintain service availability.',
          ],
        },
        {
          title: 'Sharing and retention',
          body: [
            'We do not sell personal information. We share data only with infrastructure providers, legal or safety processes, or service components needed to operate GoLive.',
            'We keep account data while your account is active or as needed for security, legal, and operational records.',
          ],
        },
        {
          title: 'Your choices',
          body: [
            'You can update your profile and link a Google account in Settings. You may contact support to request account or privacy help.',
            'For privacy questions, contact the support email shown on the Google consent screen.',
          ],
        },
      ],
    },
    terms: {
      title: 'Terms of Service',
      updated: 'Last updated: May 5, 2026',
      sections: [
        {
          title: 'Using GoLive',
          body: [
            'GoLive provides live streaming, chat, creator, wallet, and community features. You are responsible for your account activity and for keeping your password secure.',
            'Registration may require an invite code. You may link one Google account to one GoLive account when the account ownership is verified.',
          ],
        },
        {
          title: 'Community and content',
          body: [
            'Do not upload, stream, or send illegal, abusive, infringing, deceptive, or harmful content. We may moderate, restrict, or remove content and accounts to protect users and the service.',
          ],
        },
        {
          title: 'Coins and creator tools',
          body: [
            'Coins, gifts, live permissions, and creator tools may be changed, limited, or reviewed to prevent fraud, abuse, or operational risk.',
          ],
        },
        {
          title: 'Service changes',
          body: [
            'We may update the service and these terms. Continued use after updates means you accept the current terms.',
          ],
        },
      ],
    },
  },
  zh: {
    privacy: {
      title: '隐私政策',
      updated: '最后更新：2026 年 5 月 5 日',
      sections: [
        {
          title: '我们收集的信息',
          body: [
            '当你创建或使用 GoLive 账号时，我们会收集用户名、昵称、邮箱、头像、登录方式、邀请码使用记录，以及维持服务安全与稳定所需的安全记录。',
            '如果你使用 Google 登录或绑定 Google 账号，我们会接收 Google 账号标识、邮箱、邮箱验证状态、姓名和头像，用于注册、登录和账号绑定。',
          ],
        },
        {
          title: '信息用途',
          body: [
            '我们使用账号信息提供登录、个人资料、直播间、聊天、订阅、钱包、内容管理和创作者工具。',
            '我们使用安全和运行数据来防止滥用、排查故障、保护邀请制注册，并维持服务可用性。',
          ],
        },
        {
          title: '共享与保留',
          body: [
            '我们不会出售个人信息。仅在运行 GoLive 所需的基础设施、法律或安全流程、以及必要服务组件之间共享数据。',
            '账号处于活动状态期间，或出于安全、法律和运行记录需要时，我们会保留相关账号数据。',
          ],
        },
        {
          title: '你的选择',
          body: [
            '你可以在设置中更新资料并绑定 Google 账号。你也可以联系支持邮箱寻求账号或隐私帮助。',
            '隐私相关问题请联系 Google 同意屏幕上显示的支持邮箱。',
          ],
        },
      ],
    },
    terms: {
      title: '服务条款',
      updated: '最后更新：2026 年 5 月 5 日',
      sections: [
        {
          title: '使用 GoLive',
          body: [
            'GoLive 提供直播、聊天、创作者、钱包和社区功能。你需要对账号活动负责，并妥善保管密码。',
            '注册可能需要邀请码。完成账号归属验证后，一个 Google 账号只能绑定一个 GoLive 账号。',
          ],
        },
        {
          title: '社区与内容',
          body: [
            '不得上传、直播或发送违法、辱骂、侵权、欺诈或有害内容。为保护用户和服务，我们可能审核、限制或移除内容和账号。',
          ],
        },
        {
          title: 'Coins 与创作者工具',
          body: [
            'Coins、礼物、直播权限和创作者工具可能会因反欺诈、反滥用或运行风险而调整、限制或审核。',
          ],
        },
        {
          title: '服务变更',
          body: ['我们可能更新服务和这些条款。更新后继续使用 GoLive，即表示你接受当前条款。'],
        },
      ],
    },
  },
  ja: {
    privacy: {
      title: 'プライバシーポリシー',
      updated: '最終更新日: 2026年5月5日',
      sections: [
        {
          title: '収集する情報',
          body: [
            'GoLive アカウントを作成または利用する際、ユーザー名、表示名、メールアドレス、アバター、ログイン方法、招待コードの利用記録、サービス保護に必要なセキュリティ記録を収集します。',
            'Google でログインまたは連携する場合、登録、ログイン、アカウント連携のために Google アカウント ID、メールアドレス、メール確認状態、名前、プロフィール画像を受け取ります。',
          ],
        },
        {
          title: '利用目的',
          body: [
            'アカウント情報は、ログイン、プロフィール、ライブ配信、チャット、登録チャンネル、ウォレット、モデレーション、クリエイターツールの提供に利用します。',
            'セキュリティおよび運用データは、不正利用の防止、障害調査、招待制登録の保護、サービス可用性の維持に利用します。',
          ],
        },
        {
          title: '共有と保持',
          body: [
            '個人情報を販売することはありません。GoLive の運営に必要なインフラ提供者、法的または安全上の手続き、必要なサービス構成要素に限りデータを共有します。',
            'アカウントが有効な期間、または安全、法令、運用記録のために必要な期間、アカウントデータを保持します。',
          ],
        },
        {
          title: '選択肢',
          body: [
            '設定からプロフィールを更新し、Google アカウントを連携できます。アカウントやプライバシーに関する相談はサポートへ連絡できます。',
            'プライバシーに関する質問は、Google 同意画面に表示されるサポートメールへお問い合わせください。',
          ],
        },
      ],
    },
    terms: {
      title: '利用規約',
      updated: '最終更新日: 2026年5月5日',
      sections: [
        {
          title: 'GoLive の利用',
          body: [
            'GoLive はライブ配信、チャット、クリエイター、ウォレット、コミュニティ機能を提供します。アカウント上の行為とパスワード管理は利用者の責任です。',
            '登録には招待コードが必要になる場合があります。所有確認後、1つの Google アカウントは1つの GoLive アカウントにのみ連携できます。',
          ],
        },
        {
          title: 'コミュニティとコンテンツ',
          body: [
            '違法、攻撃的、権利侵害、虚偽、有害なコンテンツのアップロード、配信、送信は禁止です。ユーザーとサービスを保護するため、コンテンツやアカウントを審査、制限、削除する場合があります。',
          ],
        },
        {
          title: 'Coins とクリエイターツール',
          body: [
            'Coins、ギフト、配信権限、クリエイターツールは、不正、悪用、運用リスクを防ぐために変更、制限、審査される場合があります。',
          ],
        },
        {
          title: 'サービス変更',
          body: [
            '当社はサービスおよび本規約を更新する場合があります。更新後も利用を続ける場合、最新の規約に同意したものとみなされます。',
          ],
        },
      ],
    },
  },
};

export function PrivacyPage() {
  return <LegalPage kind="privacy" />;
}

export function TermsPage() {
  return <LegalPage kind="terms" />;
}

function LegalPage({ kind }: { kind: LegalKind }) {
  const { i18n } = useTranslation('pages');
  const lang = (i18n.resolvedLanguage ?? i18n.language).startsWith('ja')
    ? 'ja'
    : (i18n.resolvedLanguage ?? i18n.language).startsWith('zh')
      ? 'zh'
      : 'en';
  const copy = LEGAL_COPY[lang][kind];

  return (
    <main className="gl-page gl-legal-page">
      <header className="gl-legal-head">
        <Link to="/" className="gl-legal-brand">
          GoLive
        </Link>
        <h1>{copy.title}</h1>
        <p>{copy.updated}</p>
      </header>
      <div className="gl-legal-body">
        {copy.sections.map((section) => (
          <section key={section.title} className="gl-legal-section">
            <h2>{section.title}</h2>
            {section.body.map((paragraph) => (
              <p key={paragraph}>{paragraph}</p>
            ))}
          </section>
        ))}
      </div>
    </main>
  );
}
