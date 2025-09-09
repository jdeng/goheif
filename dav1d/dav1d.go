package dav1d

/*
#cgo CFLAGS: -I. -I./include -I./include/dav1d -I./src -fvisibility=hidden -std=c99
#cgo linux CFLAGS: -D_GNU_SOURCE
#cgo darwin CFLAGS: -fno-stack-check
#cgo windows CFLAGS: -D_WIN32_WINNT=0x0601 -DUNICODE=1 -D_UNICODE=1 -D__USE_MINGW_ANSI_STDIO=1
#cgo amd64 CFLAGS: -msse2 -mfpmath=sse
#cgo arm64 CFLAGS: -fno-align-functions
#cgo LDFLAGS: -lm
#cgo !windows LDFLAGS: -lpthread

#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include "dav1d/dav1d.h"
#include "dav1d/data.h"
#include "dav1d/picture.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"image"
	"unsafe"
)

type Decoder struct {
	ctx        *C.Dav1dContext
	hasPicture bool
	safeEncode bool
}

type Option func(*Decoder)

func WithSafeEncoding(b bool) Option {
	return func(dec *Decoder) {
		dec.safeEncode = b
	}
}

func NewDecoder(opts ...Option) (*Decoder, error) {
	var settings C.Dav1dSettings
	C.dav1d_default_settings(&settings)

	var ctx *C.Dav1dContext
	if ret := C.dav1d_open(&ctx, &settings); ret < 0 {
		return nil, fmt.Errorf("unable to create decoder: %d", ret)
	}

	dec := &Decoder{ctx: ctx, hasPicture: false}
	for _, opt := range opts {
		opt(dec)
	}

	return dec, nil
}

func (dec *Decoder) Free() {
	dec.Reset()
	if dec.ctx != nil {
		C.dav1d_close(&dec.ctx)
		dec.ctx = nil
	}
}

func (dec *Decoder) Reset() {
	if dec.ctx != nil {
		C.dav1d_flush(dec.ctx)
		dec.hasPicture = false
	}
}

func (dec *Decoder) DecodeImage(data []byte) (image.Image, error) {
	if dec.hasPicture {
		fmt.Printf("previous image may leak")
	}

	if len(data) == 0 {
		return nil, errors.New("no data provided")
	}

	// Create Dav1dData structure
	var dav1dData C.Dav1dData
	dataPtr := C.dav1d_data_create(&dav1dData, C.size_t(len(data)))
	if dataPtr == nil {
		return nil, errors.New("failed to allocate data")
	}
	defer C.dav1d_data_unref(&dav1dData)

	// Copy data to allocated buffer
	C.memcpy(unsafe.Pointer(dataPtr), unsafe.Pointer(&data[0]), C.size_t(len(data)))

	// Send data to decoder - handle EAGAIN properly
	for {
		ret := C.dav1d_send_data(dec.ctx, &dav1dData)
		if ret == 0 {
			break // Data consumed successfully
		}
		if ret == -11 { // DAV1D_ERR(EAGAIN) - decoder buffer full, need to get pictures first
			// Try to get a picture to free up decoder buffer
			var tempPicture C.Dav1dPicture
			picRet := C.dav1d_get_picture(dec.ctx, &tempPicture)
			if picRet == 0 {
				C.dav1d_picture_unref(&tempPicture) // We don't need this intermediate picture
				continue                            // Try sending data again
			} else if picRet != -11 { // Not EAGAIN, real error
				return nil, fmt.Errorf("intermediate get_picture error: %d", picRet)
			}
			// If picRet == EAGAIN, continue the loop to try send_data again
		} else if ret < 0 {
			return nil, fmt.Errorf("send_data error: %d", ret)
		}
	}

	// Get decoded picture - try multiple times as decoder may need time to process
	var picture C.Dav1dPicture
	maxRetries := 10 // Prevent infinite loops
	for i := 0; i < maxRetries; i++ {
		ret := C.dav1d_get_picture(dec.ctx, &picture)
		if ret == 0 {
			break // Successfully got picture
		}
		if ret == -11 { // DAV1D_ERR(EAGAIN) - not enough data yet, but this is normal
			if i == maxRetries-1 {
				return nil, fmt.Errorf("decoder unable to produce picture after %d attempts (may need more data)", maxRetries)
			}
			continue // Try again
		}
		return nil, fmt.Errorf("get_picture error: %d", ret)
	}
	defer C.dav1d_picture_unref(&picture)

	dec.hasPicture = true

	// Extract image dimensions and properties
	width := int(picture.p.w)
	height := int(picture.p.h)
	bpc := int(picture.p.bpc)
	layout := picture.p.layout

	if bpc != 8 && bpc != 10 {
		return nil, fmt.Errorf("unsupported bit depth: %d", bpc)
	}

	// Convert to Go image based on pixel layout
	switch layout {
	case C.DAV1D_PIXEL_LAYOUT_I400:
		return dec.convertGrayscale(&picture, width, height, bpc)
	case C.DAV1D_PIXEL_LAYOUT_I420:
		return dec.convertYCbCr(&picture, width, height, bpc, image.YCbCrSubsampleRatio420)
	case C.DAV1D_PIXEL_LAYOUT_I422:
		return dec.convertYCbCr(&picture, width, height, bpc, image.YCbCrSubsampleRatio422)
	case C.DAV1D_PIXEL_LAYOUT_I444:
		return dec.convertYCbCr(&picture, width, height, bpc, image.YCbCrSubsampleRatio444)
	default:
		return nil, fmt.Errorf("unsupported pixel layout: %d", layout)
	}
}

func (dec *Decoder) convertGrayscale(picture *C.Dav1dPicture, width, height, bpc int) (image.Image, error) {
	yStride := int(picture.stride[0])
	yData := picture.data[0]

	if bpc == 8 {
		gray := &image.Gray{
			Pix:    make([]uint8, width*height),
			Stride: width,
			Rect:   image.Rectangle{Min: image.Point{0, 0}, Max: image.Point{width, height}},
		}

		if dec.safeEncode {
			C.memcpy(unsafe.Pointer(&gray.Pix[0]), yData, C.size_t(len(gray.Pix)))
		} else {
			for y := 0; y < height; y++ {
				srcPtr := unsafe.Pointer(uintptr(yData) + uintptr(y*yStride))
				dstPtr := unsafe.Pointer(&gray.Pix[y*width])
				C.memcpy(dstPtr, srcPtr, C.size_t(width))
			}
		}
		return gray, nil
	} else {
		// 10-bit grayscale - convert to Gray16
		gray16 := &image.Gray16{
			Pix:    make([]uint8, width*height*2),
			Stride: width * 2,
			Rect:   image.Rectangle{Min: image.Point{0, 0}, Max: image.Point{width, height}},
		}

		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				srcPtr := (*uint16)(unsafe.Pointer(uintptr(yData) + uintptr(y*yStride+x*2)))
				val := *srcPtr << 6 // Scale 10-bit to 16-bit
				gray16.Pix[(y*width+x)*2] = uint8(val >> 8)
				gray16.Pix[(y*width+x)*2+1] = uint8(val & 0xff)
			}
		}
		return gray16, nil
	}
}

func (dec *Decoder) convertYCbCr(picture *C.Dav1dPicture, width, height, bpc int, subsample image.YCbCrSubsampleRatio) (image.Image, error) {
	yStride := int(picture.stride[0])
	cStride := int(picture.stride[1])

	yData := picture.data[0]
	cbData := picture.data[1]
	crData := picture.data[2]

	// Calculate chroma dimensions
	var cWidth, cHeight int
	switch subsample {
	case image.YCbCrSubsampleRatio420:
		cWidth = (width + 1) / 2
		cHeight = (height + 1) / 2
	case image.YCbCrSubsampleRatio422:
		cWidth = (width + 1) / 2
		cHeight = height
	case image.YCbCrSubsampleRatio444:
		cWidth = width
		cHeight = height
	default:
		return nil, fmt.Errorf("unsupported subsample ratio")
	}

	if bpc == 8 {
		ycc := &image.YCbCr{
			YStride:        width,
			CStride:        cWidth,
			SubsampleRatio: subsample,
			Rect:           image.Rectangle{Min: image.Point{0, 0}, Max: image.Point{width, height}},
		}

		if dec.safeEncode {
			// Copy data safely
			ycc.Y = make([]uint8, width*height)
			ycc.Cb = make([]uint8, cWidth*cHeight)
			ycc.Cr = make([]uint8, cWidth*cHeight)

			for y := 0; y < height; y++ {
				srcPtr := unsafe.Pointer(uintptr(yData) + uintptr(y*yStride))
				dstPtr := unsafe.Pointer(&ycc.Y[y*width])
				C.memcpy(dstPtr, srcPtr, C.size_t(width))
			}

			for y := 0; y < cHeight; y++ {
				cbSrcPtr := unsafe.Pointer(uintptr(cbData) + uintptr(y*cStride))
				cbDstPtr := unsafe.Pointer(&ycc.Cb[y*cWidth])
				C.memcpy(cbDstPtr, cbSrcPtr, C.size_t(cWidth))

				crSrcPtr := unsafe.Pointer(uintptr(crData) + uintptr(y*cStride))
				crDstPtr := unsafe.Pointer(&ycc.Cr[y*cWidth])
				C.memcpy(crDstPtr, crSrcPtr, C.size_t(cWidth))
			}
		} else {
			// Use unsafe slices directly
			ySize := width * height
			cSize := cWidth * cHeight
			ycc.Y = unsafe.Slice((*byte)(yData), ySize)
			ycc.Cb = unsafe.Slice((*byte)(cbData), cSize)
			ycc.Cr = unsafe.Slice((*byte)(crData), cSize)
		}

		return ycc, nil
	} else {
		// 10-bit YCbCr - for now, convert to 8-bit by shifting
		ycc := &image.YCbCr{
			YStride:        width,
			CStride:        cWidth,
			SubsampleRatio: subsample,
			Rect:           image.Rectangle{Min: image.Point{0, 0}, Max: image.Point{width, height}},
		}

		ycc.Y = make([]uint8, width*height)
		ycc.Cb = make([]uint8, cWidth*cHeight)
		ycc.Cr = make([]uint8, cWidth*cHeight)

		// Convert Y plane
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				srcPtr := (*uint16)(unsafe.Pointer(uintptr(yData) + uintptr(y*yStride+x*2)))
				ycc.Y[y*width+x] = uint8(*srcPtr >> 2) // 10-bit to 8-bit
			}
		}

		// Convert Cb and Cr planes
		for y := 0; y < cHeight; y++ {
			for x := 0; x < cWidth; x++ {
				cbPtr := (*uint16)(unsafe.Pointer(uintptr(cbData) + uintptr(y*cStride+x*2)))
				crPtr := (*uint16)(unsafe.Pointer(uintptr(crData) + uintptr(y*cStride+x*2)))
				ycc.Cb[y*cWidth+x] = uint8(*cbPtr >> 2) // 10-bit to 8-bit
				ycc.Cr[y*cWidth+x] = uint8(*crPtr >> 2) // 10-bit to 8-bit
			}
		}

		return ycc, nil
	}
}

// Convenience function for decoding AVIF data
func Decode(data []byte) (image.Image, error) {
	decoder, err := NewDecoder()
	if err != nil {
		return nil, err
	}
	defer decoder.Free()

	return decoder.DecodeImage(data)
}
