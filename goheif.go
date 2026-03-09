package goheif

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"

	"github.com/jdeng/goheif/dav1d"
	"github.com/jdeng/goheif/heif"
	"github.com/jdeng/goheif/libde265"
)

// SafeEncoding uses more memory but seems to make
// the library safer to use in containers.
var SafeEncoding bool = true

type gridBox struct {
	columns, rows int
	width, height int
}

func chromaDimensions(width, height int, subsample image.YCbCrSubsampleRatio) (int, int) {
	switch subsample {
	case image.YCbCrSubsampleRatio422:
		return (width + 1) / 2, height
	case image.YCbCrSubsampleRatio420:
		return (width + 1) / 2, (height + 1) / 2
	case image.YCbCrSubsampleRatio440:
		return width, (height + 1) / 2
	case image.YCbCrSubsampleRatio411:
		return (width + 3) / 4, height
	case image.YCbCrSubsampleRatio410:
		return (width + 3) / 4, (height + 1) / 2
	default:
		return width, height
	}
}

func chromaYStep(subsample image.YCbCrSubsampleRatio) int {
	switch subsample {
	case image.YCbCrSubsampleRatio420, image.YCbCrSubsampleRatio440, image.YCbCrSubsampleRatio410:
		return 2
	default:
		return 1
	}
}

func copyPlane(dst []byte, dstStart int, src []byte, srcStart int, width int, plane string) error {
	if dstStart < 0 || srcStart < 0 {
		return fmt.Errorf("%s plane offset out of bounds", plane)
	}
	if dstStart+width > len(dst) {
		return fmt.Errorf("%s destination row out of bounds", plane)
	}
	if srcStart+width > len(src) {
		return fmt.Errorf("%s source row out of bounds", plane)
	}

	copy(dst[dstStart:dstStart+width], src[srcStart:srcStart+width])
	return nil
}

func stitchYCbCrTile(dst *image.YCbCr, src *image.YCbCr, tileX, tileY int) error {
	if dst.SubsampleRatio != src.SubsampleRatio {
		return errors.New("inconsistent tile subsampling")
	}

	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	dstX := tileX * width
	dstY := tileY * height

	for row := 0; row < height; row++ {
		srcStart := src.YOffset(bounds.Min.X, bounds.Min.Y+row)
		dstStart := dst.YOffset(dstX, dstY+row)
		if err := copyPlane(dst.Y, dstStart, src.Y, srcStart, width, "luma"); err != nil {
			return err
		}
	}

	chromaWidth, chromaHeight := chromaDimensions(width, height, src.SubsampleRatio)
	dstChromaX := tileX * chromaWidth
	dstChromaY := tileY * chromaHeight
	srcChromaYStep := chromaYStep(src.SubsampleRatio)

	for row := 0; row < chromaHeight; row++ {
		srcStart := src.COffset(bounds.Min.X, bounds.Min.Y+row*srcChromaYStep)
		dstStart := (dstChromaY+row)*dst.CStride + dstChromaX

		if err := copyPlane(dst.Cb, dstStart, src.Cb, srcStart, chromaWidth, "cb"); err != nil {
			return err
		}
		if err := copyPlane(dst.Cr, dstStart, src.Cr, srcStart, chromaWidth, "cr"); err != nil {
			return err
		}
	}

	return nil
}

func newGridBox(data []byte) (*gridBox, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("invalid data")
	}
	// version := data[0]
	flags := data[1]
	rows := int(data[2]) + 1
	columns := int(data[3]) + 1

	var width, height int
	if (flags & 1) != 0 {
		if len(data) < 12 {
			return nil, fmt.Errorf("invalid data")
		}

		width = int(data[4])<<24 | int(data[5])<<16 | int(data[6])<<8 | int(data[7])
		height = int(data[8])<<24 | int(data[9])<<16 | int(data[10])<<8 | int(data[11])
	} else {
		width = int(data[4])<<8 | int(data[5])
		height = int(data[6])<<8 | int(data[7])
	}

	return &gridBox{columns: columns, rows: rows, width: width, height: height}, nil
}

func decodeHevcItem(dec *libde265.Decoder, hf *heif.File, item *heif.Item) (*image.YCbCr, error) {
	if item.Info.ItemType != "hvc1" {
		return nil, fmt.Errorf("unsupported item type: %s", item.Info.ItemType)
	}

	hvcc, ok := item.HevcConfig()
	if !ok {
		return nil, errors.New("no hvcC")
	}

	hdr := hvcc.AsHeader()
	data, err := hf.GetItemData(item)
	if err != nil {
		return nil, err
	}

	dec.Reset()
	dec.Push(hdr)
	tile, err := dec.DecodeImage(data)
	if err != nil {
		return nil, err
	}

	ycc, ok := tile.(*image.YCbCr)
	if !ok {
		return nil, errors.New("tile is not YCbCr")
	}

	return ycc, nil
}

func decodeAv1Item(dec *dav1d.Decoder, hf *heif.File, item *heif.Item) (image.Image, error) {
	if item.Info.ItemType != "av01" {
		return nil, fmt.Errorf("unsupported item type: %s", item.Info.ItemType)
	}

	data, err := hf.GetItemData(item)
	if err != nil {
		return nil, err
	}

	dec.Reset()
	img, err := dec.DecodeImage(data)
	if err != nil {
		return nil, err
	}

	return img, nil
}

func ExtractExif(ra io.ReaderAt) ([]byte, error) {
	hf := heif.Open(ra)
	return hf.EXIF()
}

func Decode(r io.Reader) (image.Image, error) {
	ra, err := asReaderAt(r)
	if err != nil {
		return nil, err
	}

	hf := heif.Open(ra)

	it, err := hf.PrimaryItem()
	if err != nil {
		return nil, err
	}

	width, height, ok := it.SpatialExtents()
	if !ok {
		return nil, errors.New("no dimension")
	}

	if it.Info == nil {
		return nil, errors.New("no item info")
	}

	// Handle AV1 items
	if it.Info.ItemType == "av01" {
		av1Dec, err := dav1d.NewDecoder(dav1d.WithSafeEncoding(SafeEncoding))
		if err != nil {
			return nil, err
		}
		defer av1Dec.Free()
		return decodeAv1Item(av1Dec, hf, it)
	}

	// Handle HEVC items
	if it.Info.ItemType == "hvc1" {
		hevcDec, err := libde265.NewDecoder(libde265.WithSafeEncoding(SafeEncoding))
		if err != nil {
			return nil, err
		}
		defer hevcDec.Free()
		return decodeHevcItem(hevcDec, hf, it)
	}

	if it.Info.ItemType != "grid" {
		return nil, errors.New("no grid")
	}

	data, err := hf.GetItemData(it)
	if err != nil {
		return nil, err
	}

	grid, err := newGridBox(data)
	if err != nil {
		return nil, err
	}

	dimg := it.Reference("dimg")
	if dimg == nil {
		return nil, errors.New("no dimg")
	}

	if len(dimg.ToItemIDs) != grid.columns*grid.rows {
		return nil, fmt.Errorf("tiles number not matched: %d != %d", len(dimg.ToItemIDs), grid.columns*grid.rows)
	}

	var out *image.YCbCr
	var tileWidth, tileHeight int
	var hevcDec *libde265.Decoder
	var av1Dec *dav1d.Decoder

	for i, y := 0, 0; y < grid.rows; y++ {
		for x := 0; x < grid.columns; x++ {
			id := dimg.ToItemIDs[i]
			item, err := hf.ItemByID(id)
			if err != nil {
				return nil, err
			}

			var img image.Image
			// Decode based on item type
			if item.Info.ItemType == "av01" {
				if av1Dec == nil {
					av1Dec, err = dav1d.NewDecoder(dav1d.WithSafeEncoding(SafeEncoding))
					if err != nil {
						return nil, err
					}
					defer av1Dec.Free()
				}
				img, err = decodeAv1Item(av1Dec, hf, item)
			} else if item.Info.ItemType == "hvc1" {
				if hevcDec == nil {
					hevcDec, err = libde265.NewDecoder(libde265.WithSafeEncoding(SafeEncoding))
					if err != nil {
						return nil, err
					}
					defer hevcDec.Free()
				}
				img, err = decodeHevcItem(hevcDec, hf, item)
			} else {
				return nil, fmt.Errorf("unsupported tile item type: %s", item.Info.ItemType)
			}

			if err != nil {
				return nil, err
			}

			ycc, ok := img.(*image.YCbCr)
			if !ok {
				return nil, errors.New("tile is not YCbCr")
			}

			rect := ycc.Bounds()
			if tileWidth == 0 {
				tileWidth, tileHeight = rect.Dx(), rect.Dy()
				xwidth, xheight := tileWidth*grid.columns, tileHeight*grid.rows
				out = image.NewYCbCr(image.Rectangle{image.Pt(0, 0), image.Pt(xwidth, xheight)}, ycc.SubsampleRatio)
			}

			if tileWidth != rect.Dx() || tileHeight != rect.Dy() {
				return nil, errors.New("inconsistent tile dimensions")
			}

			if err := stitchYCbCrTile(out, ycc, x, y); err != nil {
				return nil, err
			}

			i++
		}
	}

	//crop to actual size when applicable
	out.Rect = image.Rectangle{image.Pt(0, 0), image.Pt(width, height)}
	return out, nil
}

func DecodeConfig(r io.Reader) (image.Config, error) {
	var config image.Config

	ra, err := asReaderAt(r)
	if err != nil {
		return config, err
	}

	hf := heif.Open(ra)

	it, err := hf.PrimaryItem()
	if err != nil {
		return config, err
	}

	width, height, ok := it.SpatialExtents()
	if !ok {
		return config, errors.New("no dimension")
	}

	config = image.Config{
		ColorModel: color.YCbCrModel,
		Width:      width,
		Height:     height,
	}
	return config, nil
}

func asReaderAt(r io.Reader) (io.ReaderAt, error) {
	if ra, ok := r.(io.ReaderAt); ok {
		return ra, nil
	}

	b, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	return bytes.NewReader(b), nil
}

func init() {
	libde265.Init()
	// they check for "ftyp" at the 5th bytes, let's do the same...
	// https://github.com/strukturag/libheif/blob/master/libheif/heif.cc#L94
	image.RegisterFormat("heic", "????ftyp", Decode, DecodeConfig)
}
