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
      <DialogContent className="gl-auth-dialog p-0 sm:max-w-[420px]">
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
