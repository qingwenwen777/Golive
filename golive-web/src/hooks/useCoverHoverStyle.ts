import { useEffect, useMemo, useState, type CSSProperties } from 'react';

type CoverHoverStyle = CSSProperties & {
  '--gl-card-hover-bg': string;
};

const SAMPLE_SIZE = 28;

export function useCoverHoverStyle(coverSrc: string | undefined, seed: string): CoverHoverStyle {
  const fallback = useMemo(() => fallbackHoverColor(seed), [seed]);
  const [hoverColor, setHoverColor] = useState(fallback);

  useEffect(() => {
    setHoverColor(fallback);
    if (!coverSrc || typeof window === 'undefined') return;

    let cancelled = false;
    const image = new Image();
    image.crossOrigin = 'anonymous';
    image.decoding = 'async';

    image.onload = () => {
      if (cancelled) return;
      const sampled = sampleHoverColor(image);
      if (sampled) setHoverColor(sampled);
    };
    image.onerror = () => {
      if (!cancelled) setHoverColor(fallback);
    };
    image.src = coverSrc;

    return () => {
      cancelled = true;
      image.onload = null;
      image.onerror = null;
    };
  }, [coverSrc, fallback]);

  return useMemo(() => ({ '--gl-card-hover-bg': hoverColor }), [hoverColor]);
}

function sampleHoverColor(image: HTMLImageElement): string | null {
  try {
    const canvas = document.createElement('canvas');
    canvas.width = SAMPLE_SIZE;
    canvas.height = SAMPLE_SIZE;
    const context = canvas.getContext('2d', { willReadFrequently: true });
    if (!context) return null;

    context.drawImage(image, 0, 0, SAMPLE_SIZE, SAMPLE_SIZE);
    const pixels = context.getImageData(0, 0, SAMPLE_SIZE, SAMPLE_SIZE).data;
    let red = 0;
    let green = 0;
    let blue = 0;
    let total = 0;
    let neutralRed = 0;
    let neutralGreen = 0;
    let neutralBlue = 0;
    let neutralTotal = 0;

    for (let index = 0; index < pixels.length; index += 4) {
      const alpha = pixels[index + 3] / 255;
      if (alpha < 0.25) continue;

      const r = pixels[index];
      const g = pixels[index + 1];
      const b = pixels[index + 2];
      const max = Math.max(r, g, b);
      const min = Math.min(r, g, b);
      const chroma = (max - min) / 255;
      const luminance = (0.2126 * r + 0.7152 * g + 0.0722 * b) / 255;
      const neutralWeight = alpha * (luminance < 0.06 || luminance > 0.96 ? 0.15 : 1);

      neutralRed += r * neutralWeight;
      neutralGreen += g * neutralWeight;
      neutralBlue += b * neutralWeight;
      neutralTotal += neutralWeight;

      if ((luminance < 0.08 || luminance > 0.95) && chroma < 0.12) continue;

      const colorWeight =
        alpha *
        (0.35 + Math.min(chroma * 2.4, 1.2)) *
        (luminance < 0.14 ? 0.45 : 1) *
        (luminance > 0.86 ? 0.65 : 1);
      red += r * colorWeight;
      green += g * colorWeight;
      blue += b * colorWeight;
      total += colorWeight;
    }

    if (total > 0.5) {
      return rgbToHoverColor(red / total, green / total, blue / total);
    }
    if (neutralTotal > 0.5) {
      return rgbToHoverColor(
        neutralRed / neutralTotal,
        neutralGreen / neutralTotal,
        neutralBlue / neutralTotal,
      );
    }
  } catch {
    return null;
  }

  return null;
}

function fallbackHoverColor(seed: string): string {
  let hash = 0;
  const value = seed.trim() || 'golive';
  for (let index = 0; index < value.length; index += 1) {
    hash = (hash * 33 + value.charCodeAt(index)) >>> 0;
  }
  return `hsla(${hash % 360}, 52%, 74%, 0.22)`;
}

function rgbToHoverColor(red: number, green: number, blue: number): string {
  const { hue, saturation } = rgbToHsl(red, green, blue);
  if (saturation < 0.08) return 'hsla(215, 18%, 76%, 0.2)';

  const displaySaturation = Math.round(clamp(saturation * 100, 36, 64));
  return `hsla(${Math.round(hue)}, ${displaySaturation}%, 74%, 0.24)`;
}

function rgbToHsl(red: number, green: number, blue: number) {
  const r = red / 255;
  const g = green / 255;
  const b = blue / 255;
  const max = Math.max(r, g, b);
  const min = Math.min(r, g, b);
  const lightness = (max + min) / 2;
  const delta = max - min;

  if (delta === 0) return { hue: 0, saturation: 0 };

  const saturation = delta / (1 - Math.abs(2 * lightness - 1));
  let hue = 0;
  if (max === r) hue = ((g - b) / delta) % 6;
  else if (max === g) hue = (b - r) / delta + 2;
  else hue = (r - g) / delta + 4;

  return { hue: (hue * 60 + 360) % 360, saturation };
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), max);
}
