/* eslint-disable react-refresh/only-export-components */
import {
  type ChangeEvent,
  type CSSProperties,
  type PointerEvent as ReactPointerEvent,
  type ReactNode,
  type WheelEvent as ReactWheelEvent,
  useCallback,
  useEffect,
  useRef,
  useState,
} from 'react';
import { ImagePlus } from 'lucide-react';
import { cn } from '@/lib/cn';

const MAX_ZOOM = 4;

export interface ImageCropConfig {
  aspectRatio: number;
  outputWidth: number;
  outputHeight: number;
  quality: number;
  mimeType?: 'image/jpeg' | 'image/webp';
}

export interface ImageCropSelection {
  file: File;
  url: string;
  naturalWidth: number;
  naturalHeight: number;
  zoom: number;
  offsetX: number;
  offsetY: number;
}

interface ImageCropFieldProps {
  accept: string;
  active: boolean;
  className?: string;
  config: ImageCropConfig;
  disabled?: boolean;
  emptyLabel: string;
  fallback?: ReactNode;
  pickLabel: string;
  shape?: 'circle' | 'rect';
  validateFile: (file: File) => string | null;
  onError: (message: string | null) => void;
  onSelectionChange: (selection: ImageCropSelection | null) => void;
}

interface CropRect {
  x: number;
  y: number;
  width: number;
  height: number;
}

interface EditorGeometry {
  imageLeft: number;
  imageTop: number;
  imageWidth: number;
  imageHeight: number;
  scale: number;
  frameLeft: number;
  frameTop: number;
  frameWidth: number;
  frameHeight: number;
}

interface DragState {
  pointerId: number;
  startX: number;
  startY: number;
  crop: CropRect;
  scale: number;
}

export function ImageCropField({
  accept,
  active,
  className,
  config,
  disabled,
  emptyLabel,
  fallback,
  pickLabel,
  shape = 'rect',
  validateFile,
  onError,
  onSelectionChange,
}: ImageCropFieldProps) {
  const inputRef = useRef<HTMLInputElement | null>(null);
  const stageRef = useRef<HTMLDivElement | null>(null);
  const imageRef = useRef<HTMLImageElement | null>(null);
  const fileUrlRef = useRef('');
  const dragRef = useRef<DragState | null>(null);
  const [selection, setSelection] = useState<ImageCropSelection | null>(null);
  const [stageSize, setStageSize] = useState({ width: 0, height: 0 });
  const [dragging, setDragging] = useState(false);

  const clearSelection = useCallback(() => {
    imageRef.current = null;
    dragRef.current = null;
    setDragging(false);
    setSelection(null);
    onSelectionChange(null);
    if (fileUrlRef.current) {
      URL.revokeObjectURL(fileUrlRef.current);
      fileUrlRef.current = '';
    }
  }, [onSelectionChange]);

  useEffect(() => {
    if (!active) {
      clearSelection();
      onError(null);
    }
  }, [active, clearSelection, onError]);

  useEffect(() => {
    onSelectionChange(selection);
  }, [onSelectionChange, selection]);

  useEffect(() => () => clearSelection(), [clearSelection]);

  const selectionUrl = selection?.url;

  useEffect(() => {
    const stage = stageRef.current;
    if (!stage || !selectionUrl) {
      setStageSize({ width: 0, height: 0 });
      return;
    }

    const updateSize = () => {
      const rect = stage.getBoundingClientRect();
      setStageSize({
        width: rect.width,
        height: rect.height,
      });
    };

    updateSize();
    const observer = typeof ResizeObserver !== 'undefined' ? new ResizeObserver(updateSize) : null;
    observer?.observe(stage);
    window.addEventListener('resize', updateSize);

    return () => {
      observer?.disconnect();
      window.removeEventListener('resize', updateSize);
    };
  }, [selectionUrl]);

  const editorGeometry = selection
    ? resolveEditorGeometry(selection, config.aspectRatio, stageSize.width, stageSize.height)
    : null;

  const imageStyle: CSSProperties | undefined = editorGeometry
    ? {
        left: editorGeometry.imageLeft,
        top: editorGeometry.imageTop,
        width: editorGeometry.imageWidth,
        height: editorGeometry.imageHeight,
      }
    : undefined;

  const frameStyle: CSSProperties | undefined = editorGeometry
    ? {
        left: editorGeometry.frameLeft,
        top: editorGeometry.frameTop,
        width: editorGeometry.frameWidth,
        height: editorGeometry.frameHeight,
      }
    : undefined;

  const openPicker = () => {
    if (!disabled) inputRef.current?.click();
  };

  const handleFile = (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0] ?? null;
    event.target.value = '';
    onError(null);

    if (!file) {
      clearSelection();
      return;
    }

    const validationError = validateFile(file);
    if (validationError) {
      clearSelection();
      onError(validationError);
      return;
    }

    if (fileUrlRef.current) URL.revokeObjectURL(fileUrlRef.current);
    const url = URL.createObjectURL(file);
    fileUrlRef.current = url;

    const image = new Image();
    image.onload = () => {
      imageRef.current = image;
      setSelection({
        file,
        url,
        naturalWidth: image.naturalWidth,
        naturalHeight: image.naturalHeight,
        zoom: 1,
        offsetX: 0,
        offsetY: 0,
      });
    };
    image.onerror = () => {
      clearSelection();
      onError('Could not read this image.');
    };
    image.src = url;
  };

  const handleWheel = (event: ReactWheelEvent<HTMLDivElement>) => {
    if (!selection || disabled || !editorGeometry || editorGeometry.scale <= 0) return;
    event.preventDefault();
    event.stopPropagation();

    const stage = stageRef.current;
    if (!stage) return;
    const stageRect = stage.getBoundingClientRect();
    const pointerX =
      (event.clientX - stageRect.left - editorGeometry.imageLeft) / editorGeometry.scale;
    const pointerY =
      (event.clientY - stageRect.top - editorGeometry.imageTop) / editorGeometry.scale;
    const direction = event.deltaY < 0 ? 1.08 : 0.92;

    setSelection((current) => {
      if (!current) return current;

      const crop = resolveCrop(current, config.aspectRatio);
      const base = resolveBaseCropSize(current, config.aspectRatio);
      const nextZoom = clamp(current.zoom * direction, 1, MAX_ZOOM);
      const nextWidth = base.width / nextZoom;
      const nextHeight = base.height / nextZoom;
      const pointerInsideCrop =
        pointerX >= crop.x &&
        pointerX <= crop.x + crop.width &&
        pointerY >= crop.y &&
        pointerY <= crop.y + crop.height;
      const focalX = pointerInsideCrop ? pointerX : crop.x + crop.width / 2;
      const focalY = pointerInsideCrop ? pointerY : crop.y + crop.height / 2;
      const ratioX = crop.width > 0 ? clamp((focalX - crop.x) / crop.width, 0, 1) : 0.5;
      const ratioY = crop.height > 0 ? clamp((focalY - crop.y) / crop.height, 0, 1) : 0.5;

      return selectionFromCropRect(
        current,
        {
          x: focalX - nextWidth * ratioX,
          y: focalY - nextHeight * ratioY,
          width: nextWidth,
          height: nextHeight,
        },
        config.aspectRatio,
      );
    });
  };

  const handleFramePointerDown = (event: ReactPointerEvent<HTMLElement>) => {
    if (!selection || disabled || !editorGeometry || editorGeometry.scale <= 0) return;
    event.preventDefault();
    event.stopPropagation();

    dragRef.current = {
      pointerId: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      crop: resolveCrop(selection, config.aspectRatio),
      scale: editorGeometry.scale,
    };
    event.currentTarget.setPointerCapture(event.pointerId);
    setDragging(true);
  };

  const handleFramePointerMove = (event: ReactPointerEvent<HTMLElement>) => {
    const drag = dragRef.current;
    if (!drag || drag.pointerId !== event.pointerId) return;
    event.preventDefault();
    event.stopPropagation();

    const nextX = drag.crop.x + (event.clientX - drag.startX) / drag.scale;
    const nextY = drag.crop.y + (event.clientY - drag.startY) / drag.scale;
    setSelection((current) =>
      current
        ? selectionFromCropRect(
            current,
            {
              ...drag.crop,
              x: nextX,
              y: nextY,
            },
            config.aspectRatio,
          )
        : current,
    );
  };

  const finishFrameDrag = (event: ReactPointerEvent<HTMLElement>) => {
    const drag = dragRef.current;
    if (drag?.pointerId === event.pointerId) {
      if (event.currentTarget.hasPointerCapture(event.pointerId)) {
        event.currentTarget.releasePointerCapture(event.pointerId);
      }
      dragRef.current = null;
      setDragging(false);
    }
  };

  return (
    <div className={cn('gl-image-crop-field', selection && 'is-editing', className)}>
      <div
        ref={stageRef}
        className={cn(
          'gl-image-crop-stage',
          selection && 'is-editor',
          shape === 'circle' && !selection && 'is-circle',
        )}
        onClick={selection ? undefined : openPicker}
      >
        {selection ? (
          <div className="gl-image-crop-editor" onWheel={handleWheel}>
            <img
              src={selection.url}
              alt=""
              className="gl-image-crop-source"
              draggable={false}
              style={imageStyle ?? { visibility: 'hidden' }}
            />
            {frameStyle && (
              <button
                type="button"
                aria-label="Crop area"
                className={cn(
                  'gl-image-crop-frame',
                  shape === 'circle' && 'is-circle',
                  dragging && 'is-dragging',
                )}
                style={frameStyle}
                onPointerDown={handleFramePointerDown}
                onPointerMove={handleFramePointerMove}
                onPointerUp={finishFrameDrag}
                onPointerCancel={finishFrameDrag}
              />
            )}
          </div>
        ) : fallback ? (
          fallback
        ) : (
          <span className="gl-image-crop-empty">
            <ImagePlus size={24} />
            {emptyLabel}
          </span>
        )}
      </div>

      <button
        type="button"
        className="gl-avatar-pick-btn gl-image-crop-pick-btn"
        disabled={disabled}
        onClick={openPicker}
      >
        <ImagePlus size={16} />
        {pickLabel}
      </button>
      <input
        ref={inputRef}
        type="file"
        className="gl-image-crop-input"
        accept={accept}
        disabled={disabled}
        aria-label={pickLabel}
        onChange={handleFile}
      />
    </div>
  );
}

export async function cropSelectionToFile(
  selection: ImageCropSelection,
  config: ImageCropConfig,
  filenamePrefix: string,
): Promise<File> {
  const image = await loadImage(selection.url);
  const canvas = document.createElement('canvas');
  canvas.width = config.outputWidth;
  canvas.height = config.outputHeight;
  drawCroppedImage(canvas, image, selection, config);

  const mimeType = config.mimeType ?? 'image/jpeg';
  const blob = await new Promise<Blob>((resolve, reject) => {
    canvas.toBlob(
      (next) => {
        if (next) resolve(next);
        else reject(new Error('Could not process image.'));
      },
      mimeType,
      config.quality,
    );
  });
  const ext = mimeType === 'image/webp' ? 'webp' : 'jpg';
  return new File([blob], `${filenamePrefix}-${Date.now()}.${ext}`, {
    type: mimeType,
    lastModified: Date.now(),
  });
}

function loadImage(url: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const image = new Image();
    image.onload = () => resolve(image);
    image.onerror = () => reject(new Error('Could not load image.'));
    image.src = url;
  });
}

function drawCroppedImage(
  canvas: HTMLCanvasElement,
  image: HTMLImageElement,
  selection: ImageCropSelection,
  config: ImageCropConfig,
) {
  const ctx = canvas.getContext('2d');
  if (!ctx) return;
  const crop = resolveCrop(selection, config.aspectRatio);
  ctx.clearRect(0, 0, canvas.width, canvas.height);
  ctx.fillStyle = '#fff';
  ctx.fillRect(0, 0, canvas.width, canvas.height);
  ctx.imageSmoothingEnabled = true;
  ctx.imageSmoothingQuality = 'high';
  ctx.drawImage(image, crop.x, crop.y, crop.width, crop.height, 0, 0, canvas.width, canvas.height);
}

function resolveEditorGeometry(
  selection: ImageCropSelection,
  aspectRatio: number,
  stageWidth: number,
  stageHeight: number,
): EditorGeometry | null {
  if (stageWidth <= 0 || stageHeight <= 0) return null;

  const scale = Math.min(
    stageWidth / selection.naturalWidth,
    stageHeight / selection.naturalHeight,
  );
  if (!Number.isFinite(scale) || scale <= 0) return null;

  const imageWidth = selection.naturalWidth * scale;
  const imageHeight = selection.naturalHeight * scale;
  const imageLeft = (stageWidth - imageWidth) / 2;
  const imageTop = (stageHeight - imageHeight) / 2;
  const crop = resolveCrop(selection, aspectRatio);

  return {
    imageLeft,
    imageTop,
    imageWidth,
    imageHeight,
    scale,
    frameLeft: imageLeft + crop.x * scale,
    frameTop: imageTop + crop.y * scale,
    frameWidth: crop.width * scale,
    frameHeight: crop.height * scale,
  };
}

function resolveCrop(selection: ImageCropSelection, aspectRatio: number): CropRect {
  const base = resolveBaseCropSize(selection, aspectRatio);
  const zoom = clamp(selection.zoom, 1, MAX_ZOOM);
  const width = Math.max(1, Math.min(selection.naturalWidth, base.width / zoom));
  const height = Math.max(1, Math.min(selection.naturalHeight, base.height / zoom));
  const maxX = Math.max(0, selection.naturalWidth - width);
  const maxY = Math.max(0, selection.naturalHeight - height);
  const x = clamp(maxX / 2 + selection.offsetX * (maxX / 2), 0, maxX);
  const y = clamp(maxY / 2 + selection.offsetY * (maxY / 2), 0, maxY);

  return { x, y, width, height };
}

function resolveBaseCropSize(selection: ImageCropSelection, aspectRatio: number) {
  const sourceRatio = selection.naturalWidth / selection.naturalHeight;
  let width = selection.naturalWidth;
  let height = selection.naturalHeight;

  if (sourceRatio > aspectRatio) {
    width = selection.naturalHeight * aspectRatio;
  } else {
    height = selection.naturalWidth / aspectRatio;
  }

  return { width, height };
}

function selectionFromCropRect(
  selection: ImageCropSelection,
  crop: CropRect,
  aspectRatio: number,
): ImageCropSelection {
  const base = resolveBaseCropSize(selection, aspectRatio);
  const zoom = clamp(base.width / crop.width, 1, MAX_ZOOM);
  const width = Math.max(1, Math.min(selection.naturalWidth, base.width / zoom));
  const height = Math.max(1, Math.min(selection.naturalHeight, base.height / zoom));
  const maxX = Math.max(0, selection.naturalWidth - width);
  const maxY = Math.max(0, selection.naturalHeight - height);
  const x = clamp(crop.x, 0, maxX);
  const y = clamp(crop.y, 0, maxY);

  return {
    ...selection,
    zoom,
    offsetX: maxX > 0 ? (x - maxX / 2) / (maxX / 2) : 0,
    offsetY: maxY > 0 ? (y - maxY / 2) / (maxY / 2) : 0,
  };
}

function clamp(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, value));
}
