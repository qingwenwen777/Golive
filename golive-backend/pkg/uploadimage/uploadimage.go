package uploadimage

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	stddraw "image/draw"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	_ "golang.org/x/image/webp"
)

var (
	ErrUnsupportedType = errors.New("unsupported image type")
	ErrInvalidImage    = errors.New("invalid image")
	ErrTooLarge        = errors.New("image too large")
)

const defaultMaxPixels int64 = 20_000_000
const defaultMaxUploadBytes int64 = 10 << 20
const multipartBodyOverheadBytes int64 = 1 << 20

type Options struct {
	MaxWidth  int
	MaxHeight int
	MaxPixels int64
	MaxBytes  int64
	Quality   int
}

func LimitRequestBody(w http.ResponseWriter, r *http.Request, maxFileBytes int64) {
	if maxFileBytes <= 0 {
		maxFileBytes = defaultMaxUploadBytes
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxFileBytes+multipartBodyOverheadBytes)
}

func SaveOptimized(file *multipart.FileHeader, dir, basename, _ string, opts Options) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	data, err := readUpload(file, opts.MaxBytes)
	if err != nil {
		return "", err
	}

	ext, ok := detectAllowedExt(data)
	if !ok {
		return "", ErrUnsupportedType
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", ErrInvalidImage
	}
	maxPixels := opts.MaxPixels
	if maxPixels <= 0 {
		if opts.MaxWidth > 0 && opts.MaxHeight > 0 {
			maxPixels = int64(opts.MaxWidth) * int64(opts.MaxHeight) * 16
		} else {
			maxPixels = defaultMaxPixels
		}
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > maxPixels {
		return "", ErrInvalidImage
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", ErrInvalidImage
	}

	maxWidth, maxHeight := opts.MaxWidth, opts.MaxHeight
	if maxWidth <= 0 {
		maxWidth = img.Bounds().Dx()
	}
	if maxHeight <= 0 {
		maxHeight = img.Bounds().Dy()
	}
	width, height := fitDimensions(img.Bounds().Dx(), img.Bounds().Dy(), maxWidth, maxHeight)
	if width <= 0 || height <= 0 {
		return "", ErrInvalidImage
	}

	resized := resizeOverWhite(img, width, height)
	quality := opts.Quality
	if quality <= 0 {
		quality = 92
	}
	if quality > 95 {
		quality = 95
	}

	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, resized, &jpeg.Options{Quality: quality}); err != nil {
		return "", err
	}

	originalFits := img.Bounds().Dx() <= maxWidth && img.Bounds().Dy() <= maxHeight
	if originalFits && encoded.Len() >= int(float64(len(data))*0.8) {
		name := basename + ext
		return name, os.WriteFile(filepath.Join(dir, name), data, 0o644)
	}

	name := basename + ".jpg"
	return name, os.WriteFile(filepath.Join(dir, name), encoded.Bytes(), 0o644)
}

func readUpload(file *multipart.FileHeader, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		maxBytes = defaultMaxUploadBytes
	}
	if file.Size > maxBytes {
		return nil, ErrTooLarge
	}
	src, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer src.Close()
	data, err := io.ReadAll(io.LimitReader(src, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, ErrTooLarge
	}
	return data, nil
}

func IsInvalidUpload(err error) bool {
	return errors.Is(err, ErrUnsupportedType) || errors.Is(err, ErrInvalidImage)
}

func IsTooLarge(err error) bool {
	var maxBytesErr *http.MaxBytesError
	return errors.Is(err, ErrTooLarge) || errors.As(err, &maxBytesErr)
}

func detectAllowedExt(data []byte) (string, bool) {
	switch {
	case len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff:
		return ".jpg", true
	case len(data) >= 8 && bytes.Equal(data[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}):
		return ".png", true
	case len(data) >= 6 && (bytes.Equal(data[:6], []byte("GIF87a")) || bytes.Equal(data[:6], []byte("GIF89a"))):
		return ".gif", true
	case len(data) >= 12 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return ".webp", true
	default:
		return "", false
	}
}

func fitDimensions(width, height, maxWidth, maxHeight int) (int, int) {
	if width <= 0 || height <= 0 {
		return 0, 0
	}
	scale := math.Min(float64(maxWidth)/float64(width), float64(maxHeight)/float64(height))
	if scale >= 1 {
		return width, height
	}
	return max(1, int(math.Round(float64(width)*scale))), max(1, int(math.Round(float64(height)*scale)))
}

func resizeOverWhite(src image.Image, width, height int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	stddraw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, stddraw.Src)

	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	for y := 0; y < height; y++ {
		sy := (float64(y)+0.5)*float64(sh)/float64(height) - 0.5
		y0 := clampInt(int(math.Floor(sy)), 0, sh-1)
		y1 := clampInt(y0+1, 0, sh-1)
		fy := sy - float64(y0)
		for x := 0; x < width; x++ {
			sx := (float64(x)+0.5)*float64(sw)/float64(width) - 0.5
			x0 := clampInt(int(math.Floor(sx)), 0, sw-1)
			x1 := clampInt(x0+1, 0, sw-1)
			fx := sx - float64(x0)
			dst.Set(x, y, mixOverWhite(
				src.At(sb.Min.X+x0, sb.Min.Y+y0),
				src.At(sb.Min.X+x1, sb.Min.Y+y0),
				src.At(sb.Min.X+x0, sb.Min.Y+y1),
				src.At(sb.Min.X+x1, sb.Min.Y+y1),
				fx,
				fy,
			))
		}
	}
	return dst
}

func mixOverWhite(c00, c10, c01, c11 color.Color, fx, fy float64) color.RGBA {
	r00, g00, b00, a00 := rgba01(c00)
	r10, g10, b10, a10 := rgba01(c10)
	r01, g01, b01, a01 := rgba01(c01)
	r11, g11, b11, a11 := rgba01(c11)

	r := bilerp(r00, r10, r01, r11, fx, fy)
	g := bilerp(g00, g10, g01, g11, fx, fy)
	b := bilerp(b00, b10, b01, b11, fx, fy)
	a := bilerp(a00, a10, a01, a11, fx, fy)

	return color.RGBA{
		R: byte(clampFloat((r+(1-a))*255, 0, 255)),
		G: byte(clampFloat((g+(1-a))*255, 0, 255)),
		B: byte(clampFloat((b+(1-a))*255, 0, 255)),
		A: 255,
	}
}

func rgba01(c color.Color) (float64, float64, float64, float64) {
	r, g, b, a := c.RGBA()
	return float64(r) / 65535, float64(g) / 65535, float64(b) / 65535, float64(a) / 65535
}

func bilerp(v00, v10, v01, v11, fx, fy float64) float64 {
	top := v00 + (v10-v00)*fx
	bottom := v01 + (v11-v01)*fx
	return top + (bottom-top)*fy
}

func clampInt(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func clampFloat(value, minValue, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
