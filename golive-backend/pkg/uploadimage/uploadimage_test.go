package uploadimage

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveOptimizedResizesLargeImagesAsJpeg(t *testing.T) {
	file := multipartFileHeader(t, "cover.png", "image/png", pngBytes(t, 400, 200))
	dir := t.TempDir()

	name, err := SaveOptimized(file, dir, "cover", ".png", Options{
		MaxWidth:  100,
		MaxHeight: 100,
		Quality:   97,
	})
	if err != nil {
		t.Fatalf("SaveOptimized returned error: %v", err)
	}
	if name != "cover.jpg" {
		t.Fatalf("SaveOptimized name = %q, want cover.jpg", name)
	}

	saved, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("open optimized file: %v", err)
	}
	defer saved.Close()

	img, format, err := image.Decode(saved)
	if err != nil {
		t.Fatalf("decode optimized file: %v", err)
	}
	if format != "jpeg" {
		t.Fatalf("optimized format = %q, want jpeg", format)
	}
	if got := img.Bounds().Size(); got.X != 100 || got.Y != 50 {
		t.Fatalf("optimized size = %dx%d, want 100x50", got.X, got.Y)
	}
}

func TestSaveOptimizedKeepsSmallOriginalWhenReencodingWouldNotHelp(t *testing.T) {
	original := pngBytes(t, 16, 16)
	file := multipartFileHeader(t, "avatar.png", "image/png", original)
	dir := t.TempDir()

	name, err := SaveOptimized(file, dir, "avatar", ".png", Options{
		MaxWidth:  512,
		MaxHeight: 512,
		Quality:   94,
	})
	if err != nil {
		t.Fatalf("SaveOptimized returned error: %v", err)
	}
	if name != "avatar.png" {
		t.Fatalf("SaveOptimized name = %q, want avatar.png", name)
	}

	saved, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read saved original: %v", err)
	}
	if !bytes.Equal(saved, original) {
		t.Fatalf("small original was unexpectedly rewritten")
	}
}

func TestSaveOptimizedRejectsUnsupportedUploads(t *testing.T) {
	original := []byte("not an image payload")
	file := multipartFileHeader(t, "upload.png", "image/png", original)
	dir := t.TempDir()

	name, err := SaveOptimized(file, dir, "upload", ".png", Options{
		MaxWidth:  512,
		MaxHeight: 512,
		Quality:   92,
	})
	if !errors.Is(err, ErrUnsupportedType) {
		t.Fatalf("SaveOptimized error = %v, want ErrUnsupportedType", err)
	}
	if name != "" {
		t.Fatalf("name = %q, want empty", name)
	}
}

func TestSaveOptimizedRejectsMalformedImagesWithValidMagic(t *testing.T) {
	original := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 'b', 'a', 'd'}
	file := multipartFileHeader(t, "upload.png", "image/png", original)
	dir := t.TempDir()

	name, err := SaveOptimized(file, dir, "upload", ".png", Options{
		MaxWidth:  512,
		MaxHeight: 512,
		Quality:   92,
	})
	if !errors.Is(err, ErrInvalidImage) {
		t.Fatalf("SaveOptimized error = %v, want ErrInvalidImage", err)
	}
	if name != "" {
		t.Fatalf("name = %q, want empty", name)
	}
}

func TestSaveOptimizedRejectsImagesAbovePixelLimit(t *testing.T) {
	file := multipartFileHeader(t, "cover.png", "image/png", pngBytes(t, 200, 200))
	dir := t.TempDir()

	name, err := SaveOptimized(file, dir, "cover", ".png", Options{
		MaxWidth:  100,
		MaxHeight: 100,
		MaxPixels: 10_000,
		Quality:   92,
	})
	if !errors.Is(err, ErrInvalidImage) {
		t.Fatalf("SaveOptimized error = %v, want ErrInvalidImage", err)
	}
	if name != "" {
		t.Fatalf("name = %q, want empty", name)
	}
}

func TestSaveOptimizedUsesDetectedFormatInsteadOfHeader(t *testing.T) {
	original := jpegBytes(t, 16, 16)
	file := multipartFileHeader(t, "avatar.png", "image/png", original)
	dir := t.TempDir()

	name, err := SaveOptimized(file, dir, "avatar", ".png", Options{
		MaxWidth:  512,
		MaxHeight: 512,
		Quality:   94,
	})
	if err != nil {
		t.Fatalf("SaveOptimized returned error: %v", err)
	}
	if name != "avatar.jpg" {
		t.Fatalf("SaveOptimized name = %q, want avatar.jpg", name)
	}
}

func TestFitDimensionsOnlyShrinksInsideBounds(t *testing.T) {
	tests := []struct {
		name      string
		width     int
		height    int
		maxWidth  int
		maxHeight int
		wantW     int
		wantH     int
	}{
		{name: "wide", width: 400, height: 200, maxWidth: 100, maxHeight: 100, wantW: 100, wantH: 50},
		{name: "tall", width: 200, height: 400, maxWidth: 100, maxHeight: 100, wantW: 50, wantH: 100},
		{name: "already fits", width: 80, height: 40, maxWidth: 100, maxHeight: 100, wantW: 80, wantH: 40},
		{name: "invalid width", width: 0, height: 40, maxWidth: 100, maxHeight: 100, wantW: 0, wantH: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotW, gotH := fitDimensions(tt.width, tt.height, tt.maxWidth, tt.maxHeight)
			if gotW != tt.wantW || gotH != tt.wantH {
				t.Fatalf("fitDimensions() = %dx%d, want %dx%d", gotW, gotH, tt.wantW, tt.wantH)
			}
		})
	}
}

func multipartFileHeader(t *testing.T, filename, contentType string, data []byte) *multipart.FileHeader {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreatePart(textproto.MIMEHeader{
		"Content-Disposition": {`form-data; name="file"; filename="` + filename + `"`},
		"Content-Type":        {contentType},
	})
	if err != nil {
		t.Fatalf("create multipart part: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write multipart part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if err := req.ParseMultipartForm(int64(body.Len())); err != nil {
		t.Fatalf("parse multipart form: %v", err)
	}
	files := req.MultipartForm.File["file"]
	if len(files) != 1 {
		t.Fatalf("multipart form files = %d, want 1", len(files))
	}
	return files[0]
}

func pngBytes(t *testing.T, width, height int) []byte {
	t.Helper()

	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.NRGBA{
				R: uint8((x * 13) % 255),
				G: uint8((y * 17) % 255),
				B: uint8((x + y) % 255),
				A: 255,
			})
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func jpegBytes(t *testing.T, width, height int) []byte {
	t.Helper()

	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.NRGBA{
				R: uint8((x * 11) % 255),
				G: uint8((y * 19) % 255),
				B: uint8((x + y*2) % 255),
				A: 255,
			})
		}
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}
