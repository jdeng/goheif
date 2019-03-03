package libde265

//#cgo CXXFLAGS: -Ilibde265 -I. -std=c++11 -Wno-constant-conversion -msse4.1
//#cgo CFLAGS: -I.
// #include <stdint.h>
// #include <stdlib.h>
// #include "libde265/de265.h"
// int push_data(void *dec, const void *data, size_t size);
import "C"

import (
	"fmt"
	"image"
	"unsafe"
)

type Decoder struct {
	ctx unsafe.Pointer
}

func Init() {
	C.de265_init()
}

func Fini() {
	C.de265_free()
}

func NewDecoder() *Decoder {
	if p := C.de265_new_decoder(); p != nil {
		return &Decoder{p}
	}
	return nil
}

func (dec *Decoder) Free() {
	C.de265_free_decoder(dec.ctx)
}

func (dec *Decoder) Reset() {
	C.de265_reset(dec.ctx)
}

func (dec *Decoder) Push(data []byte) error {
	if ret := C.push_data(dec.ctx, unsafe.Pointer(&data[0]), C.size_t(len(data))); ret < 0 {
		return fmt.Errorf("push_data error")
	}
	return nil
}

func (dec *Decoder) DecodeImage(data []byte) (image.Image, error) {
	if len(data) > 0 {
		if ret := C.push_data(dec.ctx, unsafe.Pointer(&data[0]), C.size_t(len(data))); ret < 0 {
			return nil, fmt.Errorf("push_data error")
		}
	}

	if ret := C.de265_flush_data(dec.ctx); ret != C.DE265_OK {
		return nil, fmt.Errorf("flush_data error")
	}

	var more C.int = 1
	for more != 0 {
		if decerr := C.de265_decode(dec.ctx, &more); decerr != C.DE265_OK {
			return nil, fmt.Errorf("decode error")
		}

		for {
			warning := C.de265_get_warning(dec.ctx)
			if warning == C.DE265_OK {
				break
			}
			fmt.Printf("warning: %v\n", C.GoString(C.de265_get_error_text(warning)))
		}

		if img := C.de265_get_next_picture(dec.ctx); img != nil {
			width := C.de265_get_image_width(img, 0)
			height := C.de265_get_image_height(img, 0)

			var ystride, cstride C.int
			y := C.de265_get_image_plane(img, 0, &ystride)
			cb := C.de265_get_image_plane(img, 1, &cstride)
			cheight := C.de265_get_image_height(img, 1)
			cr := C.de265_get_image_plane(img, 2, &cstride)
			//			crh := C.de265_get_image_height(img, 2)

			var r image.YCbCrSubsampleRatio
			switch chroma := C.de265_get_chroma_format(img); chroma {
			case C.de265_chroma_420:
				r = image.YCbCrSubsampleRatio420
			case C.de265_chroma_422:
				r = image.YCbCrSubsampleRatio422
			case C.de265_chroma_444:
				r = image.YCbCrSubsampleRatio444
			}
			ycc := &image.YCbCr{
				Y:              C.GoBytes(unsafe.Pointer(y), C.int(height*ystride)),
				Cb:             C.GoBytes(unsafe.Pointer(cb), C.int(cheight*cstride)),
				Cr:             C.GoBytes(unsafe.Pointer(cr), C.int(cheight*cstride)),
				YStride:        int(ystride),
				CStride:        int(cstride),
				SubsampleRatio: r,
				Rect:           image.Rectangle{Min: image.Point{0, 0}, Max: image.Point{int(width), int(height)}},
			}

			C.de265_release_next_picture(dec.ctx)
			return ycc, nil
		}
	}

	return nil, fmt.Errorf("No picture")
}
