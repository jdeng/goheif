package goheif

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"

	"github.com/jdeng/goheif/libde265"
)

// DecodeHEVCAnnexB decodes one HEVC still picture from an Annex-B byte stream.
func DecodeHEVCAnnexB(data []byte) (image.Image, error) {
	sample, err := annexBToNALSample(data)
	if err != nil {
		return nil, err
	}
	dec, err := libde265.NewDecoder(libde265.WithSafeEncoding(SafeEncoding))
	if err != nil {
		return nil, err
	}
	defer dec.Free()
	return dec.DecodeImage(sample)
}

func annexBToNALSample(data []byte) ([]byte, error) {
	starts := annexBStartCodes(data)
	if len(starts) == 0 {
		return nil, errors.New("no annex-b start codes")
	}

	out := bytes.NewBuffer(make([]byte, 0, len(data)))
	for index, start := range starts {
		nalStart := start + annexBStartCodeLen(data[start:])
		nalEnd := len(data)
		if index+1 < len(starts) {
			nalEnd = starts[index+1]
		}
		for nalEnd > nalStart && data[nalEnd-1] == 0 {
			nalEnd--
		}
		if nalEnd <= nalStart {
			continue
		}
		size := nalEnd - nalStart
		if size > int(^uint32(0)) {
			return nil, fmt.Errorf("nal too large: %d bytes", size)
		}
		var length [4]byte
		binary.BigEndian.PutUint32(length[:], uint32(size))
		out.Write(length[:])
		out.Write(data[nalStart:nalEnd])
	}
	if out.Len() == 0 {
		return nil, errors.New("empty nal sample")
	}
	return out.Bytes(), nil
}

func annexBStartCodes(data []byte) []int {
	starts := make([]int, 0, 16)
	for i := 0; i < len(data)-2; i++ {
		if data[i] != 0 || data[i+1] != 0 {
			continue
		}
		switch {
		case data[i+2] == 1:
			starts = append(starts, i)
			i += 2
		case i+3 < len(data) && data[i+2] == 0 && data[i+3] == 1:
			starts = append(starts, i)
			i += 3
		}
	}
	return starts
}

func annexBStartCodeLen(data []byte) int {
	if len(data) >= 4 && data[0] == 0 && data[1] == 0 && data[2] == 0 && data[3] == 1 {
		return 4
	}
	return 3
}
