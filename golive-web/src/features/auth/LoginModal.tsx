import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { AuthPanel } from '@/features/auth/AuthPanel';
import { useAuthModalStore } from '@/stores/useAuthModalStore';

export function LoginModal() {
  const open = useAuthModalStore((s) => s.open);
  const close = useAuthModalStore((s) => s.close);
  const consumeAfterLogin = useAuthModalStore((s) => s.consumeAfterLogin);

  return (
    <Dialog open={open} onOpenChange={(v) => (v ? undefined : close())}>
      <DialogContent className="gl-auth-dialog max-h-[calc(100dvh-24px)] w-[min(440px,calc(100vw-24px))] max-w-[440px] overflow-hidden p-0">
        <DialogHeader className="sr-only">
          <DialogTitle>Sign in to GoLive</DialogTitle>
          <DialogDescription>Sign in or register for a GoLive account.</DialogDescription>
        </DialogHeader>
        <AuthPanel
          onAuthenticated={() => {
            close();
            consumeAfterLogin();
          }}
        />
      </DialogContent>
    </Dialog>
  );
}
