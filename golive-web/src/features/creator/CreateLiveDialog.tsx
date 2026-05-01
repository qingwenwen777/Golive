import { type FormEvent, useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { AxiosError } from 'axios';
import { ImagePlus, Radio, Send, Wand2 } from 'lucide-react';
import { toast } from 'sonner';
import { useMe } from '@/api/auth';
import { useSubmitCreatorApplication } from '@/api/creator';
import { useGoLive, useUploadLiveCover } from '@/api/room';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { CATEGORIES_EN } from '@/constants/catalog';
import { useAuthStore } from '@/stores/useAuthStore';
import { userDisplayName } from '@/types/user';
import type { Stream } from '@/types/stream';

export const LIVE_SESSION_STORAGE_KEY = 'golive-live-session';

export interface PublisherSession {
  streamId: string;
  streamKey: string;
  playbackUrl?: string;
  rtmpServer: string;
  createdAt: string;
}

export function publisherSessionFromStream(stream: Stream): PublisherSession | null {
  if (!stream.streamKey) return null;
  const rtmpServer = (import.meta.env.VITE_RTMP_BASE || 'rtmp://localhost/live').replace(/\/$/, '');
  return {
    streamId: stream.id,
    streamKey: stream.streamKey,
    playbackUrl: stream.playbackUrl,
    rtmpServer,
    createdAt: new Date().toISOString(),
  };
}

export function savePublisherSession(stream: Stream) {
  const session = publisherSessionFromStream(stream);
  if (!session) return;
  localStorage.setItem(LIVE_SESSION_STORAGE_KEY, JSON.stringify(session));
}

export function loadPublisherSession(): PublisherSession | null {
  try {
    const raw = localStorage.getItem(LIVE_SESSION_STORAGE_KEY);
    return raw ? (JSON.parse(raw) as PublisherSession) : null;
  } catch {
    return null;
  }
}

export interface CreateLiveDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

function liveErrorMessage(err: Error): string {
  if (err instanceof AxiosError) {
    if (err.response?.status === 401) return 'Please sign in again before starting a live.';
    if (err.response?.status === 403) {
      const data = err.response.data as { message?: string } | undefined;
      return data?.message || 'Your live permission has not been approved yet.';
    }
    if (err.response?.status === 400) return 'Title and category are required.';
  }
  return 'Could not start the live. Please try again.';
}

function permissionCopy(status: string) {
  if (status === 'pending') {
    return {
      title: 'Application under review',
      body: 'Your creator request is waiting for an administrator to review it.',
      action: 'Submitted',
    };
  }
  if (status === 'rejected') {
    return {
      title: 'Creator access was not approved',
      body: 'You can submit a new application when your channel is ready for review.',
      action: 'Apply again',
    };
  }
  return {
    title: 'Apply for creator access',
    body: 'A quick admin approval is required before you can create live rooms.',
    action: 'Submit application',
  };
}

export function CreateLiveDialog({ open, onOpenChange }: CreateLiveDialogProps) {
  const navigate = useNavigate();
  const user = useAuthStore((s) => s.user);
  const { data: meUser, refetch: refetchMe } = useMe();
  const goLive = useGoLive();
  const uploadCover = useUploadLiveCover();
  const apply = useSubmitCreatorApplication();
  const [title, setTitle] = useState('Untitled live');
  const [description, setDescription] = useState('');
  const [category, setCategory] = useState('Just Chatting');
  const [coverFile, setCoverFile] = useState<File | null>(null);
  const [coverPreview, setCoverPreview] = useState('');
  const [error, setError] = useState<string | null>(null);
  const currentUser = meUser ?? user;
  const channelName = useMemo(() => userDisplayName(currentUser), [currentUser]);
  const categories = CATEGORIES_EN.filter((c) => c !== 'All' && c !== 'Live Now');
  const livePermissionStatus = currentUser?.livePermissionStatus ?? 'none';
  const canGoLive = livePermissionStatus === 'approved';

  const isPending = goLive.isPending || uploadCover.isPending;
  const perm = permissionCopy(livePermissionStatus);

  useEffect(() => {
    if (open && user) {
      void refetchMe();
    }
  }, [open, user, refetchMe]);

  const submitApplication = () => {
    apply.mutate(undefined, {
      onSuccess: (resp) => toast.success(resp.message),
      onError: (err) => toast.error(err.message || 'Could not submit application.'),
    });
  };

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setError(null);

    try {
      const uploadedCover = coverFile ? (await uploadCover.mutateAsync(coverFile)).url : '';
      goLive.mutate(
        {
          title: title.trim(),
          description: description.trim(),
          category,
          cover: uploadedCover,
          channelName,
          avatar: currentUser?.avatar,
        },
        {
          onSuccess: (stream) => {
            savePublisherSession(stream);
            toast.success('Live room created. Add the stream key in OBS to publish video.');
            onOpenChange(false);
            navigate(`/live/${stream.id}`);
          },
          onError: (err) => {
            setError(liveErrorMessage(err));
          },
        },
      );
    } catch (err) {
      setError(err instanceof Error ? liveErrorMessage(err) : 'Could not upload the cover.');
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="gl-live-create-dialog p-0 sm:max-w-[480px]">
        <DialogHeader className="sr-only">
          <DialogTitle>Start a live</DialogTitle>
          <DialogDescription>Create a live room and get your OBS stream key.</DialogDescription>
        </DialogHeader>

        {!canGoLive ? (
          <div className="gl-live-create">
            <div className="gl-live-create-head">
              <div className="gl-live-create-icon" aria-hidden="true">
                <Radio size={20} />
              </div>
              <div>
                <h2>{perm.title}</h2>
                <p>{perm.body}</p>
              </div>
            </div>
            <div className={`gl-creator-permission is-${livePermissionStatus}`}>
              <span>Live permission</span>
              <strong>{livePermissionStatus}</strong>
            </div>
            <button
              type="button"
              disabled={apply.isPending || livePermissionStatus === 'pending'}
              className="gl-auth-submit"
              onClick={submitApplication}
            >
              <Send size={18} />
              <span>{apply.isPending ? 'Submitting...' : perm.action}</span>
            </button>
          </div>
        ) : (
          <form className="gl-live-create" onSubmit={onSubmit}>
            <div className="gl-live-create-head">
              <div className="gl-live-create-icon" aria-hidden="true">
                <Radio size={20} />
              </div>
              <div>
                <h2>Start a live</h2>
                <p>Create a room, then publish from OBS with the stream key.</p>
              </div>
            </div>

            <label className="gl-auth-field">
              <span className="gl-auth-label">Title</span>
              <span className="gl-auth-input-wrap">
                <input
                  value={title}
                  onChange={(e) => setTitle(e.target.value)}
                  maxLength={120}
                  required
                />
              </span>
            </label>

            <label className="gl-auth-field">
              <span className="gl-auth-label">Category</span>
              <select
                className="gl-live-create-select"
                value={category}
                onChange={(e) => setCategory(e.target.value)}
                required
              >
                {categories.map((item) => (
                  <option key={item} value={item}>
                    {item}
                  </option>
                ))}
              </select>
            </label>

            <label className="gl-auth-field">
              <span className="gl-auth-label">Cover</span>
              <span className="gl-live-cover-picker">
                {coverPreview ? (
                  <img src={coverPreview} alt="" />
                ) : (
                  <span className="gl-live-cover-empty">
                    <ImagePlus size={22} />
                    <span>Add cover</span>
                  </span>
                )}
                <input
                  type="file"
                  accept="image/jpeg,image/png,image/webp,image/gif"
                  onChange={(e) => {
                    const file = e.target.files?.[0] ?? null;
                    setCoverFile(file);
                    setCoverPreview(file ? URL.createObjectURL(file) : '');
                  }}
                />
              </span>
            </label>

            <label className="gl-auth-field">
              <span className="gl-auth-label">Description</span>
              <textarea
                className="gl-live-create-textarea"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                maxLength={2000}
                rows={4}
                placeholder="Optional"
              />
            </label>

            {error && (
              <div className="gl-auth-error" role="alert">
                {error}
              </div>
            )}

            <button type="submit" disabled={isPending || !title.trim()} className="gl-auth-submit">
              <Wand2 size={18} />
              <span>{isPending ? 'Starting...' : 'Create live room'}</span>
            </button>
          </form>
        )}
      </DialogContent>
    </Dialog>
  );
}
