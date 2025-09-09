/*
Copyright 2018 The go4 Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package bmff reads ISO BMFF boxes, as used by HEIF, etc.
//
// This is not so much as a generic BMFF reader as it is a BMFF reader
// as needed by HEIF, though that may change in time. For now, only
// boxes necessary for the go4.org/media/heif package have explicit
// parsers.
//
// This package makes no API compatibility promises; it exists
// primarily for use by the go4.org/media/heif package.
package bmff

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"strings"
)

func NewReader(r io.Reader) *Reader {
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(r)
	}
	return &Reader{br: bufReader{Reader: br}}
}

type Reader struct {
	br          bufReader
	lastBox     Box  // or nil
	noMoreBoxes bool // a box with size 0 (the final box) was seen
}

type BoxType [4]byte

// Common box types.
var (
	TypeFtyp = BoxType{'f', 't', 'y', 'p'}
	TypeMeta = BoxType{'m', 'e', 't', 'a'}
	TypeMdat = BoxType{'m', 'd', 'a', 't'}
)

func (t BoxType) String() string { return string(t[:]) }

func (t BoxType) EqualString(s string) bool {
	// Could be cleaner, but see ohttps://github.com/golang/go/issues/24765
	return len(s) == 4 && s[0] == t[0] && s[1] == t[1] && s[2] == t[2] && s[3] == t[3]
}

type parseFunc func(b box, br *bufio.Reader) (Box, error)

// Box represents a BMFF box.
type Box interface {
	Size() int64 // 0 means unknown (will read to end of file)
	Type() BoxType

	// Parses parses the box, populating the fields
	// in the returned concrete type.
	//
	// If Parse has already been called, Parse returns nil.
	// If the box type is unknown, the returned error is ErrUnknownBox
	// and it's guaranteed that no bytes have been read from the box.
	Parse() (Box, error)

	// Body returns the inner bytes of the box, ignoring the header.
	// The body may start with the 4 byte header of a "Full Box" if the
	// box's type derives from a full box. Most users will use Parse
	// instead.
	// Body will return a new reader at the beginning of the box if the
	// outer box has already been parsed.
	Body() io.Reader
}

// ErrUnknownBox is returned by Box.Parse for unrecognized box types.
var ErrUnknownBox = errors.New("heif: unknown box")

type parserFunc func(b *box, br *bufReader) (Box, error)

func boxType(s string) BoxType {
	if len(s) != 4 {
		panic("bogus boxType length")
	}
	return BoxType{s[0], s[1], s[2], s[3]}
}

var parsers = map[BoxType]parserFunc{
	boxType("dinf"): parseDataInformationBox,
	boxType("dref"): parseDataReferenceBox,
	boxType("ftyp"): parseFileTypeBox,
	boxType("hdlr"): parseHandlerBox,
	boxType("iinf"): parseItemInfoBox,
	boxType("infe"): parseItemInfoEntry,
	boxType("iloc"): parseItemLocationBox,
	boxType("ipco"): parseItemPropertyContainerBox,
	boxType("ipma"): parseItemPropertyAssociation,
	boxType("iprp"): parseItemPropertiesBox,
	boxType("irot"): parseImageRotation,
	boxType("imir"): parseImageMirror,
	boxType("ispe"): parseImageSpatialExtentsProperty,
	boxType("meta"): parseMetaBox,
	boxType("pitm"): parsePrimaryItemBox,
	boxType("idat"): parseItemDataBox,
	boxType("iref"): parseItemReferenceBox,
	boxType("hvcC"): parseItemHevcConfigBox,
	boxType("av1C"): parseItemAv1ConfigBox,
}

type box struct {
	size    int64 // 0 means unknown, will read to end of file (box container)
	boxType BoxType
	body    io.Reader
	parsed  Box    // if non-nil, the Parsed result
	slurp   []byte // if non-nil, the contents slurped to memory
}

func (b *box) Size() int64   { return b.size }
func (b *box) Type() BoxType { return b.boxType }

func (b *box) Body() io.Reader {
	if b.slurp != nil {
		return bytes.NewReader(b.slurp)
	}
	return b.body
}

func (b *box) Parse() (Box, error) {
	if b.parsed != nil {
		return b.parsed, nil
	}
	parser, ok := parsers[b.Type()]
	if !ok {
		return nil, ErrUnknownBox
	}
	v, err := parser(b, &bufReader{Reader: bufio.NewReader(b.Body())})
	if err != nil {
		return nil, err
	}
	b.parsed = v
	return v, nil
}

type FullBox struct {
	*box
	Version uint8
	Flags   uint32 // 24 bits
}

// ReadBox reads the next box.
//
// If the previously read box was not read to completion, ReadBox consumes
// the rest of its data.
//
// At the end, the error is io.EOF.
func (r *Reader) ReadBox() (Box, error) {
	if r.noMoreBoxes {
		return nil, io.EOF
	}
	if r.lastBox != nil {
		if _, err := io.Copy(ioutil.Discard, r.lastBox.Body()); err != nil {
			return nil, err
		}
	}
	var buf [8]byte

	_, err := io.ReadFull(r.br, buf[:4])
	if err != nil {
		return nil, err
	}
	box := &box{
		size: int64(binary.BigEndian.Uint32(buf[:4])),
	}

	_, err = io.ReadFull(r.br, box.boxType[:]) // 4 more bytes
	if err != nil {
		return nil, err
	}

	// Special cases for size:
	var remain int64
	switch box.size {
	case 1:
		// 1 means it's actually a 64-bit size, after the type.
		_, err = io.ReadFull(r.br, buf[:8])
		if err != nil {
			return nil, err
		}
		box.size = int64(binary.BigEndian.Uint64(buf[:8]))
		if box.size < 0 {
			// Go uses int64 for sizes typically, but BMFF uses uint64.
			// We assume for now that nobody actually uses boxes larger
			// than int64.
			return nil, fmt.Errorf("unexpectedly large box %q", box.boxType)
		}
		remain = box.size - 2*4 - 8
	case 0:
		// 0 means unknown & to read to end of file. No more boxes.
		r.noMoreBoxes = true
	default:
		remain = box.size - 2*4
	}
	if remain < 0 {
		return nil, fmt.Errorf("Box header for %q has size %d, suggesting %d (negative) bytes remain", box.boxType, box.size, remain)
	}
	if box.size > 0 {
		box.body = io.LimitReader(r.br, remain)
	} else {
		box.body = r.br
	}
	r.lastBox = box
	return box, nil
}

// ReadAndParseBox wraps the ReadBox method, ensuring that the read box is of type typ
// and parses successfully. It returns the parsed box.
func (r *Reader) ReadAndParseBox(typ BoxType) (Box, error) {
	box, err := r.ReadBox()
	if err != nil {
		return nil, fmt.Errorf("error reading %q box: %v", typ, err)
	}
	if box.Type() != typ {
		return nil, fmt.Errorf("error reading %q box: got box type %q instead", typ, box.Type())
	}
	pbox, err := box.Parse()
	if err != nil {
		return nil, fmt.Errorf("error parsing read %q box: %v", typ, err)
	}
	return pbox, nil
}

func readFullBox(outer *box, br *bufReader) (fb FullBox, err error) {
	fb.box = outer
	// Parse FullBox header.
	buf, err := br.Peek(4)
	if err != nil {
		return FullBox{}, fmt.Errorf("failed to read 4 bytes of FullBox: %v", err)
	}
	fb.Version = buf[0]
	buf[0] = 0
	fb.Flags = binary.BigEndian.Uint32(buf[:4])
	br.Discard(4)
	return fb, nil
}

type FileTypeBox struct {
	*box
	MajorBrand   string   // 4 bytes
	MinorVersion string   // 4 bytes
	Compatible   []string // all 4 bytes
}

func parseFileTypeBox(outer *box, br *bufReader) (Box, error) {
	buf, err := br.Peek(8)
	if err != nil {
		return nil, err
	}
	ft := &FileTypeBox{
		box:          outer,
		MajorBrand:   string(buf[:4]),
		MinorVersion: string(buf[4:8]),
	}
	br.Discard(8)
	for {
		buf, err := br.Peek(4)
		if err == io.EOF {
			return ft, nil
		}
		if err != nil {
			return nil, err
		}
		ft.Compatible = append(ft.Compatible, string(buf[:4]))
		br.Discard(4)
	}
}

type MetaBox struct {
	FullBox
	Children []Box
}

func parseMetaBox(outer *box, br *bufReader) (Box, error) {
	fb, err := readFullBox(outer, br)
	if err != nil {
		return nil, err
	}
	mb := &MetaBox{FullBox: fb}
	return mb, br.parseAppendBoxes(&mb.Children)
}

func (br *bufReader) parseAppendBoxes(dst *[]Box) error {
	if br.err != nil {
		return br.err
	}
	boxr := NewReader(br.Reader)
	for {
		inner, err := boxr.ReadBox()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			br.err = err
			return err
		}
		slurp, err := ioutil.ReadAll(inner.Body())
		if err != nil {
			br.err = err
			return err
		}
		inner.(*box).slurp = slurp
		*dst = append(*dst, inner)
	}
}

// ItemInfoEntry represents an "infe" box.
//
// TODO: currently only parses Version 2 boxes.
type ItemInfoEntry struct {
	FullBox

	ItemID          uint16
	ProtectionIndex uint16
	ItemType        string // always 4 bytes

	Name string

	// If Type == "mime":
	ContentType     string
	ContentEncoding string

	// If Type == "uri ":
	ItemURIType string
}

func parseItemInfoEntry(outer *box, br *bufReader) (Box, error) {
	fb, err := readFullBox(outer, br)
	if err != nil {
		return nil, err
	}
	ie := &ItemInfoEntry{FullBox: fb}
	if fb.Version != 2 {
		return nil, fmt.Errorf("TODO: found version %d infe box. Only 2 is supported now.", fb.Version)
	}

	ie.ItemID, _ = br.readUint16()
	ie.ProtectionIndex, _ = br.readUint16()
	if !br.ok() {
		return nil, br.err
	}
	buf, err := br.Peek(4)
	if err != nil {
		return nil, err
	}
	ie.ItemType = string(buf[:4])
	br.Discard(4) // CRITICAL: Must discard the 4 bytes we peeked
	ie.Name, _ = br.readString()

	switch ie.ItemType {
	case "mime":
		ie.ContentType, _ = br.readString()
		if br.anyRemain() {
			ie.ContentEncoding, _ = br.readString()
		}
	case "uri ":
		ie.ItemURIType, _ = br.readString()
	default:
		// Handle av01 and other unknown item types - just continue
	}
	if !br.ok() {
		return nil, br.err
	}
	return ie, nil
}

// ItemInfoBox represents an "iinf" box.
type ItemInfoBox struct {
	FullBox
	Count     uint32
	ItemInfos []*ItemInfoEntry
}

func parseItemInfoBox(outer *box, br *bufReader) (Box, error) {
	fb, err := readFullBox(outer, br)
	if err != nil {
		return nil, err
	}
	ib := &ItemInfoBox{FullBox: fb}

	if ib.Version >= 1 {
		ib.Count, _ = br.readUint32()
	} else {
		count, _ := br.readUint16()
		ib.Count = uint32(count)
	}

	var itemInfos []Box
	br.parseAppendBoxes(&itemInfos)
	if br.ok() {
		for _, box := range itemInfos {
			pb, err := box.Parse()
			if err == ErrUnknownBox {
				// Skip unknown boxes gracefully in AVIF files
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("error parsing ItemInfoEntry in ItemInfoBox: %v", err)
			}
			if iie, ok := pb.(*ItemInfoEntry); ok {
				ib.ItemInfos = append(ib.ItemInfos, iie)
			}
		}
	}
	if !br.ok() {
		return FullBox{}, br.err
	}
	return ib, nil
}

// ItemReferenceBox represents an "iref" box.
type ItemReferenceBox struct {
	FullBox
	ItemRefs []*ItemReferenceEntry
}

type ItemReferenceEntry struct {
	*box
	FromItemID uint32
	Count      uint16
	ToItemIDs  []uint32
}

func parseItemReferenceBox(outer *box, br *bufReader) (Box, error) {
	fb, err := readFullBox(outer, br)
	if err != nil {
		return nil, err
	}
	ib := &ItemReferenceBox{FullBox: fb}

	var itemRefs []Box
	br.parseAppendBoxes(&itemRefs)

	if br.ok() {
		for _, b := range itemRefs {
			pb, err := parseItemReferenceEntry(b.(*box), &bufReader{Reader: bufio.NewReader(b.Body())}, ib.Version)
			if err != nil {
				return nil, fmt.Errorf("error parsing ItemReferenceEntry in ItemReferenceBox: %v", err)
			}
			if iie, ok := pb.(*ItemReferenceEntry); ok {
				ib.ItemRefs = append(ib.ItemRefs, iie)
			}
		}
	}
	if !br.ok() {
		return FullBox{}, br.err
	}
	return ib, nil
}

func parseItemReferenceEntry(outer *box, br *bufReader, version uint8) (Box, error) {
	ie := &ItemReferenceEntry{box: outer}

	if version == 0 {
		itemID, _ := br.readUint16()
		ie.FromItemID = uint32(itemID)
		ie.Count, _ = br.readUint16()
		for i := 0; i < int(ie.Count); i += 1 {
			itemID, _ := br.readUint16()
			ie.ToItemIDs = append(ie.ToItemIDs, uint32(itemID))
		}
	} else {
		ie.FromItemID, _ = br.readUint32()
		ie.Count, _ = br.readUint16()
		for i := 0; i < int(ie.Count); i += 1 {
			itemID, _ := br.readUint32()
			ie.ToItemIDs = append(ie.ToItemIDs, itemID)
		}
	}

	return ie, nil
}

// bufReader adds some HEIF/BMFF-specific methods around a *bufio.Reader.
type bufReader struct {
	*bufio.Reader
	err error // sticky error
}

// ok reports whether all previous reads have been error-free.
func (br *bufReader) ok() bool { return br.err == nil }

func (br *bufReader) anyRemain() bool {
	if br.err != nil {
		return false
	}
	_, err := br.Peek(1)
	return err == nil
}

func (br *bufReader) readUintN(bits uint8) (uint64, error) {
	if br.err != nil {
		return 0, br.err
	}
	if bits == 0 {
		return 0, nil
	}
	nbyte := bits / 8
	buf, err := br.Peek(int(nbyte))
	if err != nil {
		br.err = err
		return 0, err
	}
	defer br.Discard(int(nbyte))
	switch bits {
	case 8:
		return uint64(buf[0]), nil
	case 16:
		return uint64(binary.BigEndian.Uint16(buf[:2])), nil
	case 32:
		return uint64(binary.BigEndian.Uint32(buf[:4])), nil
	case 64:
		return binary.BigEndian.Uint64(buf[:8]), nil
	default:
		br.err = fmt.Errorf("invalid uintn read size")
		return 0, br.err
	}
}

func (br *bufReader) readUint8() (uint8, error) {
	if br.err != nil {
		return 0, br.err
	}
	v, err := br.ReadByte()
	if err != nil {
		br.err = err
		return 0, err
	}
	return v, nil
}

func (br *bufReader) readUint16() (uint16, error) {
	if br.err != nil {
		return 0, br.err
	}
	buf, err := br.Peek(2)
	if err != nil {
		br.err = err
		return 0, err
	}
	v := binary.BigEndian.Uint16(buf[:2])
	br.Discard(2)
	return v, nil
}

func (br *bufReader) readUint32() (uint32, error) {
	if br.err != nil {
		return 0, br.err
	}
	buf, err := br.Peek(4)
	if err != nil {
		br.err = err
		return 0, err
	}
	v := binary.BigEndian.Uint32(buf[:4])
	br.Discard(4)
	return v, nil
}

func (br *bufReader) readString() (string, error) {
	if br.err != nil {
		return "", br.err
	}
	s0, err := br.ReadString(0)
	if err != nil {
		br.err = err
		return "", err
	}
	s := strings.TrimSuffix(s0, "\x00")
	if len(s) == len(s0) {
		err = fmt.Errorf("unexpected non-null terminated string")
		br.err = err
		return "", err
	}
	return s, nil
}

// HEIF: ipco
type ItemPropertyContainerBox struct {
	*box
	Properties []Box // of ItemProperty or ItemFullProperty
}

func parseItemPropertyContainerBox(outer *box, br *bufReader) (Box, error) {
	ipc := &ItemPropertyContainerBox{box: outer}
	return ipc, br.parseAppendBoxes(&ipc.Properties)
}

// HEIF: iprp
type ItemPropertiesBox struct {
	*box
	PropertyContainer *ItemPropertyContainerBox
	Associations      []*ItemPropertyAssociation // at least 1
}

func parseItemPropertiesBox(outer *box, br *bufReader) (Box, error) {
	ip := &ItemPropertiesBox{
		box: outer,
	}

	var boxes []Box
	err := br.parseAppendBoxes(&boxes)
	if err != nil {
		return nil, err
	}
	if len(boxes) < 2 {
		return nil, fmt.Errorf("expect at least 2 boxes in children; got 0")
	}

	cb, err := boxes[0].Parse()
	if err != nil {
		return nil, fmt.Errorf("failed to parse first box, %q: %v", boxes[0].Type(), err)
	}

	var ok bool
	ip.PropertyContainer, ok = cb.(*ItemPropertyContainerBox)
	if !ok {
		return nil, fmt.Errorf("unexpected type %T for ItemPropertieBox.PropertyContainer", cb)
	}

	// Association boxes
	ip.Associations = make([]*ItemPropertyAssociation, 0, len(boxes)-1)
	for _, box := range boxes[1:] {
		boxp, err := box.Parse()
		if err != nil {
			return nil, fmt.Errorf("failed to parse association box: %v", err)
		}
		ipa, ok := boxp.(*ItemPropertyAssociation)
		if !ok {
			return nil, fmt.Errorf("unexpected box %q instead of ItemPropertyAssociation", boxp.Type())
		}
		ip.Associations = append(ip.Associations, ipa)
	}
	return ip, nil
}

type ItemPropertyAssociation struct {
	FullBox
	EntryCount uint32
	Entries    []ItemPropertyAssociationItem
}

// not a box
type ItemProperty struct {
	Essential bool
	Index     uint16
}

// not a box
type ItemPropertyAssociationItem struct {
	ItemID            uint32
	AssociationsCount int            // as declared
	Associations      []ItemProperty // as parsed
}

func parseItemPropertyAssociation(outer *box, br *bufReader) (Box, error) {
	fb, err := readFullBox(outer, br)
	if err != nil {
		return nil, err
	}
	ipa := &ItemPropertyAssociation{FullBox: fb}
	count, _ := br.readUint32()
	ipa.EntryCount = count

	for i := uint64(0); i < uint64(count) && br.ok(); i++ {
		var itemID uint32
		if fb.Version < 1 {
			itemID16, _ := br.readUint16()
			itemID = uint32(itemID16)
		} else {
			itemID, _ = br.readUint32()
		}
		assocCount, _ := br.readUint8()
		ipai := ItemPropertyAssociationItem{
			ItemID:            itemID,
			AssociationsCount: int(assocCount),
		}
		for j := 0; j < int(assocCount) && br.ok(); j++ {
			first, _ := br.readUint8()
			essential := first&(1<<7) != 0
			first &^= byte(1 << 7)

			var index uint16
			if fb.Flags&1 != 0 {
				second, _ := br.readUint8()
				index = uint16(first)<<8 | uint16(second)
			} else {
				index = uint16(first)
			}
			ipai.Associations = append(ipai.Associations, ItemProperty{
				Essential: essential,
				Index:     index,
			})
		}
		ipa.Entries = append(ipa.Entries, ipai)
	}
	if !br.ok() {
		return nil, br.err
	}
	return ipa, nil
}

type ImageSpatialExtentsProperty struct {
	FullBox
	ImageWidth  uint32
	ImageHeight uint32
}

func parseImageSpatialExtentsProperty(outer *box, br *bufReader) (Box, error) {
	fb, err := readFullBox(outer, br)
	if err != nil {
		return nil, err
	}
	w, err := br.readUint32()
	if err != nil {
		return nil, err
	}
	h, err := br.readUint32()
	if err != nil {
		return nil, err
	}
	return &ImageSpatialExtentsProperty{
		FullBox:     fb,
		ImageWidth:  w,
		ImageHeight: h,
	}, nil
}

type OffsetLength struct {
	Offset, Length uint64
}

// not a box
type ItemLocationBoxEntry struct {
	ItemID             uint16
	ConstructionMethod uint8 // actually uint4
	DataReferenceIndex uint16
	BaseOffset         uint64 // uint32 or uint64, depending on encoding
	ExtentCount        uint16
	Extents            []OffsetLength
}

// box "iloc"
type ItemLocationBox struct {
	FullBox

	offsetSize, lengthSize, baseOffsetSize, indexSize uint8 // actually uint4

	ItemCount uint16
	Items     []ItemLocationBoxEntry
}

func parseItemLocationBox(outer *box, br *bufReader) (Box, error) {
	fb, err := readFullBox(outer, br)
	if err != nil {
		return nil, err
	}
	ilb := &ItemLocationBox{
		FullBox: fb,
	}
	buf, err := br.Peek(4)
	if err != nil {
		return nil, err
	}
	ilb.offsetSize = buf[0] >> 4
	ilb.lengthSize = buf[0] & 15
	ilb.baseOffsetSize = buf[1] >> 4
	if fb.Version > 0 { // version 1
		ilb.indexSize = buf[1] & 15
	}

	ilb.ItemCount = binary.BigEndian.Uint16(buf[2:4])
	br.Discard(4)

	for i := 0; br.ok() && i < int(ilb.ItemCount); i++ {
		var ent ItemLocationBoxEntry
		ent.ItemID, _ = br.readUint16()
		if fb.Version > 0 { // version 1
			cmeth, _ := br.readUint16()
			ent.ConstructionMethod = byte(cmeth & 15)
		}
		ent.DataReferenceIndex, _ = br.readUint16()
		if br.ok() && ilb.baseOffsetSize > 0 {
			if ilb.baseOffsetSize == 4 {
				bo, _ := br.readUint32()
				ent.BaseOffset = uint64(bo)
			} else if ilb.baseOffsetSize == 8 {
				bo, _ := br.readUint32()
				ent.BaseOffset = uint64(bo) << 32
				bo, _ = br.readUint32()
				ent.BaseOffset |= uint64(bo)
			}
			// br.Discard(int(ilb.baseOffsetSize) / 8)
		}
		ent.ExtentCount, _ = br.readUint16()
		for j := 0; br.ok() && j < int(ent.ExtentCount); j++ {
			var ol OffsetLength
			ol.Offset, _ = br.readUintN(ilb.offsetSize * 8)
			ol.Length, _ = br.readUintN(ilb.lengthSize * 8)
			if br.err != nil {
				return nil, br.err
			}
			ent.Extents = append(ent.Extents, ol)
		}
		ilb.Items = append(ilb.Items, ent)
	}
	if !br.ok() {
		return nil, br.err
	}
	return ilb, nil
}

// a "hdlr" box.
type HandlerBox struct {
	FullBox
	HandlerType string // always 4 bytes; usually "pict" for iOS Camera images
	Name        string
}

func parseHandlerBox(gen *box, br *bufReader) (Box, error) {
	fb, err := readFullBox(gen, br)
	if err != nil {
		return nil, err
	}
	hb := &HandlerBox{
		FullBox: fb,
	}
	buf, err := br.Peek(20)
	if err != nil {
		return nil, err
	}
	hb.HandlerType = string(buf[4:8])
	br.Discard(20)

	hb.Name, _ = br.readString()
	return hb, br.err
}

// a "dinf" box
type DataInformationBox struct {
	*box
	Children []Box
}

func parseDataInformationBox(gen *box, br *bufReader) (Box, error) {
	dib := &DataInformationBox{box: gen}
	return dib, br.parseAppendBoxes(&dib.Children)
}

// a "dref" box.
type DataReferenceBox struct {
	FullBox
	EntryCount uint32
	Children   []Box
}

func parseDataReferenceBox(gen *box, br *bufReader) (Box, error) {
	fb, err := readFullBox(gen, br)
	if err != nil {
		return nil, err
	}
	drb := &DataReferenceBox{FullBox: fb}
	drb.EntryCount, _ = br.readUint32()
	return drb, br.parseAppendBoxes(&drb.Children)
}

// "pitm" box
type PrimaryItemBox struct {
	FullBox
	ItemID uint16
}

func parsePrimaryItemBox(gen *box, br *bufReader) (Box, error) {
	fb, err := readFullBox(gen, br)
	if err != nil {
		return nil, err
	}
	pib := &PrimaryItemBox{FullBox: fb}
	pib.ItemID, _ = br.readUint16()
	if !br.ok() {
		return nil, br.err
	}
	return pib, nil
}

type ItemDataBox struct {
	FullBox
	Data []byte
}

func parseItemDataBox(gen *box, br *bufReader) (Box, error) {
	fb, err := readFullBox(gen, br)
	if err != nil {
		return nil, err
	}

	data, err := ioutil.ReadAll(fb.Body())
	if err != nil {
		return nil, err
	}

	if !br.ok() {
		return nil, br.err
	}

	idb := &ItemDataBox{FullBox: fb, Data: data}
	return idb, nil
}

// ImageRotation is a HEIF "irot" rotation property.
type ImageRotation struct {
	*box
	Angle uint8 // 1 means 90 degrees counter-clockwise, 2 means 180 counter-clockwise
}

func parseImageRotation(gen *box, br *bufReader) (Box, error) {
	v, err := br.readUint8()
	if err != nil {
		return nil, err
	}
	return &ImageRotation{box: gen, Angle: v & 3}, nil
}

// ImageMirror is a HEIF "imir" mirror property.
const (
	MirrorVertical   uint8 = 0
	MirrorHorizontal uint8 = 1
)

type ImageMirror struct {
	*box
	Mirror uint8
}

func parseImageMirror(gen *box, br *bufReader) (Box, error) {
	v, err := br.readUint8()
	if err != nil {
		return nil, err
	}
	return &ImageMirror{box: gen, Mirror: v & 1}, nil
}

// ItemHevcConfigBox is a HEIF "hvcC" property
type hevcConfig struct {
	version                          uint8
	generalProfileSpace              uint8
	generalTierFlag                  uint8
	generalProfileIdc                uint8
	generalProfileCompatibilityFlags uint32

	generalLevelIdc uint8

	minSpatialSegmentationIdc uint16
	parallelismType           uint8
	chromaFormat              uint8
	bitDepthLuma              uint8
	bitDepthChroma            uint8
	avgFrameRate              uint16

	constantFrameRate uint8
	numTemporalLayers uint8
	temporalIdNested  uint8
}

type hevcNalArray struct {
	completeness uint8
	unitType     uint8
	units        [][]byte
}

type ItemHevcConfigBox struct {
	*box
	config   hevcConfig
	nalArray []*hevcNalArray
}

type av1Config struct {
	marker                           uint8  // must be 1
	version                          uint8  // must be 1
	seqProfile                       uint8  // 3 bits
	seqLevelIdx0                     uint8  // 5 bits
	seqTier0                         uint8  // 1 bit
	highBitdepth                     uint8  // 1 bit
	twelveBit                        uint8  // 1 bit
	monochrome                       uint8  // 1 bit
	chromaSubsamplingX               uint8  // 1 bit
	chromaSubsamplingY               uint8  // 1 bit
	chromaSamplePosition             uint8  // 2 bits
	initialPresentationDelayPresent  uint8  // 1 bit
	initialPresentationDelayMinusOne uint8  // 4 bits (optional)
	configOBUs                       []byte // remaining bytes
}

type ItemAv1ConfigBox struct {
	*box
	config av1Config
}

func (ib *ItemHevcConfigBox) AsHeader() []byte {
	var out []byte
	for _, na := range ib.nalArray {
		for _, unit := range na.units {
			n := len(unit)
			out = append(out, byte((n>>24)&0xff))
			out = append(out, byte((n>>16)&0xff))
			out = append(out, byte((n>>8)&0xff))
			out = append(out, byte((n>>0)&0xff))
			out = append(out, unit...)
		}
	}

	return out
}

func parseItemHevcConfigBox(gen *box, br *bufReader) (Box, error) {
	ib := &ItemHevcConfigBox{box: gen}

	c := &ib.config
	c.version, _ = br.readUint8()

	ch, _ := br.readUint8()
	c.generalProfileSpace = uint8((ch >> 6) & 3)
	c.generalTierFlag = uint8((ch >> 5) & 1)
	c.generalProfileIdc = uint8(ch & 0x1F)

	c.generalProfileCompatibilityFlags, _ = br.readUint32()

	for i := 0; i < 6; i += 1 {
		//TODO: general_constraint_indicator_flags
		ch, _ = br.readUint8()
	}

	c.generalLevelIdc, _ = br.readUint8()
	c.minSpatialSegmentationIdc, _ = br.readUint16()
	c.parallelismType, _ = br.readUint8()
	c.chromaFormat, _ = br.readUint8()
	c.bitDepthLuma, _ = br.readUint8()
	c.bitDepthChroma, _ = br.readUint8()
	c.avgFrameRate, _ = br.readUint16()

	ch, _ = br.readUint8()
	c.constantFrameRate = uint8((ch >> 6) & 0x03)
	c.numTemporalLayers = uint8((ch >> 3) & 0x07)
	c.temporalIdNested = uint8((ch >> 2) & 1)

	numArrays, err := br.readUint8()
	if err != nil {
		return nil, err
	}

	for i := 0; i < int(numArrays); i += 1 {
		ch, _ := br.readUint8()

		na := &hevcNalArray{}
		na.completeness = uint8((ch >> 6) & 1)
		na.unitType = uint8(ch & 0x3F)

		numUnits, _ := br.readUint16()
		for j := 0; j < int(numUnits); j += 1 {
			size, _ := br.readUint16()
			if size == 0 { // ignore empty NAL units
				continue
			}

			unit := make([]byte, size)
			if _, err := io.ReadFull(br, unit); err != nil {
				return nil, err
			}
			na.units = append(na.units, unit)
		}

		ib.nalArray = append(ib.nalArray, na)
	}

	if !br.ok() {
		return nil, br.err
	}

	return ib, nil
}

func parseItemAv1ConfigBox(gen *box, br *bufReader) (Box, error) {
	ib := &ItemAv1ConfigBox{box: gen}

	c := &ib.config

	// Read first byte: marker (1 bit) + version (7 bits)
	firstByte, err := br.readUint8()
	if err != nil {
		return nil, err
	}
	c.marker = (firstByte >> 7) & 1
	c.version = firstByte & 0x7F

	// Read second byte: seq_profile (3 bits) + seq_level_idx_0 (5 bits)
	secondByte, err := br.readUint8()
	if err != nil {
		return nil, err
	}
	c.seqProfile = (secondByte >> 5) & 0x07
	c.seqLevelIdx0 = secondByte & 0x1F

	// Read third byte: seq_tier_0 (1 bit) + high_bitdepth (1 bit) + twelve_bit (1 bit) + monochrome (1 bit) +
	//                   chroma_subsampling_x (1 bit) + chroma_subsampling_y (1 bit) + chroma_sample_position (2 bits)
	thirdByte, err := br.readUint8()
	if err != nil {
		return nil, err
	}
	c.seqTier0 = (thirdByte >> 7) & 1
	c.highBitdepth = (thirdByte >> 6) & 1
	c.twelveBit = (thirdByte >> 5) & 1
	c.monochrome = (thirdByte >> 4) & 1
	c.chromaSubsamplingX = (thirdByte >> 3) & 1
	c.chromaSubsamplingY = (thirdByte >> 2) & 1
	c.chromaSamplePosition = thirdByte & 0x03

	// Read fourth byte: reserved (3 bits) + initial_presentation_delay_present (1 bit) +
	//                   initial_presentation_delay_minus_one (4 bits) if present
	fourthByte, err := br.readUint8()
	if err != nil {
		return nil, err
	}
	c.initialPresentationDelayPresent = (fourthByte >> 4) & 1
	if c.initialPresentationDelayPresent == 1 {
		c.initialPresentationDelayMinusOne = fourthByte & 0x0F
	}

	// Read any remaining configOBUs
	remaining := make([]byte, 0)
	for {
		b, err := br.readUint8()
		if err != nil {
			break // End of data
		}
		remaining = append(remaining, b)
	}
	c.configOBUs = remaining

	if !br.ok() {
		return nil, br.err
	}

	return ib, nil
}
