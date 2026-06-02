package main

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestIsImage(t *testing.T) {
	tests := []struct {
		contentType string
		want        bool
	}{
		{"image/jpeg", true},
		{"image/png", true},
		{"image/gif", true},
		{"image/webp", false},
		{"application/pdf", false},
		{"text/plain", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			if got := isImage(tt.contentType); got != tt.want {
				t.Errorf("isImage(%q) = %v, want %v", tt.contentType, got, tt.want)
			}
		})
	}
}

func TestGenerateThumbnail_JPEG(t *testing.T) {
	// Create a 512x512 test JPEG image.
	orig := image.NewRGBA(image.Rect(0, 0, 512, 512))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, orig, nil); err != nil {
		t.Fatalf("encode test image: %v", err)
	}
	data := buf.Bytes()

	thumb, err := generateThumbnail(data, "image/jpeg")
	if err != nil {
		t.Fatalf("generateThumbnail() error = %v", err)
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(thumb))
	if err != nil {
		t.Fatalf("decode thumbnail config: %v", err)
	}
	if cfg.Width != 256 {
		t.Errorf("thumbnail width = %d, want 256", cfg.Width)
	}
	if cfg.Height != 256 {
		t.Errorf("thumbnail height = %d, want 256", cfg.Height)
	}
}

func TestGenerateThumbnail_PNG(t *testing.T) {
	// Create a 512x256 test PNG image.
	orig := image.NewRGBA(image.Rect(0, 0, 512, 256))
	var buf bytes.Buffer
	if err := png.Encode(&buf, orig); err != nil {
		t.Fatalf("encode test image: %v", err)
	}
	data := buf.Bytes()

	thumb, err := generateThumbnail(data, "image/png")
	if err != nil {
		t.Fatalf("generateThumbnail() error = %v", err)
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(thumb))
	if err != nil {
		t.Fatalf("decode thumbnail config: %v", err)
	}
	if cfg.Width != 256 {
		t.Errorf("thumbnail width = %d, want 256", cfg.Width)
	}
	if cfg.Height != 128 {
		t.Errorf("thumbnail height = %d, want 128", cfg.Height)
	}
}

func TestGenerateThumbnail_SmallImage(t *testing.T) {
	// Create a 128x128 image — smaller than thumbnailWidth, should return original data.
	orig := image.NewRGBA(image.Rect(0, 0, 128, 128))
	var buf bytes.Buffer
	if err := png.Encode(&buf, orig); err != nil {
		t.Fatalf("encode test image: %v", err)
	}
	data := buf.Bytes()

	thumb, err := generateThumbnail(data, "image/png")
	if err != nil {
		t.Fatalf("generateThumbnail() error = %v", err)
	}

	if !bytes.Equal(thumb, data) {
		t.Error("small image should be returned as-is, but got different data")
	}
}
