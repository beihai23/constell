package main

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"strings"
)

const thumbnailWidth = 256

func isImage(contentType string) bool {
	return strings.HasPrefix(contentType, "image/jpeg") ||
		strings.HasPrefix(contentType, "image/png") ||
		strings.HasPrefix(contentType, "image/gif")
}

func generateThumbnail(data []byte, contentType string) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= thumbnailWidth {
		return data, nil
	}

	ratio := float64(thumbnailWidth) / float64(w)
	newH := int(float64(h) * ratio)
	thumb := image.NewRGBA(image.Rect(0, 0, thumbnailWidth, newH))

	for y := 0; y < newH; y++ {
		for x := 0; x < thumbnailWidth; x++ {
			sx := int(float64(x) / ratio)
			sy := int(float64(y) / ratio)
			thumb.Set(x, y, img.At(sx, sy))
		}
	}

	var buf bytes.Buffer
	if strings.Contains(contentType, "png") {
		if err := png.Encode(&buf, thumb); err != nil {
			return nil, fmt.Errorf("encode png thumbnail: %w", err)
		}
	} else {
		if err := jpeg.Encode(&buf, thumb, &jpeg.Options{Quality: 85}); err != nil {
			return nil, fmt.Errorf("encode jpeg thumbnail: %w", err)
		}
	}
	return buf.Bytes(), nil
}
