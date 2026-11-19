package media

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"strings"

	xdraw "golang.org/x/image/draw"
)

const (
	thumbMaxEdge   = 320
	previewMaxEdge = 1280
	jpegQuality    = 85
)

// Derivative is an encoded JPEG plus dimensions.
type Derivative struct {
	JPEG   []byte
	Width  int
	Height int
}

// BuildImageDerivatives decodes an image and produces thumb + preview JPEGs.
func BuildImageDerivatives(r io.Reader) (origW, origH int, thumb, preview Derivative, err error) {
	img, _, err := image.Decode(r)
	if err != nil {
		return 0, 0, Derivative{}, Derivative{}, fmt.Errorf("decode image: %w", err)
	}
	b := img.Bounds()
	origW, origH = b.Dx(), b.Dy()
	thumb, err = resizeJPEG(img, thumbMaxEdge)
	if err != nil {
		return 0, 0, Derivative{}, Derivative{}, err
	}
	preview, err = resizeJPEG(img, previewMaxEdge)
	if err != nil {
		return 0, 0, Derivative{}, Derivative{}, err
	}
	return origW, origH, thumb, preview, nil
}

// BuildVideoStubDerivatives returns placeholder thumb/preview JPEGs for videos.
func BuildVideoStubDerivatives() (thumb, preview Derivative, err error) {
	thumb, err = solidJPEG(thumbMaxEdge, thumbMaxEdge*3/4, color.RGBA{R: 40, G: 50, B: 55, A: 255})
	if err != nil {
		return Derivative{}, Derivative{}, err
	}
	preview, err = solidJPEG(previewMaxEdge, previewMaxEdge*3/4, color.RGBA{R: 40, G: 50, B: 55, A: 255})
	return thumb, preview, err
}

func resizeJPEG(src image.Image, maxEdge int) (Derivative, error) {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	nw, nh := fit(w, h, maxEdge)
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return Derivative{}, err
	}
	return Derivative{JPEG: buf.Bytes(), Width: nw, Height: nh}, nil
}

func solidJPEG(w, h int, c color.Color) (Derivative, error) {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: c}, image.Point{}, draw.Src)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		return Derivative{}, err
	}
	return Derivative{JPEG: buf.Bytes(), Width: w, Height: h}, nil
}

func fit(w, h, maxEdge int) (int, int) {
	if w <= 0 || h <= 0 {
		return maxEdge, maxEdge
	}
	if w <= maxEdge && h <= maxEdge {
		return w, h
	}
	if w >= h {
		return maxEdge, max(1, h*maxEdge/w)
	}
	return max(1, w*maxEdge/h), maxEdge
}

func classifyContentType(ct, filename string) (Kind, error) {
	ct = strings.ToLower(strings.TrimSpace(strings.Split(ct, ";")[0]))
	name := strings.ToLower(filename)
	switch {
	case strings.HasPrefix(ct, "image/jpeg"), strings.HasPrefix(ct, "image/png"),
		strings.HasPrefix(ct, "image/gif"), strings.HasPrefix(ct, "image/webp"),
		strings.HasSuffix(name, ".jpg"), strings.HasSuffix(name, ".jpeg"),
		strings.HasSuffix(name, ".png"), strings.HasSuffix(name, ".gif"):
		return KindPhoto, nil
	case strings.HasPrefix(ct, "video/"),
		strings.HasSuffix(name, ".mp4"), strings.HasSuffix(name, ".mov"),
		strings.HasSuffix(name, ".webm"), strings.HasSuffix(name, ".m4v"):
		return KindVideo, nil
	default:
		return "", fmt.Errorf("unsupported content type %q", ct)
	}
}
