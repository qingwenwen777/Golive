// promptTitle is shown (already translated) when the browser can't copy and
// the text has to be copied by hand.
export async function copyText(
  value: string,
  promptTitle = '',
): Promise<'clipboard' | 'fallback' | 'manual'> {
  const text = String(value ?? '');
  if (!text) throw new Error('empty-copy-value');

  if (navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text);
      return 'clipboard';
    } catch {
      // Fall through to the legacy path. HTTP deployments commonly land here.
    }
  }

  if (copyWithTextarea(text)) {
    return 'fallback';
  }

  window.prompt(promptTitle, text);
  return 'manual';
}

function copyWithTextarea(value: string): boolean {
  const textarea = document.createElement('textarea');
  textarea.value = value;
  textarea.setAttribute('readonly', '');
  textarea.style.position = 'fixed';
  textarea.style.top = '0';
  textarea.style.left = '0';
  textarea.style.width = '1px';
  textarea.style.height = '1px';
  textarea.style.opacity = '0';
  textarea.style.pointerEvents = 'none';

  document.body.appendChild(textarea);
  textarea.focus();
  textarea.select();
  textarea.setSelectionRange(0, textarea.value.length);

  try {
    return document.execCommand('copy');
  } catch {
    return false;
  } finally {
    document.body.removeChild(textarea);
  }
}
