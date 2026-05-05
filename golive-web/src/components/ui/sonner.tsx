import { Toaster as Sonner, type ToasterProps } from 'sonner';
import { useThemeStore } from '@/stores/useThemeStore';
import { cn } from '@/lib/cn';

export function Toaster({ className, ...props }: ToasterProps) {
  const theme = useThemeStore((s) => s.theme);
  return (
    <Sonner
      theme={theme === 'light' ? 'light' : 'dark'}
      position="bottom-right"
      richColors
      closeButton={false}
      {...props}
      className={cn('gl-toaster', className)}
    />
  );
}
