import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { Instagram, Link2, type LucideIcon } from 'lucide-react';
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog';
import { LoadableImage } from '@/components/LoadableImage';
import { cn } from '@/lib/cn';
import { copyText } from '@/lib/clipboard';

export interface ShareDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  url: string;
  description?: string;
  previewImage?: string;
  previewKicker?: string;
  previewMeta?: string;
  imageAlt?: string;
}

export function ShareDialog({
  open,
  onOpenChange,
  title,
  url,
  description,
  previewImage,
  previewKicker,
  previewMeta,
  imageAlt,
}: ShareDialogProps) {
  const { t } = useTranslation('pages');
  const shareText = description?.trim() || title;
  const shareUrl = url || (typeof window !== 'undefined' ? window.location.href : '');

  const handleCopy = async () => {
    try {
      const method = await copyText(
        shareUrl,
        t('copyPrompt', {
          ns: 'common',
          target: t('shareDialog.copyTarget', { defaultValue: 'share link' }),
          defaultValue: 'Copy {{target}}',
        }),
      );
      if (method === 'manual') {
        toast.info(
          t('shareDialog.copyManual', {
            defaultValue: "Automatic copy isn't available. Copy the link manually.",
          }),
        );
      } else {
        toast.success(t('shareDialog.copySuccess', { defaultValue: 'Link copied.' }));
      }
    } catch {
      toast.error(t('shareDialog.copyFailed', { defaultValue: 'Could not copy the link.' }));
    }
  };

  const actions: ShareAction[] = [
    {
      id: 'copy',
      label: t('shareDialog.copyLink', { defaultValue: 'Copy link' }),
      Icon: Link2,
      onClick: handleCopy,
    },
    {
      id: 'x',
      label: t('shareDialog.x', { defaultValue: 'X' }),
      mark: 'X',
      onClick: () => openShareTarget(buildXShareUrl(shareText, shareUrl)),
    },
    {
      id: 'instagram',
      label: t('shareDialog.instagram', { defaultValue: 'Instagram' }),
      Icon: Instagram,
      onClick: async () => {
        await handleCopy();
        openShareTarget('https://www.instagram.com/');
      },
    },
    {
      id: 'reddit',
      label: t('shareDialog.reddit', { defaultValue: 'Reddit' }),
      mark: 'r',
      onClick: () => openShareTarget(buildRedditShareUrl(title, shareUrl)),
    },
  ];

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent overlayClassName="gl-share-overlay" className="gl-share-dialog max-w-none p-0">
        <div className="gl-share-dialog-body">
          <DialogTitle className="gl-share-title">{title}</DialogTitle>
          <DialogDescription className="sr-only">
            {description ||
              t('shareDialog.description', { defaultValue: 'Share this GoLive page.' })}
          </DialogDescription>

          <div className="gl-share-divider" />

          <div className="gl-share-preview" aria-hidden="true">
            <div className={cn('gl-share-preview-media', previewImage && 'has-image')}>
              {previewImage ? (
                <LoadableImage src={previewImage} alt={imageAlt ?? ''} loading="lazy" />
              ) : (
                <div className="gl-share-preview-fallback">
                  <span>
                    {previewKicker || t('shareDialog.preview', { defaultValue: 'Share' })}
                  </span>
                  <strong>GoLive</strong>
                </div>
              )}
            </div>
            <div className="gl-share-preview-copy">
              {previewKicker && <span>{previewKicker}</span>}
              <h3>{title}</h3>
              {description && <p>{description}</p>}
              {previewMeta && <small>{previewMeta}</small>}
            </div>
            <strong className="gl-share-preview-brand">GoLive</strong>
          </div>

          <div className="gl-share-actions">
            {actions.map((action) => (
              <button key={action.id} type="button" onClick={action.onClick}>
                <span className="gl-share-action-icon">
                  {action.Icon ? <action.Icon size={24} strokeWidth={2.1} /> : action.mark}
                </span>
                <span>{action.label}</span>
              </button>
            ))}
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}

interface ShareAction {
  id: string;
  label: string;
  Icon?: LucideIcon;
  mark?: string;
  onClick: () => void | Promise<void>;
}

function openShareTarget(url: string) {
  window.open(url, '_blank', 'noopener,noreferrer');
}

function buildXShareUrl(text: string, url: string): string {
  const target = new URL('https://twitter.com/intent/tweet');
  target.searchParams.set('text', text);
  target.searchParams.set('url', url);
  return target.toString();
}

function buildRedditShareUrl(title: string, url: string): string {
  const target = new URL('https://www.reddit.com/submit');
  target.searchParams.set('title', title);
  target.searchParams.set('url', url);
  return target.toString();
}
