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
	"bytes"
	"debug/elf"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageParseELFRejectsMalformedImages(t *testing.T) {
	cases := []struct {
		name   string
		mutate func([]byte) []byte
	}{
		{"unsupported version", func(b []byte) []byte { b[6] = 2; return b }},
		{"unsupported class", func(b []byte) []byte { b[4] = 3; return b }},
		{"unsupported byte order", func(b []byte) []byte { b[5] = 0; return b }},
		{"truncated header", func(b []byte) []byte { return b[:60] }},
		{"zero program header entry size", func(b []byte) []byte {
			binary.LittleEndian.PutUint16(b[56:58], 1)
			return b
		}},
		{"program header table outside file", func(b []byte) []byte {
			binary.LittleEndian.PutUint16(b[54:56], 56)
			binary.LittleEndian.PutUint16(b[56:58], 4)
			binary.LittleEndian.PutUint64(b[32:40], 1<<40)
			return b
		}},
		{"section header entry size mismatch", func(b []byte) []byte {
			binary.LittleEndian.PutUint16(b[58:60], 65)
			return b
		}},
		{"section header table outside file", func(b []byte) []byte {
			binary.LittleEndian.PutUint64(b[40:48], 1<<40)
			return b
		}},
		{"section name table index out of range", func(b []byte) []byte {
			binary.LittleEndian.PutUint16(b[62:64], 3)
			return b
		}},
		{"name table is not a string table", func(b []byte) []byte {
			binary.LittleEndian.PutUint32(b[0x200+128+4:0x200+128+8], 1)
			return b
		}},
		{"name table outside file", func(b []byte) []byte {
			binary.LittleEndian.PutUint64(b[0x200+128+24:0x200+128+32], 1<<40)
			return b
		}},
		{"section name offset outside table", func(b []byte) []byte {
			binary.LittleEndian.PutUint32(b[0x200+64:0x200+64+4], 0xffff)
			return b
		}},
		{"unterminated section name", func(b []byte) []byte {
			binary.LittleEndian.PutUint64(b[0x200+128+32:0x200+128+40], 16)
			return b
		}},
		{"section data outside file", func(b []byte) []byte {
			binary.LittleEndian.PutUint64(b[0x200+64+24:0x200+64+32], 1<<40)
			binary.LittleEndian.PutUint64(b[0x200+64+32:0x200+64+40], 1)
			return b
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			image := testCase.mutate(minimalELF())
			if _, err := parseELF(image); err == nil {
				t.Fatal("malformed ELF was accepted")
			}
		})
	}
}

func TestCrossPlatformCoverageSignFileRejectsInvalidInputsAndOutputs(t *testing.T) {
	directory := t.TempDir()
	if err := SignFile("", ""); err == nil {
		t.Fatal("empty paths were accepted")
	}
	if err := SignFile(filepath.Join(directory, "missing"), filepath.Join(directory, "out")); err == nil ||
		!strings.Contains(err.Error(), "stat input") {
		t.Fatalf("missing input error = %v", err)
	}
	if err := SignFile(directory, filepath.Join(directory, "out")); err == nil ||
		!strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("directory input error = %v", err)
	}
	noSections := minimalELF()
	binary.LittleEndian.PutUint16(noSections[60:62], 0)
	inputPath := filepath.Join(directory, "nosections")
	if err := os.WriteFile(inputPath, noSections, 0600); err != nil {
		t.Fatal(err)
	}
	if err := SignFile(inputPath, filepath.Join(directory, "out")); err == nil ||
		!strings.Contains(err.Error(), "no section table") {
		t.Fatalf("sectionless ELF error = %v", err)
	}
	unreadablePath := filepath.Join(directory, "unreadable")
	if err := os.WriteFile(unreadablePath, minimalELF(), 0600); err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &readInputFile, func(string) ([]byte, error) {
		return nil, errors.New("read unavailable")
	})
	if err := SignFile(unreadablePath, filepath.Join(directory, "out")); err == nil ||
		!strings.Contains(err.Error(), "read input") {
		t.Fatalf("unreadable input error = %v", err)
	}
	blocker := filepath.Join(directory, "blocker")
	if err := os.WriteFile(blocker, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := SignFile(inputPath, filepath.Join(blocker, "out")); err == nil {
		t.Fatal("output under a regular file was accepted")
	}
}

func TestCrossPlatformCoverageRewriteRejectsTooManySections(t *testing.T) {
	const entrySize = 64
	const sectionCount = 65535
	tableOffset := uint64(64)
	stringsOffset := tableOffset + sectionCount*entrySize
	sectionNames := []byte("\x00.shstrtab\x00")
	data := make([]byte, int(stringsOffset)+len(sectionNames)+16)
	copy(data[:4], []byte{0x7f, 'E', 'L', 'F'})
	data[4] = 2
	data[5] = 1
	data[6] = 1
	binary.LittleEndian.PutUint16(data[16:18], 2)
	binary.LittleEndian.PutUint16(data[18:20], 183)
	binary.LittleEndian.PutUint64(data[40:48], tableOffset)
	binary.LittleEndian.PutUint16(data[58:60], entrySize)
	binary.LittleEndian.PutUint16(data[60:62], sectionCount)
	binary.LittleEndian.PutUint16(data[62:64], 1)
	copy(data[stringsOffset:], sectionNames)
	strtab := tableOffset + entrySize
	binary.LittleEndian.PutUint32(data[strtab+4:strtab+8], 3)
	binary.LittleEndian.PutUint64(data[strtab+24:strtab+32], stringsOffset)
	binary.LittleEndian.PutUint64(data[strtab+32:strtab+40], uint64(len(sectionNames)))
	binary.LittleEndian.PutUint64(data[strtab+48:strtab+56], 1)
	image, err := parseELF(data)
	if err != nil {
		t.Fatalf("fixture did not parse: %v", err)
	}
	if _, err := rewriteELF(image); err == nil || !strings.Contains(err.Error(), "too many ELF sections") {
		t.Fatalf("section overflow error = %v", err)
	}
}

func TestCrossPlatformCoverageSignLargeELFBuildsDeepMerkleTree(t *testing.T) {
	directory := t.TempDir()
	inputPath := filepath.Join(directory, "large")
	outputPath := filepath.Join(directory, "signed")
	data := minimalELF()
	const extra = 700 * 1024
	data = append(data, make([]byte, extra)...)
	binary.LittleEndian.PutUint64(data[0x200+64+32:0x200+64+40], 1+extra)
	if err := os.WriteFile(inputPath, data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := SignFile(inputPath, outputPath); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() <= int64(extra) {
		t.Fatalf("signed output too small: %d", info.Size())
	}
}

func TestCrossPlatformCoverageMerkleRootSinglePage(t *testing.T) {
	single := make([]byte, 100)
	single[7] = 0x42
	root := merkleRoot(single, pageSize)
	if len(root) != 32 || string(root) == string(make([]byte, 32)) {
		t.Fatalf("single-page root invalid: %x", root)
	}
	if got := merkleRoot(nil, 0); got != nil {
		t.Fatalf("empty data root = %x, want nil", got)
	}
}

func TestCrossPlatformCoverageSignBigEndianELF(t *testing.T) {
	directory := t.TempDir()
	inputPath := filepath.Join(directory, "be")
	outputPath := filepath.Join(directory, "signed")
	if err := os.WriteFile(inputPath, bigEndianELF(), 0644); err != nil {
		t.Fatal(err)
	}
	if err := SignFile(inputPath, outputPath); err != nil {
		t.Fatal(err)
	}
	output, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if output[5] != 2 {
		t.Fatalf("signed output byte order = %d, want big endian", output[5])
	}
	if got := binary.BigEndian.Uint16(output[16:18]); got != 2 {
		t.Fatalf("signed output type = %d, want ET_EXEC", got)
	}
}

func TestCrossPlatformCoverageRewriteRelocatesOverflowingStringTable(t *testing.T) {
	directory := t.TempDir()
	inputPath := filepath.Join(directory, "boundary")
	outputPath := filepath.Join(directory, "signed")
	if err := os.WriteFile(inputPath, pageBoundaryELF(), 0644); err != nil {
		t.Fatal(err)
	}
	if err := SignFile(inputPath, outputPath); err != nil {
		t.Fatal(err)
	}
	parsed, err := elf.Open(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	defer parsed.Close()
	if parsed.Section(".codesign") == nil || parsed.Section(".shstrtab") == nil {
		t.Fatal("relocated string table lost required sections")
	}
}

func bigEndianELF() []byte {
	const sectionOffset = 0x200
	sectionNames := []byte("\x00.text\x00.shstrtab\x00")
	data := make([]byte, sectionOffset+3*64)
	copy(data[:4], []byte{0x7f, 'E', 'L', 'F'})
	data[4] = 2
	data[5] = 2
	data[6] = 1
	binary.BigEndian.PutUint16(data[16:18], 2)
	binary.BigEndian.PutUint16(data[18:20], 62)
	binary.BigEndian.PutUint32(data[20:24], 1)
	binary.BigEndian.PutUint64(data[40:48], sectionOffset)
	binary.BigEndian.PutUint16(data[58:60], 64)
	binary.BigEndian.PutUint16(data[60:62], 3)
	binary.BigEndian.PutUint16(data[62:64], 2)
	data[0x100] = 0xc3
	copy(data[0x101:], sectionNames)
	writeHeader := func(offset int, name, typ uint32, dataOffset, size uint64) {
		binary.BigEndian.PutUint32(data[offset:offset+4], name)
		binary.BigEndian.PutUint32(data[offset+4:offset+8], typ)
		binary.BigEndian.PutUint64(data[offset+24:offset+32], dataOffset)
		binary.BigEndian.PutUint64(data[offset+32:offset+40], size)
		binary.BigEndian.PutUint64(data[offset+48:offset+56], 1)
	}
	writeHeader(sectionOffset+64, 1, 1, 0x100, 1)
	writeHeader(sectionOffset+128, 7, 3, 0x101, uint64(len(sectionNames)))
	return data
}

func TestCrossPlatformCoverageRewritePreservesSectionsAfterStringTable(t *testing.T) {
	for _, shstrCapacity := range []uint32{26, 64} {
		t.Run(fmt.Sprintf("capacity-%d", shstrCapacity), func(t *testing.T) {
			directory := t.TempDir()
			inputPath := filepath.Join(directory, "input")
			outputPath := filepath.Join(directory, "signed")
			fixture, pattern := adjacentSectionELF(shstrCapacity)
			if err := os.WriteFile(inputPath, fixture, 0644); err != nil {
				t.Fatal(err)
			}
			if err := SignFile(inputPath, outputPath); err != nil {
				t.Fatal(err)
			}
			parsed, err := elf.Open(outputPath)
			if err != nil {
				t.Fatal(err)
			}
			defer parsed.Close()
			rodata := parsed.Section(".rodata")
			if rodata == nil {
				t.Fatal(".rodata section was dropped")
			}
			content, err := rodata.Data()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(content, pattern) {
				t.Fatalf(".rodata content corrupted: %x", content)
			}
			if parsed.Section(".codesign") == nil {
				t.Fatal(".codesign section missing")
			}
		})
	}
}

func adjacentSectionELF(shstrCapacity uint32) ([]byte, []byte) {
	const sectionOffset = 0x200
	const shstrOffset = 0x110
	names := []byte("\x00.text\x00.rodata\x00.shstrtab\x00")
	rodataOffset := (uint64(shstrOffset) + uint64(shstrCapacity) + 7) &^ 7
	pattern := make([]byte, 32)
	for i := range pattern {
		pattern[i] = byte(0xa0 + i%16)
	}
	data := make([]byte, sectionOffset+4*64)
	copy(data[:4], []byte{0x7f, 'E', 'L', 'F'})
	data[4] = 2
	data[5] = 1
	data[6] = 1
	binary.LittleEndian.PutUint16(data[16:18], 2)
	binary.LittleEndian.PutUint16(data[18:20], 62)
	binary.LittleEndian.PutUint32(data[20:24], 1)
	binary.LittleEndian.PutUint64(data[40:48], sectionOffset)
	binary.LittleEndian.PutUint16(data[58:60], 64)
	binary.LittleEndian.PutUint16(data[60:62], 4)
	binary.LittleEndian.PutUint16(data[62:64], 3)
	data[0x100] = 0xc3
	copy(data[shstrOffset:], names)
	copy(data[rodataOffset:], pattern)
	writeHeader := func(offset int, name, typ uint32, dataOffset, size uint64) {
		binary.LittleEndian.PutUint32(data[offset:offset+4], name)
		binary.LittleEndian.PutUint32(data[offset+4:offset+8], typ)
		binary.LittleEndian.PutUint64(data[offset+24:offset+32], dataOffset)
		binary.LittleEndian.PutUint64(data[offset+32:offset+40], size)
		binary.LittleEndian.PutUint64(data[offset+48:offset+56], 1)
	}
	writeHeader(sectionOffset+64, 1, 1, 0x100, 1)
	writeHeader(sectionOffset+128, 7, 1, rodataOffset, uint64(len(pattern)))
	writeHeader(sectionOffset+192, 15, 3, shstrOffset, uint64(shstrCapacity))
	return data, pattern
}

func TestCrossPlatformCoverageResignRejectsLoadableSegmentBeyondCodesign(t *testing.T) {
	directory := t.TempDir()
	inputPath := filepath.Join(directory, "input")
	outputPath := filepath.Join(directory, "output")
	data := sectionAfterCodesignELF()
	// Add one PT_LOAD whose file extent covers .tail at 0x2000, past the
	// retired .codesign block at 0x1000; the layout cannot be preserved.
	const phoff = 0x180
	binary.LittleEndian.PutUint64(data[32:40], phoff)
	binary.LittleEndian.PutUint16(data[54:56], 56)
	binary.LittleEndian.PutUint16(data[56:58], 1)
	binary.LittleEndian.PutUint32(data[phoff:phoff+4], 1)
	binary.LittleEndian.PutUint64(data[phoff+8:phoff+16], 0x2000)
	binary.LittleEndian.PutUint64(data[phoff+32:phoff+40], 64)
	if err := os.WriteFile(inputPath, data, 0644); err != nil {
		t.Fatal(err)
	}
	err := SignFile(inputPath, outputPath)
	if err == nil || !strings.Contains(err.Error(), "loadable segment extends beyond") {
		t.Fatalf("SignFile() = %v, want loadable-segment rejection", err)
	}
	if _, statErr := os.Stat(outputPath); !os.IsNotExist(statErr) {
		t.Fatal("rejected re-sign must not produce an output file")
	}
}

func TestCrossPlatformCoverageResignPreservesDeclaredAlignment(t *testing.T) {
	directory := t.TempDir()
	inputPath := filepath.Join(directory, "input")
	outputPath := filepath.Join(directory, "output")
	if err := os.WriteFile(inputPath, sectionAfterCodesignELF(), 0644); err != nil {
		t.Fatal(err)
	}
	if err := SignFile(inputPath, outputPath); err != nil {
		t.Fatal(err)
	}
	parsed, err := elf.Open(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	defer parsed.Close()
	extra := parsed.Section(".extra")
	if extra == nil {
		t.Fatal(".extra section was dropped while re-signing")
	}
	if extra.Addralign != 8192 {
		t.Fatalf(".extra sh_addralign = %d, want declared 8192", extra.Addralign)
	}
	if extra.Offset%8192 != 0 {
		t.Fatalf(".extra offset %#x is not 8192-aligned", extra.Offset)
	}
}

func TestCrossPlatformCoverageRejectsCodesignOverlappingElfHeader(t *testing.T) {
	directory := t.TempDir()
	inputPath := filepath.Join(directory, "input")
	outputPath := filepath.Join(directory, "output")
	data := sectionAfterCodesignELF()
	// Move the existing .codesign section to offset zero so the retired
	// extent covers the ELF header itself.
	binary.LittleEndian.PutUint64(data[0x200+320+24:0x200+320+32], 0)
	if err := os.WriteFile(inputPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	err := SignFile(inputPath, outputPath)
	if err == nil || !strings.Contains(err.Error(), "overlaps the ELF header") {
		t.Fatalf("SignFile() = %v, want ELF-header overlap rejection", err)
	}
	if _, statErr := os.Stat(outputPath); !os.IsNotExist(statErr) {
		t.Fatal("rejected re-sign must not produce an output file")
	}
}

func TestCrossPlatformCoverageRejectsInvalidSegmentExtent(t *testing.T) {
	data := sectionAfterCodesignELF()
	const phoff = 0x180
	binary.LittleEndian.PutUint64(data[32:40], phoff)
	binary.LittleEndian.PutUint16(data[54:56], 56)
	binary.LittleEndian.PutUint16(data[56:58], 1)
	binary.LittleEndian.PutUint32(data[phoff:phoff+4], 1)
	binary.LittleEndian.PutUint64(data[phoff+32:phoff+40], 1<<40)
	if _, err := parseELF(data); err == nil || !strings.Contains(err.Error(), "program segment extent") {
		t.Fatalf("parseELF() = %v, want segment extent rejection", err)
	}
}

func TestCrossPlatformCoverageResignRejectsInvalidSectionAlignment(t *testing.T) {
	data := sectionAfterCodesignELF()
	// .extra declares addralign 3 (not a power of two) and lives past the
	// retired .codesign block, so relocation must reject it.
	binary.LittleEndian.PutUint64(data[0x200+192+48:0x200+192+56], 3)
	image, err := parseELF(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rewriteELF(image); err == nil || !strings.Contains(err.Error(), "unsupported alignment") {
		t.Fatalf("rewriteELF() = %v, want alignment rejection", err)
	}
}

func TestCrossPlatformCoverageZeroSectionAlignmentRelocatesUnitAligned(t *testing.T) {
	data := sectionAfterCodesignELF()
	binary.LittleEndian.PutUint64(data[0x200+192+48:0x200+192+56], 0)
	directory := t.TempDir()
	inputPath := filepath.Join(directory, "input")
	outputPath := filepath.Join(directory, "output")
	if err := os.WriteFile(inputPath, data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := SignFile(inputPath, outputPath); err != nil {
		t.Fatal(err)
	}
	parsed, err := elf.Open(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	defer parsed.Close()
	if parsed.Section(".extra") == nil {
		t.Fatal(".extra section was dropped")
	}
}

func TestCrossPlatformCoverageSign32RejectsBadProgramHeader(t *testing.T) {
	data := minimalELF32()
	binary.LittleEndian.PutUint32(data[28:32], 0x180)
	binary.LittleEndian.PutUint16(data[42:44], 32)
	binary.LittleEndian.PutUint16(data[44:46], 1)
	binary.LittleEndian.PutUint32(data[0x180:0x184], 1)
	binary.LittleEndian.PutUint32(data[0x180+16:0x180+20], 1<<30)
	if _, err := parseELF(data); err == nil || !strings.Contains(err.Error(), "program segment extent") {
		t.Fatalf("parseELF32() = %v, want segment extent rejection", err)
	}
}

func TestCrossPlatformCoverageRejectsUndersizedProgramHeaderEntry(t *testing.T) {
	data := minimalELF()
	// One program-header entry of one byte anchored at the end of the file:
	// the table extent fits, but decoding the fixed-size entry would read
	// past the buffer, so parsing must reject instead of panicking.
	binary.LittleEndian.PutUint64(data[32:40], uint64(len(data))-8)
	binary.LittleEndian.PutUint16(data[54:56], 1)
	binary.LittleEndian.PutUint16(data[56:58], 1)
	if _, err := parseELF(data); err == nil || !strings.Contains(err.Error(), "program header table") {
		t.Fatalf("parseELF() = %v, want program header table rejection", err)
	}

	data32 := minimalELF32()
	binary.LittleEndian.PutUint32(data32[28:32], uint32(len(data32))-8)
	binary.LittleEndian.PutUint16(data32[42:44], 4)
	binary.LittleEndian.PutUint16(data32[44:46], 1)
	if _, err := parseELF(data32); err == nil || !strings.Contains(err.Error(), "program header table") {
		t.Fatalf("parseELF32() = %v, want program header table rejection", err)
	}
}

func TestCrossPlatformCoverageSignFileRejectsPanicInputWithoutOutput(t *testing.T) {
	directory := t.TempDir()
	inputPath := filepath.Join(directory, "input")
	outputPath := filepath.Join(directory, "output")
	data := minimalELF()
	binary.LittleEndian.PutUint64(data[32:40], uint64(len(data))-8)
	binary.LittleEndian.PutUint16(data[54:56], 1)
	binary.LittleEndian.PutUint16(data[56:58], 1)
	if err := os.WriteFile(inputPath, data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := SignFile(inputPath, outputPath); err == nil ||
		!strings.Contains(err.Error(), "program header table") {
		t.Fatalf("SignFile() = %v, want program header table rejection", err)
	}
	if _, statErr := os.Stat(outputPath); !os.IsNotExist(statErr) {
		t.Fatal("rejected input must not produce an output file")
	}
}

func TestCrossPlatformCoverageRejectsCorruptedRewrittenOutput(t *testing.T) {
	directory := t.TempDir()
	inputPath := filepath.Join(directory, "input")
	outputPath := filepath.Join(directory, "output")
	if err := os.WriteFile(inputPath, minimalELF(), 0644); err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &validateOutput, func([]byte) (*elfImage, error) {
		return nil, errors.New("corrupted output")
	})
	err := SignFile(inputPath, outputPath)
	if err == nil || !strings.Contains(err.Error(), "self-validation") {
		t.Fatalf("SignFile() = %v, want self-validation rejection", err)
	}
	if _, statErr := os.Stat(outputPath); !os.IsNotExist(statErr) {
		t.Fatal("failed self-validation must not produce an output file")
	}
}

func TestCrossPlatformCoverageResignPreservesSectionsAfterCodesign(t *testing.T) {
	directory := t.TempDir()
	inputPath := filepath.Join(directory, "input")
	firstPath := filepath.Join(directory, "first")
	secondPath := filepath.Join(directory, "second")
	if err := os.WriteFile(inputPath, sectionAfterCodesignELF(), 0644); err != nil {
		t.Fatal(err)
	}
	if err := SignFile(inputPath, firstPath); err != nil {
		t.Fatal(err)
	}
	parsed, err := elf.Open(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	defer parsed.Close()
	tail := parsed.Section(".tail")
	if tail == nil {
		t.Fatal(".tail section was dropped while re-signing")
	}
	content, err := tail.Data()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(content, tailPattern()) {
		t.Fatalf(".tail content corrupted: %x", content)
	}
	extra := parsed.Section(".extra")
	if extra == nil {
		t.Fatal(".extra section was dropped while re-signing")
	}
	extraContent, err := extra.Data()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(extraContent, []byte{0xAB, 0xCD, 0xEF, 0x99}) {
		t.Fatalf(".extra content corrupted: %x", extraContent)
	}
	if err := SignFile(firstPath, secondPath); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(secondPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("re-signing a signed ELF is not byte-stable")
	}
}

func tailPattern() []byte {
	pattern := make([]byte, 64)
	for i := range pattern {
		pattern[i] = byte(0x30 + i%10)
	}
	return pattern
}

func sectionAfterCodesignELF() []byte {
	const shoff = 0x200
	const codesignOffset = 0x1000
	const tailOffset = 0x2000
	const extraOffset = 0x2100
	names := []byte("\x00.text\x00.tail\x00.extra\x00.shstrtab\x00.codesign\x00")
	pattern := tailPattern()
	data := make([]byte, extraOffset+4+6*64+16)
	copy(data[:4], []byte{0x7f, 'E', 'L', 'F'})
	data[4] = 2
	data[5] = 1
	data[6] = 1
	binary.LittleEndian.PutUint16(data[16:18], 2)
	binary.LittleEndian.PutUint16(data[18:20], 62)
	binary.LittleEndian.PutUint32(data[20:24], 1)
	binary.LittleEndian.PutUint64(data[40:48], shoff)
	binary.LittleEndian.PutUint16(data[58:60], 64)
	binary.LittleEndian.PutUint16(data[60:62], 6)
	binary.LittleEndian.PutUint16(data[62:64], 4)
	data[0x100] = 0xc3
	copy(data[0x110:], names)
	copy(data[tailOffset:], pattern)
	copy(data[extraOffset:], []byte{0xAB, 0xCD, 0xEF, 0x99})
	writeHeader := func(offset int, name, typ uint32, dataOffset, size, addralign uint64) {
		binary.LittleEndian.PutUint32(data[offset:offset+4], name)
		binary.LittleEndian.PutUint32(data[offset+4:offset+8], typ)
		binary.LittleEndian.PutUint64(data[offset+24:offset+32], dataOffset)
		binary.LittleEndian.PutUint64(data[offset+32:offset+40], size)
		binary.LittleEndian.PutUint64(data[offset+48:offset+56], addralign)
	}
	writeHeader(shoff+64, 1, 1, 0x100, 1, 1)
	writeHeader(shoff+128, 7, 1, tailOffset, uint64(len(pattern)), 16)
	writeHeader(shoff+192, 13, 1, extraOffset, 4, 8192)
	writeHeader(shoff+256, 20, 3, 0x110, uint64(len(names)), 1)
	writeHeader(shoff+320, 30, 1, codesignOffset, pageSize, pageSize)
	return data
}

func pageBoundaryELF() []byte {
	const fileSize = 4094
	const shoff = 0x40
	sectionNames := []byte("\x00.text\x00.shstrtab\x00")
	data := make([]byte, fileSize)
	copy(data[:4], []byte{0x7f, 'E', 'L', 'F'})
	data[4] = 2
	data[5] = 1
	data[6] = 1
	binary.LittleEndian.PutUint16(data[16:18], 2)
	binary.LittleEndian.PutUint16(data[18:20], 62)
	binary.LittleEndian.PutUint32(data[20:24], 1)
	binary.LittleEndian.PutUint64(data[40:48], shoff)
	binary.LittleEndian.PutUint16(data[58:60], 64)
	binary.LittleEndian.PutUint16(data[60:62], 3)
	binary.LittleEndian.PutUint16(data[62:64], 2)
	data[0x100] = 0xc3
	copy(data[fileSize-len(sectionNames):], sectionNames)
	writeHeader := func(offset int, name, typ uint32, dataOffset, size uint64) {
		binary.LittleEndian.PutUint32(data[offset:offset+4], name)
		binary.LittleEndian.PutUint32(data[offset+4:offset+8], typ)
		binary.LittleEndian.PutUint64(data[offset+24:offset+32], dataOffset)
		binary.LittleEndian.PutUint64(data[offset+32:offset+40], size)
		binary.LittleEndian.PutUint64(data[offset+48:offset+56], 1)
	}
	writeHeader(shoff+64, 1, 1, 0x100, 1)
	writeHeader(shoff+128, 7, 3, uint64(fileSize-len(sectionNames)), uint64(len(sectionNames)))
	return data
}

func FuzzParseELFNeverPanics(f *testing.F) {
	f.Add(minimalELF())
	f.Add(minimalELF32())
	f.Add(bigEndianELF())
	f.Add(sectionAfterCodesignELF())
	seed := minimalELF()
	binary.LittleEndian.PutUint64(seed[32:40], uint64(len(seed))-8)
	binary.LittleEndian.PutUint16(seed[54:56], 1)
	binary.LittleEndian.PutUint16(seed[56:58], 1)
	f.Add(seed)
	f.Fuzz(func(t *testing.T, data []byte) {
		image, err := parseELF(data)
		if err == nil {
			_, _ = rewriteELF(image)
		}
	})
}
