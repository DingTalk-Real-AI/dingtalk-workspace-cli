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

package main

import (
	"debug/elf"
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCrossPlatformCoverageBinarySignToolCLI(t *testing.T) {
	directory := t.TempDir()
	inputPath := filepath.Join(directory, "input")
	outputPath := filepath.Join(directory, "output")
	if err := os.WriteFile(inputPath, minimalSignedToolELF(), 0750); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"sign", "-inFile", inputPath, "-outFile", outputPath, "-selfSign", "1"}); err != nil {
		t.Fatal(err)
	}
	output, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(output[:4]) != "\x7fELF" {
		t.Fatal("signed output is not an ELF file")
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(outputPath)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := info.Mode().Perm(), os.FileMode(0750); got != want {
			t.Fatalf("output mode = %o, want %o", got, want)
		}
	}
}

func minimalSignedToolELF() []byte {
	const sectionOffset = 0x200
	sectionNames := []byte("\x00.text\x00.shstrtab\x00")
	data := make([]byte, sectionOffset+3*64)
	copy(data[:4], []byte{0x7f, 'E', 'L', 'F'})
	data[4] = byte(elf.ELFCLASS64)
	data[5] = byte(elf.ELFDATA2LSB)
	data[6] = 1
	binary.LittleEndian.PutUint16(data[16:18], uint16(elf.ET_EXEC))
	binary.LittleEndian.PutUint16(data[18:20], uint16(elf.EM_X86_64))
	binary.LittleEndian.PutUint32(data[20:24], 1)
	binary.LittleEndian.PutUint64(data[40:48], sectionOffset)
	binary.LittleEndian.PutUint16(data[58:60], 64)
	binary.LittleEndian.PutUint16(data[60:62], 3)
	binary.LittleEndian.PutUint16(data[62:64], 2)
	data[0x100] = 0xc3
	copy(data[0x101:], sectionNames)
	writeHeader := func(offset int, name, typ uint32, dataOffset, size uint64) {
		binary.LittleEndian.PutUint32(data[offset:offset+4], name)
		binary.LittleEndian.PutUint32(data[offset+4:offset+8], typ)
		binary.LittleEndian.PutUint64(data[offset+24:offset+32], dataOffset)
		binary.LittleEndian.PutUint64(data[offset+32:offset+40], size)
		binary.LittleEndian.PutUint64(data[offset+48:offset+56], 1)
	}
	writeHeader(sectionOffset+64, 1, uint32(elf.SHT_PROGBITS), 0x100, 1)
	writeHeader(sectionOffset+128, 7, uint32(elf.SHT_STRTAB), 0x101, uint64(len(sectionNames)))
	return data
}
