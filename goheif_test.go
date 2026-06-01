package goheif

import (
	"bytes"
	"encoding/binary"
	"image"
	"io"
	"os"
	"testing"

	"github.com/jdeng/goheif/heif"
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

func TestDecodeHEVCAnnexB(t *testing.T) {
	stream, width, height := testHEVCAnnexBStream(t, "testdata/camel.heic")
	img, err := DecodeHEVCAnnexB(stream)
	if err != nil {
		t.Fatal(err)
	}
	if got := img.Bounds(); got.Dx() != width || got.Dy() != height {
		t.Fatalf("decoded image size = %dx%d, want %dx%d", got.Dx(), got.Dy(), width, height)
	}
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

func testHEVCAnnexBStream(t *testing.T, path string) ([]byte, int, int) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	hf := heif.Open(bytes.NewReader(data))
	item, err := hf.PrimaryItem()
	if err != nil {
		t.Fatal(err)
	}
	if item.Info != nil && item.Info.ItemType == "grid" {
		dimg := item.Reference("dimg")
		if dimg == nil || len(dimg.ToItemIDs) == 0 {
			t.Fatal("grid item has no dimg references")
		}
		item, err = hf.ItemByID(dimg.ToItemIDs[0])
		if err != nil {
			t.Fatal(err)
		}
	}
	if item.Info == nil || item.Info.ItemType != "hvc1" {
		t.Fatalf("test item type = %v, want hvc1", item.Info)
	}
	width, height, ok := item.SpatialExtents()
	if !ok {
		t.Fatal("test item has no spatial extents")
	}
	config, ok := item.HevcConfig()
	if !ok {
		t.Fatal("test item has no hvcC")
	}
	itemData, err := hf.GetItemData(item)
	if err != nil {
		t.Fatal(err)
	}

	var sample bytes.Buffer
	sample.Write(config.AsHeader())
	sample.Write(itemData)
	return lengthPrefixedNALSampleToAnnexB(t, sample.Bytes()), width, height
}

func lengthPrefixedNALSampleToAnnexB(t *testing.T, sample []byte) []byte {
	t.Helper()

	var out bytes.Buffer
	for len(sample) > 0 {
		if len(sample) < 4 {
			t.Fatalf("truncated nal length: %d bytes left", len(sample))
		}
		size := int(binary.BigEndian.Uint32(sample[:4]))
		sample = sample[4:]
		if size <= 0 || size > len(sample) {
			t.Fatalf("invalid nal size %d with %d bytes left", size, len(sample))
		}
		out.Write([]byte{0, 0, 0, 1})
		out.Write(sample[:size])
		sample = sample[size:]
	}
	return out.Bytes()
}
