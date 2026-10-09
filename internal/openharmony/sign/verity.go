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
	"encoding/binary"
)

const (
	fsVerityHashAlgorithmSHA256 = 1
	fsVerityBlockLog            = 12
	selfSignFlag                = 1 << 4
	codeSignVersion             = 3
	descriptorSize              = 256
	signatureSize               = sha256.Size
)

func merkleRoot(data []byte, codeOffset int) []byte {
	leafCount := (len(data) + pageSize - 1) / pageSize
	if leafCount == 0 {
		return nil
	}
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
		if start/pageSize == codeOffset/pageSize {
			for offset := 0; offset < sha256.Size; offset++ {
				leaves[index*sha256.Size+offset] = 0
			}
		} else {
			copy(leaves[index*sha256.Size:], digest[:])
		}
	}
	if len(data) <= pageSize {
		return append([]byte(nil), leaves[:sha256.Size]...)
	}
	level := leaves
	for {
		pageCount := (len(level) + pageSize - 1) / pageSize
		next := make([]byte, pageCount*sha256.Size)
		for index := 0; index < pageCount; index++ {
			var page [pageSize]byte
			start := index * pageSize
			end := start + pageSize
			if end > len(level) {
				end = len(level)
			}
			copy(page[:], level[start:end])
			digest := sha256.Sum256(page[:])
			copy(next[index*sha256.Size:], digest[:])
		}
		if len(level) <= pageSize {
			var rootPage [pageSize]byte
			copy(rootPage[:], level)
			root := sha256.Sum256(rootPage[:])
			return root[:]
		}
		level = next
	}
}

func descriptor(fileSize int, root []byte, signSize uint32) []byte {
	result := make([]byte, descriptorSize)
	result[0] = 1
	result[1] = fsVerityHashAlgorithmSHA256
	result[2] = fsVerityBlockLog
	binary.LittleEndian.PutUint32(result[4:8], signSize)
	binary.LittleEndian.PutUint64(result[8:16], uint64(fileSize))
	copy(result[16:16+sha256.Size], root)
	binary.LittleEndian.PutUint32(result[112:116], selfSignFlag)
	result[255] = codeSignVersion
	return result
}

func codeSignBlock(fileSize int, root []byte) []byte {
	unsignedDescriptor := descriptor(fileSize, root, 0)
	signature := sha256.Sum256(unsignedDescriptor)
	signedDescriptor := descriptor(fileSize, root, signatureSize)
	block := make([]byte, pageSize)
	binary.LittleEndian.PutUint32(block[0:4], 1)
	binary.LittleEndian.PutUint32(block[4:8], descriptorSize+signatureSize)
	copy(block[8:8+descriptorSize], signedDescriptor)
	copy(block[8+descriptorSize:], signature[:])
	return block
}
