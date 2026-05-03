// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import {
  ImageCropField,
  cropSelectionToFile,
  type ImageCropConfig,
  type ImageCropSelection,
} from './ImageCropField';

const createObjectURLMock = vi.fn((file: File) => `blob:test-${file.name}`);
const revokeObjectURLMock = vi.fn();
const drawImageMock = vi.fn();
const clearRectMock = vi.fn();
const fillRectMock = vi.fn();

let nextImageSize = { width: 800, height: 600 };
let imageShouldError = false;

class MockImage {
  onload: ((event: Event) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  naturalWidth = nextImageSize.width;
  naturalHeight = nextImageSize.height;
  private currentSrc = '';

  set src(value: string) {
    this.currentSrc = value;
    this.naturalWidth = nextImageSize.width;
    this.naturalHeight = nextImageSize.height;
    queueMicrotask(() => {
      if (imageShouldError) {
        this.onerror?.(new Event('error'));
        return;
      }
      this.onload?.(new Event('load'));
    });
  }

  get src() {
    return this.currentSrc;
  }
}

class MockResizeObserver {
  observe = vi.fn();
  disconnect = vi.fn();
  unobserve = vi.fn();
}

const cropConfig: ImageCropConfig = {
  aspectRatio: 1,
  outputWidth: 512,
  outputHeight: 512,
  quality: 0.94,
  mimeType: 'image/jpeg',
};

function renderCropField(overrides: Partial<Parameters<typeof ImageCropField>[0]> = {}) {
  const props: Parameters<typeof ImageCropField>[0] = {
    accept: 'image/*',
    active: true,
    config: cropConfig,
    emptyLabel: 'Choose image',
    pickLabel: 'Pick image',
    validateFile: () => null,
    onError: vi.fn(),
    onSelectionChange: vi.fn(),
    ...overrides,
  };

  const view = render(<ImageCropField {...props} />);
  return { ...view, props };
}

describe('ImageCropField', () => {
  beforeEach(() => {
    nextImageSize = { width: 800, height: 600 };
    imageShouldError = false;
    createObjectURLMock.mockClear();
    revokeObjectURLMock.mockClear();
    drawImageMock.mockClear();
    clearRectMock.mockClear();
    fillRectMock.mockClear();

    vi.stubGlobal('Image', MockImage);
    vi.stubGlobal('ResizeObserver', MockResizeObserver);
    Object.defineProperty(URL, 'createObjectURL', {
      configurable: true,
      value: createObjectURLMock,
    });
    Object.defineProperty(URL, 'revokeObjectURL', {
      configurable: true,
      value: revokeObjectURLMock,
    });
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it('loads a valid file, reports natural image size, and clears object urls when deactivated', async () => {
    const onSelectionChange = vi.fn();
    const onError = vi.fn();
    const { props, rerender } = renderCropField({ onSelectionChange, onError });
    const file = new File(['avatar'], 'avatar.png', { type: 'image/png' });

    fireEvent.change(screen.getByLabelText('Pick image'), {
      target: { files: [file] },
    });

    await waitFor(() => {
      expect(onSelectionChange).toHaveBeenLastCalledWith(
        expect.objectContaining({
          file,
          url: 'blob:test-avatar.png',
          naturalWidth: 800,
          naturalHeight: 600,
          zoom: 1,
          offsetX: 0,
          offsetY: 0,
        }),
      );
    });

    expect(createObjectURLMock).toHaveBeenCalledWith(file);
    expect(onError).toHaveBeenCalledWith(null);

    rerender(<ImageCropField {...props} active={false} />);

    await waitFor(() => {
      expect(revokeObjectURLMock).toHaveBeenCalledWith('blob:test-avatar.png');
      expect(onSelectionChange).toHaveBeenLastCalledWith(null);
    });
  });

  it('rejects invalid files before creating an object url', () => {
    const onError = vi.fn();
    const onSelectionChange = vi.fn();
    renderCropField({
      onError,
      onSelectionChange,
      validateFile: () => 'Only jpg, png, webp, or gif are allowed.',
    });

    fireEvent.change(screen.getByLabelText('Pick image'), {
      target: { files: [new File(['bad'], 'notes.txt', { type: 'text/plain' })] },
    });

    expect(createObjectURLMock).not.toHaveBeenCalled();
    expect(onError).toHaveBeenLastCalledWith('Only jpg, png, webp, or gif are allowed.');
    expect(onSelectionChange).toHaveBeenLastCalledWith(null);
  });

  it('renders a crop selection into the configured output canvas and mime type', async () => {
    const toBlobCalls: Array<{ width: number; height: number; type?: string; quality?: number }> =
      [];
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockImplementation((contextId) => {
      if (contextId !== '2d') return null;
      return {
        clearRect: clearRectMock,
        fillRect: fillRectMock,
        drawImage: drawImageMock,
        fillStyle: '',
        imageSmoothingEnabled: false,
        imageSmoothingQuality: 'low',
      } as unknown as CanvasRenderingContext2D;
    });
    vi.spyOn(HTMLCanvasElement.prototype, 'toBlob').mockImplementation(function (
      this: HTMLCanvasElement,
      callback: BlobCallback,
      type?: string,
      quality?: number,
    ) {
      toBlobCalls.push({ width: this.width, height: this.height, type, quality });
      callback(new Blob(['cropped'], { type }));
    });

    const selection: ImageCropSelection = {
      file: new File(['cover'], 'cover.png', { type: 'image/png' }),
      url: 'blob:test-cover.png',
      naturalWidth: 4000,
      naturalHeight: 2000,
      zoom: 2,
      offsetX: 1,
      offsetY: -1,
    };

    const output = await cropSelectionToFile(
      selection,
      {
        aspectRatio: 16 / 5,
        outputWidth: 1600,
        outputHeight: 500,
        quality: 0.93,
        mimeType: 'image/webp',
      },
      'channel-cover',
    );

    expect(output.name).toMatch(/^channel-cover-\d+\.webp$/);
    expect(output.type).toBe('image/webp');
    expect(toBlobCalls).toEqual([{ width: 1600, height: 500, type: 'image/webp', quality: 0.93 }]);
    expect(clearRectMock).toHaveBeenCalledWith(0, 0, 1600, 500);
    expect(fillRectMock).toHaveBeenCalledWith(0, 0, 1600, 500);
    expect(drawImageMock).toHaveBeenCalledWith(
      expect.any(MockImage),
      2000,
      0,
      2000,
      625,
      0,
      0,
      1600,
      500,
    );
  });
});
