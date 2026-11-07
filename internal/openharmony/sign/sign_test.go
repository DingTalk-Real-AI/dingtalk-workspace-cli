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
	"crypto/sha256"
	"debug/elf"
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCrossPlatformCoverageSignFileWritesSelfSignedELF(t *testing.T) {
	directory := t.TempDir()
	inputPath := filepath.Join(directory, "input")
	outputPath := filepath.Join(directory, "output")
	if err := os.WriteFile(inputPath, minimalELF(), 0751); err != nil {
		t.Fatal(err)
	}
	if err := SignFile(inputPath, outputPath); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if got, want := info.Mode().Perm(), os.FileMode(0751); got != want {
			t.Fatalf("mode = %o, want %o", got, want)
		}
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := elf.Open(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	defer parsed.Close()
	if parsed.Section(".profile") != nil || parsed.Section(".permission") != nil {
		t.Fatal("signature metadata sections were not removed")
	}
	codeSign := parsed.Section(".codesign")
	if codeSign == nil || codeSign.Size != pageSize || codeSign.Offset%pageSize != 0 {
		t.Fatalf("invalid .codesign section: %#v", codeSign)
	}
	block, err := codeSign.Data()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := binary.LittleEndian.Uint32(block[0:4]), uint32(1); got != want {
		t.Fatalf("block type = %d, want %d", got, want)
	}
	if got, want := binary.LittleEndian.Uint32(block[4:8]), uint32(288); got != want {
		t.Fatalf("block length = %d, want %d", got, want)
	}
	descriptorBytes := block[8 : 8+descriptorSize]
	if got, want := binary.LittleEndian.Uint32(descriptorBytes[4:8]), uint32(signatureSize); got != want {
		t.Fatalf("signature size = %d, want %d", got, want)
	}
	if got, want := binary.LittleEndian.Uint64(descriptorBytes[8:16]), uint64(len(data)); got != want {
		t.Fatalf("file size = %d, want %d", got, want)
	}
	if got, want := binary.LittleEndian.Uint32(descriptorBytes[112:116]), uint32(selfSignFlag); got != want {
		t.Fatalf("flags = %x, want %x", got, want)
	}
	if got, want := descriptorBytes[255], byte(codeSignVersion); got != want {
		t.Fatalf("code sign version = %d, want %d", got, want)
	}
	root := expectedMerkleRoot(data, int(codeSign.Offset))
	if string(descriptorBytes[16:48]) != string(root) {
		t.Fatal("descriptor root hash does not match fs-verity root")
	}
	unsigned := descriptor(len(data), root, 0)
	wantSignature := sha256.Sum256(unsigned)
	if string(block[264:296]) != string(wantSignature[:]) {
		t.Fatal("self-signature does not match unsigned descriptor digest")
	}
}

func TestCrossPlatformCoverageSignFileSupportsELF32(t *testing.T) {
	directory := t.TempDir()
	inputPath := filepath.Join(directory, "input32")
	outputPath := filepath.Join(directory, "output32")
	if err := os.WriteFile(inputPath, minimalELF32(), 0644); err != nil {
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
	if parsed.Class != elf.ELFCLASS32 {
		t.Fatalf("class = %v, want ELF32", parsed.Class)
	}
	codeSign := parsed.Section(".codesign")
	if codeSign == nil || codeSign.Size != pageSize || codeSign.Offset%pageSize != 0 {
		t.Fatalf("invalid ELF32 .codesign section: %#v", codeSign)
	}
}

func TestCrossPlatformCoverageSignFileSamePathIsDeterministic(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "binary")
	if err := os.WriteFile(path, minimalELF(), 0701); err != nil {
		t.Fatal(err)
	}
	if err := SignFile(path, path); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := SignFile(path, path); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("same-path signing is not deterministic")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if got, want := info.Mode().Perm(), os.FileMode(0701); got != want {
			t.Fatalf("mode = %o, want %o", got, want)
		}
	}
}

func TestCrossPlatformCoverageSignFileRejectsMalformedInputWithoutReplacingOutput(t *testing.T) {
	directory := t.TempDir()
	inputPath := filepath.Join(directory, "input")
	outputPath := filepath.Join(directory, "output")
	if err := os.WriteFile(inputPath, []byte("not an ELF"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outputPath, []byte("old output"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SignFile(inputPath, outputPath); err == nil {
		t.Fatal("malformed ELF was accepted")
	}
	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old output" {
		t.Fatal("failed signing replaced existing output")
	}
}

func minimalELF32() []byte {
	const sectionOffset = 0x180
	strings := []byte("\x00.text\x00.shstrtab\x00")
	data := make([]byte, sectionOffset+3*40)
	copy(data[:4], []byte{0x7f, 'E', 'L', 'F'})
	data[4] = byte(elf.ELFCLASS32)
	data[5] = byte(elf.ELFDATA2LSB)
	data[6] = 1
	binary.LittleEndian.PutUint16(data[16:18], uint16(elf.ET_EXEC))
	binary.LittleEndian.PutUint16(data[18:20], uint16(elf.EM_ARM))
	binary.LittleEndian.PutUint32(data[20:24], 1)
	binary.LittleEndian.PutUint32(data[32:36], sectionOffset)
	binary.LittleEndian.PutUint16(data[40:42], 52)
	binary.LittleEndian.PutUint16(data[46:48], 40)
	binary.LittleEndian.PutUint16(data[48:50], 3)
	binary.LittleEndian.PutUint16(data[50:52], 2)
	data[0x100] = 0xe0
	copy(data[0x101:], strings)
	writeSectionHeader := func(offset int, name, typ, dataOffset, size uint32) {
		binary.LittleEndian.PutUint32(data[offset:offset+4], name)
		binary.LittleEndian.PutUint32(data[offset+4:offset+8], typ)
		binary.LittleEndian.PutUint32(data[offset+16:offset+20], dataOffset)
		binary.LittleEndian.PutUint32(data[offset+20:offset+24], size)
		binary.LittleEndian.PutUint32(data[offset+28:offset+32], 1)
	}
	writeSectionHeader(sectionOffset+40, 1, uint32(elf.SHT_PROGBITS), 0x100, 1)
	writeSectionHeader(sectionOffset+80, 7, uint32(elf.SHT_STRTAB), 0x101, uint32(len(strings)))
	return data
}

func expectedMerkleRoot(data []byte, codeOffset int) []byte {
	leafCount := (len(data) + pageSize - 1) / pageSize
	leaves := make([]byte, leafCount*sha256.Size)
	for index := 0; index < leafCount; index++ {
		var page [pageSize]byte
		start := index * pageSize
		end := start + pageSize
		if end > len(data) {
			end = len(data)
		}
		copy(page[:], data[start:end])
		digest := sha256.Sum256(page[:])
		if start/pageSize != codeOffset/pageSize {
			copy(leaves[index*sha256.Size:], digest[:])
		}
	}
	var hashPage [pageSize]byte
	copy(hashPage[:], leaves)
	root := sha256.Sum256(hashPage[:])
	return root[:]
}

func minimalELF() []byte {
	const sectionOffset = 0x200
	strings := []byte("\x00.text\x00.shstrtab\x00")
	data := make([]byte, sectionOffset+3*64)
	copy(data[:4], []byte{0x7f, 'E', 'L', 'F'})
	data[4] = byte(elf.ELFCLASS64)
	data[5] = byte(elf.ELFDATA2LSB)
	data[6] = 1
	binary.LittleEndian.PutUint16(data[16:18], uint16(elf.ET_EXEC))
	binary.LittleEndian.PutUint16(data[18:20], uint16(elf.EM_X86_64))
	binary.LittleEndian.PutUint32(data[20:24], 1)
	binary.LittleEndian.PutUint64(data[40:48], sectionOffset)
	binary.LittleEndian.PutUint16(data[52:54], 64)
	binary.LittleEndian.PutUint16(data[58:60], 64)
	binary.LittleEndian.PutUint16(data[60:62], 3)
	binary.LittleEndian.PutUint16(data[62:64], 2)
	data[0x100] = 0xc3
	copy(data[0x101:], strings)
	writeSectionHeader := func(offset int, name uint32, typ uint32, dataOffset uint64, size uint64) {
		binary.LittleEndian.PutUint32(data[offset:offset+4], name)
		binary.LittleEndian.PutUint32(data[offset+4:offset+8], typ)
		binary.LittleEndian.PutUint64(data[offset+24:offset+32], dataOffset)
		binary.LittleEndian.PutUint64(data[offset+32:offset+40], size)
		binary.LittleEndian.PutUint64(data[offset+48:offset+56], 1)
	}
	writeSectionHeader(sectionOffset+64, 1, uint32(elf.SHT_PROGBITS), 0x100, 1)
	writeSectionHeader(sectionOffset+128, 7, uint32(elf.SHT_STRTAB), 0x101, uint64(len(strings)))
	return data
}
