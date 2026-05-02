import { type ChangeEvent, type FormEvent, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ImagePlus, Save } from 'lucide-react';
import { toast } from 'sonner';
import { useUploadChannelCover } from '@/api/auth';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import type { User } from '@/types/user';
import { userDisplayName } from '@/types/user';

const MAX_COVER_SIZE = 5 * 1024 * 1024;
const COVER_TYPES = new Set(['image/jpeg', 'image/png', 'image/webp', 'image/gif']);
const COVER_EXT = /\.(jpe?g|png|webp|gif)$/i;

export interface ChannelCoverUploadDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  user: User | null;
}

export function ChannelCoverUploadDialog({
  open,
  onOpenChange,
  user,
}: ChannelCoverUploadDialogProps) {
  const { t } = useTranslation('pages');
  const uploadCover = useUploadChannelCover();
  const [file, setFile] = useState<File | null>(null);
  const [preview, setPreview] = useState('');
  const [error, setError] = useState<string | null>(null);
  const displayName = userDisplayName(user);

  useEffect(() => {
    if (!file) {
      setPreview('');
      return;
    }
    const next = URL.createObjectURL(file);
    setPreview(next);
    return () => URL.revokeObjectURL(next);
  }, [file]);

  useEffect(() => {
    if (!open) {
      setFile(null);
      setError(null);
    }
  }, [open]);

  const handleFile = (e: ChangeEvent<HTMLInputElement>) => {
    const next = e.target.files?.[0] ?? null;
    e.target.value = '';
    setError(null);
    if (!next) {
      setFile(null);
      return;
    }
    if (!isAllowedCover(next)) {
      setFile(null);
      setError(t('upload.imageTypeError'));
      return;
    }
    if (next.size > MAX_COVER_SIZE) {
      setFile(null);
      setError(t('upload.cover.sizeError'));
      return;
    }
    setFile(next);
  };

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (!user) {
      setError(t('upload.cover.signInError'));
      return;
    }
    if (!file) {
      setError(t('upload.chooseFirst'));
      return;
    }
    uploadCover.mutate(file, {
      onSuccess: () => {
        toast.success(t('upload.cover.success'));
        onOpenChange(false);
      },
      onError: (err) => setError(err.message || t('upload.cover.failed')),
    });
  };

  const cover = preview || user?.cover || '';

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="gl-avatar-dialog p-0 sm:max-w-[560px]">
        <DialogHeader className="sr-only">
          <DialogTitle>{t('upload.cover.title')}</DialogTitle>
          <DialogDescription>{t('upload.description')}</DialogDescription>
        </DialogHeader>

        <form className="gl-avatar-form" onSubmit={handleSubmit}>
          <div className="gl-live-create-head">
            <div className="gl-live-create-icon" aria-hidden="true">
              <ImagePlus size={20} />
            </div>
            <div>
              <h2>{t('upload.cover.title')}</h2>
              <p>{displayName}</p>
            </div>
          </div>

          <label className="gl-cover-picker">
            <span className="gl-cover-preview">
              {cover ? (
                <img src={cover} alt="" />
              ) : (
                <span>
                  <ImagePlus size={24} />
                  {t('upload.cover.add')}
                </span>
              )}
            </span>
            <span className="gl-avatar-pick-btn">
              <ImagePlus size={16} />
              {t('upload.chooseImage')}
            </span>
            <input
              type="file"
              accept="image/jpeg,image/png,image/webp,image/gif"
              onChange={handleFile}
            />
          </label>

          {error && (
            <div className="gl-auth-error" role="alert">
              {error}
            </div>
          )}

          <button type="submit" disabled={!file || uploadCover.isPending} className="gl-auth-submit">
            <Save size={18} />
            <span>{uploadCover.isPending ? t('upload.saving') : t('upload.cover.save')}</span>
          </button>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function isAllowedCover(file: File): boolean {
  if (COVER_TYPES.has(file.type)) return true;
  return COVER_EXT.test(file.name);
}
