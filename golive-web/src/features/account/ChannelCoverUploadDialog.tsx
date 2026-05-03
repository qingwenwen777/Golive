import { type FormEvent, useEffect, useState } from 'react';
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
import {
  ImageCropField,
  cropSelectionToFile,
  type ImageCropSelection,
} from '@/features/media/ImageCropField';
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
  const [selection, setSelection] = useState<ImageCropSelection | null>(null);
  const [processing, setProcessing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const displayName = userDisplayName(user);

  useEffect(() => {
    if (!open) {
      setSelection(null);
      setProcessing(false);
      setError(null);
    }
  }, [open]);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (!user) {
      setError(t('upload.cover.signInError'));
      return;
    }
    if (!selection) {
      setError(t('upload.chooseFirst'));
      return;
    }

    setProcessing(true);
    setError(null);
    try {
      const cropped = await cropSelectionToFile(selection, CHANNEL_COVER_CROP_CONFIG, 'channel-cover');
      uploadCover.mutate(cropped, {
        onSuccess: () => {
          toast.success(t('upload.cover.success'));
          onOpenChange(false);
        },
        onError: (err) => {
          setProcessing(false);
          setError(err.message || t('upload.cover.failed'));
        },
      });
    } catch {
      setProcessing(false);
      setError(t('upload.cover.failed'));
    }
  };

  const cover = user?.cover || '';

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

          <ImageCropField
            active={open}
            accept="image/jpeg,image/png,image/webp,image/gif"
            className="gl-cover-crop"
            config={CHANNEL_COVER_CROP_CONFIG}
            disabled={uploadCover.isPending || processing}
            emptyLabel={t('upload.cover.add')}
            fallback={
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
            }
            pickLabel={t('upload.chooseImage')}
            validateFile={(next) => {
              if (!isAllowedCover(next)) return t('upload.imageTypeError');
              if (next.size > MAX_COVER_SIZE) return t('upload.cover.sizeError');
              return null;
            }}
            onError={setError}
            onSelectionChange={setSelection}
          />

          {error && (
            <div className="gl-auth-error" role="alert">
              {error}
            </div>
          )}

          <button
            type="submit"
            disabled={!selection || uploadCover.isPending || processing}
            className="gl-auth-submit"
          >
            <Save size={18} />
            <span>{uploadCover.isPending || processing ? t('upload.saving') : t('upload.cover.save')}</span>
          </button>
        </form>
      </DialogContent>
    </Dialog>
  );
}

const CHANNEL_COVER_CROP_CONFIG = {
  aspectRatio: 16 / 5,
  outputWidth: 1600,
  outputHeight: 500,
  quality: 0.93,
  mimeType: 'image/jpeg',
} as const;

function isAllowedCover(file: File): boolean {
  if (COVER_TYPES.has(file.type)) return true;
  return COVER_EXT.test(file.name);
}
