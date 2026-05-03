import { type FormEvent, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Camera, Save } from 'lucide-react';
import { toast } from 'sonner';
import { useUploadAvatar } from '@/api/auth';
import { Avatar } from '@/components/Avatar';
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

const MAX_AVATAR_SIZE = 5 * 1024 * 1024;
const AVATAR_TYPES = new Set(['image/jpeg', 'image/png', 'image/webp', 'image/gif']);
const AVATAR_EXT = /\.(jpe?g|png|webp|gif)$/i;

export interface AvatarUploadDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  user: User | null;
}

export function AvatarUploadDialog({ open, onOpenChange, user }: AvatarUploadDialogProps) {
  const { t } = useTranslation('pages');
  const uploadAvatar = useUploadAvatar();
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
      setError(t('upload.avatar.signInError'));
      return;
    }
    if (!selection) {
      setError(t('upload.chooseFirst'));
      return;
    }

    setProcessing(true);
    setError(null);
    try {
      const cropped = await cropSelectionToFile(selection, AVATAR_CROP_CONFIG, 'avatar');
      uploadAvatar.mutate(cropped, {
        onSuccess: () => {
          toast.success(t('upload.avatar.success'));
          onOpenChange(false);
        },
        onError: (err) => {
          setProcessing(false);
          setError(err.message || t('upload.avatar.failed'));
        },
      });
    } catch {
      setProcessing(false);
      setError(t('upload.avatar.failed'));
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="gl-avatar-dialog p-0 sm:max-w-[420px]">
        <DialogHeader className="sr-only">
          <DialogTitle>{t('upload.avatar.title')}</DialogTitle>
          <DialogDescription>{t('upload.description')}</DialogDescription>
        </DialogHeader>

        <form className="gl-avatar-form" onSubmit={handleSubmit}>
          <div className="gl-live-create-head">
            <div className="gl-live-create-icon" aria-hidden="true">
              <Camera size={20} />
            </div>
            <div>
              <h2>{t('upload.avatar.title')}</h2>
              <p>{displayName}</p>
            </div>
          </div>

          <ImageCropField
            active={open}
            accept="image/jpeg,image/png,image/webp,image/gif"
            className="gl-avatar-crop"
            config={AVATAR_CROP_CONFIG}
            disabled={uploadAvatar.isPending || processing}
            emptyLabel={t('upload.chooseImage')}
            fallback={
              <span className="gl-avatar-preview">
                <Avatar name={displayName} src={user?.avatar} size={116} />
              </span>
            }
            pickLabel={t('upload.chooseImage')}
            shape="circle"
            validateFile={(next) => {
              if (!isAllowedAvatar(next)) return t('upload.imageTypeError');
              if (next.size > MAX_AVATAR_SIZE) return t('upload.avatar.sizeError');
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
            disabled={!selection || uploadAvatar.isPending || processing}
            className="gl-auth-submit"
          >
            <Save size={18} />
            <span>{uploadAvatar.isPending || processing ? t('upload.saving') : t('upload.avatar.save')}</span>
          </button>
        </form>
      </DialogContent>
    </Dialog>
  );
}

const AVATAR_CROP_CONFIG = {
  aspectRatio: 1,
  outputWidth: 512,
  outputHeight: 512,
  quality: 0.94,
  mimeType: 'image/jpeg',
} as const;

function isAllowedAvatar(file: File): boolean {
  if (AVATAR_TYPES.has(file.type)) return true;
  return AVATAR_EXT.test(file.name);
}
