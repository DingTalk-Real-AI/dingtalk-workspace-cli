// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sign

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	elfClass32 = 1
	elfClass64 = 2
	elfDataLSB = 1
	elfDataMSB = 2

	sectionTypeProgbits = 1
	sectionTypeStrtab   = 3
	sectionTypeNobits   = 8

	pageSize = 4096

	segmentTypeLoad = 1

	maxSectionAlignment = 1 << 20
)

type elfHeader struct {
	class     byte
	order     binary.ByteOrder
	phoff     uint64
	phentsize uint16
	phnum     uint16
	shoff     uint64
	shentsize uint16
	shnum     uint16
	shstrndx  uint16
}

type elfSegment struct {
	typ    uint32
	offset uint64
	filesz uint64
}

type elfSection struct {
	name       string
	typ        uint32
	flags      uint64
	addr       uint64
	offset     uint64
	size       uint64
	nameOffset uint32
	link       uint32
	info       uint32
	addralign  uint64
	entsize    uint64
}

type elfImage struct {
	header   elfHeader
	sections []elfSection
	segments []elfSegment
	data     []byte
}

type rewrittenELF struct {
	data       []byte
	codeOffset int
}

func parseELF(data []byte) (*elfImage, error) {
	if len(data) < 16 || string(data[:4]) != "\x7fELF" {
		return nil, errors.New("input is not an ELF file")
	}
	if data[6] != 1 {
		return nil, errors.New("unsupported ELF version")
	}
	class := data[4]
	if class != elfClass32 && class != elfClass64 {
		return nil, fmt.Errorf("unsupported ELF class %d", class)
	}
	var order binary.ByteOrder
	switch data[5] {
	case elfDataLSB:
		order = binary.LittleEndian
	case elfDataMSB:
		order = binary.BigEndian
	default:
		return nil, fmt.Errorf("unsupported ELF byte order %d", data[5])
	}
	headerSize := 52
	sectionHeaderSize := 40
	if class == elfClass64 {
		headerSize = 64
		sectionHeaderSize = 64
	}
	if len(data) < headerSize {
		return nil, errors.New("truncated ELF header")
	}
	h := elfHeader{class: class, order: order}
	if class == elfClass32 {
		h.phoff = uint64(order.Uint32(data[28:32]))
		h.shoff = uint64(order.Uint32(data[32:36]))
		h.phentsize = order.Uint16(data[42:44])
		h.phnum = order.Uint16(data[44:46])
		h.shentsize = order.Uint16(data[46:48])
		h.shnum = order.Uint16(data[48:50])
		h.shstrndx = order.Uint16(data[50:52])
	} else {
		h.phoff = order.Uint64(data[32:40])
		h.shoff = order.Uint64(data[40:48])
		h.phentsize = order.Uint16(data[54:56])
		h.phnum = order.Uint16(data[56:58])
		h.shentsize = order.Uint16(data[58:60])
		h.shnum = order.Uint16(data[60:62])
		h.shstrndx = order.Uint16(data[62:64])
	}
	if h.phnum > 0 {
		if h.phentsize == 0 || !rangeOK(h.phoff, uint64(h.phentsize)*uint64(h.phnum), len(data)) {
			return nil, errors.New("invalid ELF program header table")
		}
	}
	segments := make([]elfSegment, h.phnum)
	for i := range segments {
		offset := h.phoff + uint64(i)*uint64(h.phentsize)
		segments[i] = readSegmentHeader(data[offset:], h.class, h.order)
		if !rangeOK(segments[i].offset, segments[i].filesz, len(data)) {
			return nil, errors.New("invalid ELF program segment extent")
		}
	}
	if h.shnum == 0 {
		return &elfImage{header: h, segments: segments, data: data}, nil
	}
	if h.shentsize != uint16(sectionHeaderSize) || !rangeOK(h.shoff, uint64(h.shentsize)*uint64(h.shnum), len(data)) {
		return nil, errors.New("invalid ELF section header table")
	}
	if h.shstrndx >= h.shnum {
		return nil, errors.New("invalid ELF section name table index")
	}
	sections := make([]elfSection, h.shnum)
	for i := range sections {
		offset := h.shoff + uint64(i)*uint64(h.shentsize)
		sections[i] = readSectionHeader(data[offset:], h.class, h.order)
	}
	nameTable := sections[h.shstrndx]
	if nameTable.typ != sectionTypeStrtab || !rangeOK(nameTable.offset, nameTable.size, len(data)) {
		return nil, errors.New("invalid ELF section name table")
	}
	nameData := data[nameTable.offset : nameTable.offset+nameTable.size]
	for i := range sections {
		name, err := sectionName(nameData, sections[i].nameOffset)
		if err != nil {
			return nil, fmt.Errorf("section %d name: %w", i, err)
		}
		sections[i].name = name
		if sections[i].typ != sectionTypeNobits && !rangeOK(sections[i].offset, sections[i].size, len(data)) {
			return nil, fmt.Errorf("section %d data is outside the file", i)
		}
	}
	return &elfImage{header: h, sections: sections, segments: segments, data: data}, nil
}

// readSegmentHeader decodes one program-header entry; the caller has already
// validated the table range and entry size.
func readSegmentHeader(data []byte, class byte, order binary.ByteOrder) elfSegment {
	segment := elfSegment{}
	if class == elfClass32 {
		segment.typ = order.Uint32(data[0:4])
		segment.offset = uint64(order.Uint32(data[4:8]))
		segment.filesz = uint64(order.Uint32(data[16:20]))
		return segment
	}
	segment.typ = order.Uint32(data[0:4])
	segment.offset = order.Uint64(data[8:16])
	segment.filesz = order.Uint64(data[32:40])
	return segment
}

// readSectionHeader decodes one complete entry; the caller has already
// validated the table range and entry size, so entries are never truncated.
func readSectionHeader(data []byte, class byte, order binary.ByteOrder) elfSection {
	section := elfSection{}
	if class == elfClass32 {
		section.nameOffset = order.Uint32(data[0:4])
		section.typ = order.Uint32(data[4:8])
		section.flags = uint64(order.Uint32(data[8:12]))
		section.addr = uint64(order.Uint32(data[12:16]))
		section.offset = uint64(order.Uint32(data[16:20]))
		section.size = uint64(order.Uint32(data[20:24]))
		section.link = order.Uint32(data[24:28])
		section.info = order.Uint32(data[28:32])
		section.addralign = uint64(order.Uint32(data[32:36]))
		section.entsize = uint64(order.Uint32(data[36:40]))
		return section
	}
	section.nameOffset = order.Uint32(data[0:4])
	section.typ = order.Uint32(data[4:8])
	section.flags = order.Uint64(data[8:16])
	section.addr = order.Uint64(data[16:24])
	section.offset = order.Uint64(data[24:32])
	section.size = order.Uint64(data[32:40])
	section.link = order.Uint32(data[40:44])
	section.info = order.Uint32(data[44:48])
	section.addralign = order.Uint64(data[48:56])
	section.entsize = order.Uint64(data[56:64])
	return section
}

func sectionName(table []byte, offset uint32) (string, error) {
	if offset >= uint32(len(table)) {
		return "", errors.New("name offset is outside the string table")
	}
	end := offset
	for end < uint32(len(table)) && table[end] != 0 {
		end++
	}
	if end == uint32(len(table)) {
		return "", errors.New("unterminated section name")
	}
	return string(table[offset:end]), nil
}

func rewriteELF(image *elfImage) (rewrittenELF, error) {
	if len(image.sections) == 0 {
		return rewrittenELF{}, errors.New("ELF has no section table")
	}
	cut := uint64(len(image.data))
	for index := 1; index < len(image.sections); index++ {
		section := image.sections[index]
		if section.name == ".codesign" && section.typ != sectionTypeNobits && section.offset < cut {
			cut = section.offset
		}
	}
	if cut < uint64(expectedELFHeaderSize(image.header.class)) {
		return rewrittenELF{}, errors.New("existing .codesign section overlaps the ELF header")
	}
	for _, segment := range image.segments {
		if segment.typ == segmentTypeLoad && segment.offset+segment.filesz > cut {
			return rewrittenELF{}, errors.New("loadable segment extends beyond the retired .codesign section")
		}
	}
	oldTableEnd := image.header.shoff + uint64(image.header.shentsize)*uint64(image.header.shnum)
	base := append([]byte(nil), image.data[:cut]...)
	if rangeOK(image.header.shoff, oldTableEnd-image.header.shoff, len(base)) {
		zeroRange(base, image.header.shoff, oldTableEnd-image.header.shoff)
	}
	// Retire the old signature and name-table extents before relocation so a
	// section moved in front of the cut can never land inside a region that
	// is about to be zeroed.
	for index := 1; index < len(image.sections); index++ {
		section := image.sections[index]
		if index == int(image.header.shstrndx) || isSignatureSection(section.name) {
			if section.typ != sectionTypeNobits && rangeOK(section.offset, section.size, len(base)) {
				zeroRange(base, section.offset, section.size)
			}
		}
	}
	kept := make([]elfSection, 0, len(image.sections)+2)
	kept = append(kept, image.sections[0])
	for index := 1; index < len(image.sections); index++ {
		section := image.sections[index]
		if index == int(image.header.shstrndx) || isSignatureSection(section.name) {
			continue
		}
		if section.typ != sectionTypeNobits && section.offset+section.size > cut {
			// The section lived beyond a previous .codesign block; move its
			// bytes in front of the cut so re-signing keeps them while the
			// layout stays byte-stable across repeated runs. Placement honors
			// the declared sh_addralign so the section stays loadable.
			alignment := section.addralign
			if alignment == 0 {
				alignment = 1
			}
			if alignment&(alignment-1) != 0 || alignment > maxSectionAlignment {
				return rewrittenELF{}, fmt.Errorf("section %s declares unsupported alignment %d", section.name, alignment)
			}
			originalOffset := section.offset
			section.offset = alignUp(uint64(len(base)), alignment)
			base = append(base, make([]byte, int(section.offset)-len(base))...)
			base = append(base, image.data[originalOffset:originalOffset+section.size]...)
		}
		kept = append(kept, section)
	}
	codeOffset := alignUp(uint64(len(base)), pageSize)
	shstrIndex := len(kept)
	kept = append(kept, elfSection{name: ".shstrtab", typ: sectionTypeStrtab, addralign: 1})
	kept = append(kept, elfSection{name: ".codesign", typ: sectionTypeProgbits, offset: codeOffset, size: pageSize, addralign: pageSize})
	stringTable, nameOffsets := makeStringTable(kept)
	sectionHeaderOffset := alignUp(codeOffset+pageSize, uint64(wordSize(image.header.class)))
	sectionHeaderSize := uint64(expectedSectionHeaderSize(image.header.class))
	// Reuse the original string-table slot only while the regrown table still
	// fits inside the retired section's own extent; growing beyond it could
	// overwrite data of any section that follows. Otherwise relocate the table
	// behind the new section-header table, where it can never collide.
	oldShstr := image.sections[image.header.shstrndx]
	shstrOffset := oldShstr.offset
	if shstrOffset < uint64(expectedELFHeaderSize(image.header.class)) || uint64(len(stringTable)) > oldShstr.size {
		shstrOffset = sectionHeaderOffset + sectionHeaderSize*uint64(len(kept))
	}
	kept[shstrIndex].offset = shstrOffset
	kept[shstrIndex].size = uint64(len(stringTable))
	for i := range kept {
		kept[i].nameOffset = nameOffsets[i]
	}
	newSize := sectionHeaderOffset + sectionHeaderSize*uint64(len(kept))
	if shstrOffset+uint64(len(stringTable)) > newSize {
		newSize = shstrOffset + uint64(len(stringTable))
	}
	if len(kept) > int(^uint16(0)) {
		return rewrittenELF{}, errors.New("too many ELF sections")
	}
	output := make([]byte, int(newSize))
	copy(output, base)
	copy(output[codeOffset:], make([]byte, pageSize))
	copy(output[shstrOffset:], stringTable)
	writeSectionHeaders(output[sectionHeaderOffset:], kept, image.header.class, image.header.order)
	writeELFHeader(output, image.header, sectionHeaderOffset, uint16(len(kept)), uint16(shstrIndex))
	return rewrittenELF{data: output, codeOffset: int(codeOffset)}, nil
}

func makeStringTable(sections []elfSection) ([]byte, []uint32) {
	table := []byte{0}
	offsets := make([]uint32, len(sections))
	for i := 1; i < len(sections); i++ {
		offsets[i] = uint32(len(table))
		table = append(table, sections[i].name...)
		table = append(table, 0)
	}
	return table, offsets
}

func writeELFHeader(data []byte, header elfHeader, sectionOffset uint64, sectionCount, nameIndex uint16) {
	if header.class == elfClass32 {
		header.order.PutUint32(data[32:36], uint32(sectionOffset))
		header.order.PutUint16(data[48:50], sectionCount)
		header.order.PutUint16(data[50:52], nameIndex)
		return
	}
	header.order.PutUint64(data[40:48], sectionOffset)
	header.order.PutUint16(data[60:62], sectionCount)
	header.order.PutUint16(data[62:64], nameIndex)
}

func writeSectionHeaders(data []byte, sections []elfSection, class byte, order binary.ByteOrder) {
	width := expectedSectionHeaderSize(class)
	for i, section := range sections {
		out := data[i*width : (i+1)*width]
		if class == elfClass32 {
			order.PutUint32(out[0:4], section.nameOffset)
			order.PutUint32(out[4:8], section.typ)
			order.PutUint32(out[8:12], uint32(section.flags))
			order.PutUint32(out[12:16], uint32(section.addr))
			order.PutUint32(out[16:20], uint32(section.offset))
			order.PutUint32(out[20:24], uint32(section.size))
			order.PutUint32(out[24:28], section.link)
			order.PutUint32(out[28:32], section.info)
			order.PutUint32(out[32:36], uint32(section.addralign))
			order.PutUint32(out[36:40], uint32(section.entsize))
			continue
		}
		order.PutUint32(out[0:4], section.nameOffset)
		order.PutUint32(out[4:8], section.typ)
		order.PutUint64(out[8:16], section.flags)
		order.PutUint64(out[16:24], section.addr)
		order.PutUint64(out[24:32], section.offset)
		order.PutUint64(out[32:40], section.size)
		order.PutUint32(out[40:44], section.link)
		order.PutUint32(out[44:48], section.info)
		order.PutUint64(out[48:56], section.addralign)
		order.PutUint64(out[56:64], section.entsize)
	}
}

func isSignatureSection(name string) bool {
	return name == ".codesign" || name == ".profile" || name == ".permission"
}

func expectedELFHeaderSize(class byte) int {
	if class == elfClass32 {
		return 52
	}
	return 64
}

func expectedSectionHeaderSize(class byte) int {
	if class == elfClass32 {
		return 40
	}
	return 64
}

func wordSize(class byte) int {
	if class == elfClass32 {
		return 4
	}
	return 8
}

func rangeOK(offset, size uint64, length int) bool {
	return offset <= uint64(length) && size <= uint64(length)-offset
}

// zeroRange clears an already-validated in-file range.
func zeroRange(data []byte, offset, size uint64) {
	for i := offset; i < offset+size; i++ {
		data[i] = 0
	}
}

func alignUp(value, alignment uint64) uint64 {
	rem := value % alignment
	if rem == 0 {
		return value
	}
	return value + alignment - rem
}
