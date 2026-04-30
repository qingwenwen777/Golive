import { type ChangeEvent, type FormEvent, useEffect, useState } from 'react';
import { Camera, ImagePlus, Save } from 'lucide-react';
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
  const uploadAvatar = useUploadAvatar();
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
    if (!isAllowedAvatar(next)) {
      setFile(null);
      setError('Use a JPG, PNG, WebP, or GIF image.');
      return;
    }
    if (next.size > MAX_AVATAR_SIZE) {
      setFile(null);
      setError('Avatar must be 5MB or smaller.');
      return;
    }
    setFile(next);
  };

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (!user) {
      setError('Please sign in before changing your avatar.');
      return;
    }
    if (!file) {
      setError('Choose an image first.');
      return;
    }
    uploadAvatar.mutate(file, {
      onSuccess: () => {
        toast.success('Avatar updated.');
        onOpenChange(false);
      },
      onError: (err) => setError(err.message || 'Could not update avatar.'),
    });
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="gl-avatar-dialog p-0 sm:max-w-[420px]">
        <DialogHeader className="sr-only">
          <DialogTitle>Change avatar</DialogTitle>
          <DialogDescription>Upload a JPG, PNG, WebP, or GIF image up to 5MB.</DialogDescription>
        </DialogHeader>

        <form className="gl-avatar-form" onSubmit={handleSubmit}>
          <div className="gl-live-create-head">
            <div className="gl-live-create-icon" aria-hidden="true">
              <Camera size={20} />
            </div>
            <div>
              <h2>Change avatar</h2>
              <p>{displayName}</p>
            </div>
          </div>

          <label className="gl-avatar-picker">
            <span className="gl-avatar-preview">
              {preview ? (
                <img src={preview} alt="" />
              ) : (
                <Avatar name={displayName} src={user?.avatar} size={116} />
              )}
            </span>
            <span className="gl-avatar-pick-btn">
              <ImagePlus size={16} />
              Choose image
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

          <button type="submit" disabled={!file || uploadAvatar.isPending} className="gl-auth-submit">
            <Save size={18} />
            <span>{uploadAvatar.isPending ? 'Saving...' : 'Save avatar'}</span>
          </button>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function isAllowedAvatar(file: File): boolean {
  if (AVATAR_TYPES.has(file.type)) return true;
  return AVATAR_EXT.test(file.name);
}
