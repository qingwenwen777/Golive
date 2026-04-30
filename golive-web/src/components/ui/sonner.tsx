import { Toaster as Sonner, type ToasterProps } from 'sonner';
import { useThemeStore } from '@/stores/useThemeStore';

export function Toaster(props: ToasterProps) {
  const theme = useThemeStore((s) => s.theme);
  return (
    <Sonner
      theme={theme === 'light' ? 'light' : 'dark'}
      position="bottom-right"
      richColors
      closeButton={false}
      {...props}
    />
  );
}
