package goheif

import (
	"bytes"
	"image"
	"io"
	"os"
	"testing"
)

func TestFormatRegistered(t *testing.T) {
	b, err := os.ReadFile("testdata/camel.heic")
	if err != nil {
		t.Fatal(err)
	}

	img, dec, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("unable to decode heic image: %s", err)
	}

	if got, want := dec, "heic"; got != want {
		t.Errorf("unexpected decoder: got %s, want %s", got, want)
	}

	if w, h := img.Bounds().Dx(), img.Bounds().Dy(); w != 1596 || h != 1064 {
		t.Errorf("unexpected decoded image size: got %dx%d, want 1596x1064", w, h)
	}

	t.Logf("Successfully decoded HEIC image: %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
}

func TestDecodeAVIF(t *testing.T) {
	file, err := os.Open("testdata/fox.avif")
	if err != nil {
		t.Skipf("Test AVIF file not found: %v", err)
	}
	defer file.Close()

	// Decode using the main goheif package with AV1 support
	img, err := Decode(file)
	if err != nil {
		t.Fatalf("Failed to decode AVIF image: %v", err)
	}

	// Check that we got a valid image
	if img == nil {
		t.Fatal("Decoded image is nil")
	}

	bounds := img.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		t.Fatalf("Invalid image dimensions: %dx%d", bounds.Dx(), bounds.Dy())
	}

	t.Logf("Successfully decoded AVIF image: %dx%d", bounds.Dx(), bounds.Dy())
}

func BenchmarkSafeEncoding(b *testing.B) {
	benchEncoding(b, true)
}

func BenchmarkRegularEncoding(b *testing.B) {
	benchEncoding(b, false)
}

func benchEncoding(b *testing.B, safe bool) {
	b.Helper()

	currentSetting := SafeEncoding
	defer func() {
		SafeEncoding = currentSetting
	}()
	SafeEncoding = safe

	f, err := os.ReadFile("testdata/camel.heic")
	if err != nil {
		b.Fatal(err)
	}
	r := bytes.NewReader(f)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Decode(r)
		r.Seek(0, io.SeekStart)
	}
}
