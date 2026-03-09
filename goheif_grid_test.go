package goheif

import (
	"image"
	"reflect"
	"testing"
)

func TestStitchYCbCrTileUsesVisibleTileWidth(t *testing.T) {
	dst := image.NewYCbCr(image.Rect(0, 0, 8, 2), image.YCbCrSubsampleRatio420)
	src := &image.YCbCr{
		Y:              []uint8{1, 2, 3, 4, 200, 201, 5, 6, 7, 8, 202, 203},
		Cb:             []uint8{9, 10, 204, 205},
		Cr:             []uint8{11, 12, 206, 207},
		YStride:        6,
		CStride:        4,
		SubsampleRatio: image.YCbCrSubsampleRatio420,
		Rect:           image.Rect(0, 0, 4, 2),
	}

	if err := stitchYCbCrTile(dst, src, 1, 0); err != nil {
		t.Fatalf("stitchYCbCrTile returned error: %v", err)
	}

	wantY := []uint8{
		0, 0, 0, 0, 1, 2, 3, 4,
		0, 0, 0, 0, 5, 6, 7, 8,
	}
	if !reflect.DeepEqual(dst.Y, wantY) {
		t.Fatalf("unexpected Y plane: got %v want %v", dst.Y, wantY)
	}

	wantCb := []uint8{0, 0, 9, 10}
	if !reflect.DeepEqual(dst.Cb, wantCb) {
		t.Fatalf("unexpected Cb plane: got %v want %v", dst.Cb, wantCb)
	}

	wantCr := []uint8{0, 0, 11, 12}
	if !reflect.DeepEqual(dst.Cr, wantCr) {
		t.Fatalf("unexpected Cr plane: got %v want %v", dst.Cr, wantCr)
	}
}

func TestStitchYCbCrTileRejectsSubsampleMismatch(t *testing.T) {
	dst := image.NewYCbCr(image.Rect(0, 0, 4, 2), image.YCbCrSubsampleRatio420)
	src := image.NewYCbCr(image.Rect(0, 0, 4, 2), image.YCbCrSubsampleRatio444)

	err := stitchYCbCrTile(dst, src, 0, 0)
	if err == nil {
		t.Fatal("expected stitchYCbCrTile to reject mismatched subsampling")
	}
}
