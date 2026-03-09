package libde265

import (
	"encoding/binary"
	"reflect"
	"testing"
)

func appendUint16(dst []byte, value uint16) []byte {
	buf := make([]byte, 2)
	nativeEndian.PutUint16(buf, value)
	return append(dst, buf...)
}

func TestUnpackHighBitDepthPlane10Bit(t *testing.T) {
	row1 := []uint16{0, 4, 512, 1023}
	row2 := []uint16{16, 64, 768, 900}
	src := make([]byte, 0, 2*(len(row1)+len(row2)))
	for _, sample := range append(row1, row2...) {
		src = appendUint16(src, sample)
	}

	got, stride, err := unpackHighBitDepthPlane(src, 2, 8, 10)
	if err != nil {
		t.Fatalf("unpackHighBitDepthPlane returned error: %v", err)
	}

	if stride != 4 {
		t.Fatalf("unexpected stride: got %d want 4", stride)
	}

	want := []byte{
		0, 1, 128, 255,
		4, 16, 192, 225,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected plane bytes: got %v want %v", got, want)
	}
}

func TestUnpackHighBitDepthPlaneRejectsMisalignedStride(t *testing.T) {
	src := make([]byte, 9)
	binary.LittleEndian.PutUint16(src[:2], 1023)

	_, _, err := unpackHighBitDepthPlane(src, 1, 9, 10)
	if err == nil {
		t.Fatal("expected unpackHighBitDepthPlane to reject a misaligned stride")
	}
}
