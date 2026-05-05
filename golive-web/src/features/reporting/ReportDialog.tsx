import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { AxiosError } from 'axios';
import { AlertTriangle, Flag, Loader2, ShieldCheck } from 'lucide-react';
import { toast } from 'sonner';
import {
  useSubmitReport,
  type CreateReportPayload,
  type ReportReason,
} from '@/api/contentModeration';
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog';
import { cn } from '@/lib/cn';
import { useAuthModalStore } from '@/stores/useAuthModalStore';
import { useIsAuthed } from '@/stores/useAuthStore';

export type ReportTargetDraft = Omit<CreateReportPayload, 'reason' | 'description'>;

const REPORT_REASONS: Array<{ value: ReportReason; icon: typeof Flag }> = [
  { value: 'spam', icon: Flag },
  { value: 'harassment', icon: ShieldCheck },
  { value: 'sexual', icon: AlertTriangle },
  { value: 'violence', icon: AlertTriangle },
  { value: 'hate', icon: Flag },
  { value: 'scam', icon: ShieldCheck },
  { value: 'illegal', icon: AlertTriangle },
  { value: 'other', icon: Flag },
];

function reportErrorMessage(err: unknown, t: ReturnType<typeof useTranslation>['t']): string {
  if (err instanceof AxiosError) {
    const data = err.response?.data as { reason?: string; message?: string } | undefined;
    if (data?.reason === 'report_duplicate') return t('report.errors.duplicate');
    if (data?.reason === 'report_daily_limit') return t('report.errors.dailyLimit');
    if (data?.reason === 'reason_required') return t('report.errors.reasonRequired');
    if (err.response?.status === 401) return t('report.errors.loginRequired');
    return data?.message ?? err.message;
  }
  return err instanceof Error ? err.message : t('report.errors.failed');
}

export function ReportDialog({
  open,
  target,
  onOpenChange,
}: {
  open: boolean;
  target: ReportTargetDraft | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation('pages');
  const isAuthed = useIsAuthed();
  const openLogin = useAuthModalStore((s) => s.openLogin);
  const submitReport = useSubmitReport();
  const [reason, setReason] = useState<ReportReason | ''>('');
  const [description, setDescription] = useState('');
  const descriptionLength = useMemo(() => Array.from(description).length, [description]);

  const reset = () => {
    setReason('');
    setDescription('');
  };

  const submit = () => {
    if (!target) return;
    if (!isAuthed) {
      openLogin();
      return;
    }
    if (!reason) {
      toast.error(t('report.errors.reasonRequired'));
      return;
    }
    submitReport.mutate(
      {
        ...target,
        reason,
        description,
      },
      {
        onSuccess: () => {
          toast.success(t('report.submitted'));
          onOpenChange(false);
          reset();
        },
        onError: (err) => toast.error(reportErrorMessage(err, t)),
      },
    );
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        onOpenChange(next);
        if (!next) reset();
      }}
    >
      <DialogContent className="gl-report-dialog p-0 sm:max-w-[520px]">
        <div className="gl-report-dialog-body">
          <div className="gl-report-dialog-icon" aria-hidden="true">
            <Flag size={22} />
          </div>
          <DialogTitle>{t('report.dialogTitle')}</DialogTitle>
          <DialogDescription>{t('report.dialogDescription')}</DialogDescription>

          <div className="gl-report-reason-grid" role="radiogroup" aria-label={t('report.type')}>
            {REPORT_REASONS.map(({ value, icon: Icon }) => (
              <button
                key={value}
                type="button"
                className={cn('gl-report-reason', reason === value && 'is-selected')}
                aria-checked={reason === value}
                role="radio"
                onClick={() => setReason(value)}
              >
                <Icon size={16} />
                <span>{t(`report.reasons.${value}`)}</span>
              </button>
            ))}
          </div>

          <label className="gl-report-note">
            <span>{t('report.descriptionLabel')}</span>
            <textarea
              value={description}
              maxLength={100}
              onChange={(event) => setDescription(event.target.value)}
              placeholder={t('report.descriptionPlaceholder')}
            />
            <small>{descriptionLength}/100</small>
          </label>

          <div className="gl-report-actions">
            <button
              type="button"
              className="gl-report-cancel"
              onClick={() => onOpenChange(false)}
              disabled={submitReport.isPending}
            >
              {t('report.cancel')}
            </button>
            <button
              type="button"
              className="gl-report-submit"
              onClick={submit}
              disabled={!reason || submitReport.isPending}
            >
              {submitReport.isPending && <Loader2 size={16} className="animate-spin" />}
              {submitReport.isPending ? t('report.submitting') : t('report.submit')}
            </button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
