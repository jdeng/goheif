package goheif

import (
	"fmt"
	"github.com/jdeng/goheif/heif"
	"github.com/jdeng/goheif/libde265"
	"image"
	"image/color"
	"io"
)

type gridBox struct {
	columns, rows int
	width, height int
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

func decodeHevcItem(hf *heif.File, item *heif.Item) (*image.YCbCr, error) {
	if item.Info.ItemType != "hvc1" {
		return nil, fmt.Errorf("Unsupported item type: %s", item.Info.ItemType)
	}

	hvcc, ok := item.HevcConfig()
	if !ok {
		return nil, fmt.Errorf("No hvcC")
	}

	hdr := hvcc.AsHeader()
	data, err := hf.GetItemData(item)
	if err != nil {
		return nil, err
	}

	dec := libde265.NewDecoder()
	dec.Push(hdr)
	tile, err := dec.DecodeImage(data)
	dec.Free()
	if err != nil {
		return nil, err
	}

	ycc, ok := tile.(*image.YCbCr)
	if !ok {
		return nil, fmt.Errorf("Tile is not YCbCr")
	}

	return ycc, nil
}

func DecodeImageExif(ra io.ReaderAt) ([]byte, error) {
	hf := heif.Open(ra)
	return hf.EXIF()
}

func DecodeImage(ra io.ReaderAt) (image.Image, error) {
	hf := heif.Open(ra)

	it, err := hf.PrimaryItem()
	if err != nil {
		return nil, err
	}

	width, height, ok := it.SpatialExtents()
	if !ok {
		return nil, fmt.Errorf("No dimension")
	}

	if it.Info == nil {
		return nil, fmt.Errorf("No item info")
	}

	if it.Info.ItemType == "hvc1" {
		return decodeHevcItem(hf, it)
	}

	if it.Info.ItemType != "grid" {
		return nil, fmt.Errorf("No grid")
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
		return nil, fmt.Errorf("No dimg")
	}

	if len(dimg.ToItemIDs) != grid.columns*grid.rows {
		return nil, fmt.Errorf("Tiles number not matched")
	}

	var out *image.YCbCr
	var tileWidth, tileHeight int
	for i, y := 0, 0; y < grid.rows; y += 1 {
		for x := 0; x < grid.columns; x += 1 {
			id := dimg.ToItemIDs[i]
			item, err := hf.ItemByID(id)
			if err != nil {
				return nil, err
			}

			ycc, err := decodeHevcItem(hf, item)
			if err != nil {
				return nil, err
			}

			rect := ycc.Bounds()
			if tileWidth == 0 {
				tileWidth, tileHeight = rect.Dx(), rect.Dy()
				width, height := tileWidth*grid.columns, tileHeight*grid.rows
				out = image.NewYCbCr(image.Rectangle{image.Pt(0, 0), image.Pt(width, height)}, ycc.SubsampleRatio)
			}

			if tileWidth != rect.Dx() || tileHeight != rect.Dy() {
				return nil, fmt.Errorf("Inconsistent tile dimensions")
			}

			// copy y stride data
			for i := 0; i < rect.Dy(); i += 1 {
				copy(out.Y[(y*tileHeight+i)*out.YStride+x*ycc.YStride:], ycc.Y[i*ycc.YStride:(i+1)*ycc.YStride])
			}

			// height of c strides
			cHeight := len(ycc.Cb) / ycc.CStride

			// copy c stride data
			for i := 0; i < cHeight; i += 1 {
				copy(out.Cb[(y*cHeight+i)*out.CStride+x*ycc.CStride:], ycc.Cb[i*ycc.CStride:(i+1)*ycc.CStride])
				copy(out.Cr[(y*cHeight+i)*out.CStride+x*ycc.CStride:], ycc.Cr[i*ycc.CStride:(i+1)*ycc.CStride])
			}

			i += 1
		}
	}

	//crop to actual size when applicable
	out.Rect = image.Rectangle{image.Pt(0, 0), image.Pt(width, height)}
	return out, nil
}

func DecodeConfig(ra io.ReaderAt) (image.Config, error) {
	var config image.Config

	hf := heif.Open(ra)

	it, err := hf.PrimaryItem()
	if err != nil {
		return config, err
	}

	width, height, ok := it.SpatialExtents()
	if !ok {
		return config, fmt.Errorf("No dimension")
	}

	config = image.Config{
		ColorModel: color.YCbCrModel,
		Width:      width,
		Height:     height,
	}
	return config, nil
}

func init() {
	libde265.Init()
	//image.RegisterFormat("heif", "????ftypheic", decodeImage, decodeConfig)
}
