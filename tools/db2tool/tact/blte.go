// Go translation of TACTSharp's BLTE decoder (https://github.com/wowdev/TACTSharp,
// v0.0.13-alpha, commit d0ab516eb98b5db35682467b6e4977d88955046d).
// Copyright (c) 2024 Martin Benjamins. MIT License — see tools/db2tool/NOTICES.md.
//
// Keyless: no TACT keys are loaded, so 'E' (encrypted) chunks are left
// zero-filled in the output — exactly what the WDC layer's encrypted-section
// skip expects. 'F' never occurs.

package tact

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
)

// blteDecode decodes a BLTE blob. totalDecompSize may be 0 (computed from
// chunk infos for multi-chunk files; required for single-chunk non-'N').
func blteDecode(data []byte, totalDecompSize uint64) ([]byte, error) {
	if len(data) < 9 || data[0] != 'B' || data[1] != 'L' || data[2] != 'T' || data[3] != 'E' {
		return nil, fmt.Errorf("invalid BLTE header")
	}
	const fixedHeaderSize = 8
	headerSize := int(be32(data[4:]))
	const maxDecodedBytes = uint64(maxCASCFileBytes)

	if headerSize == 0 {
		mode := data[fixedHeaderSize]
		if mode != 'N' && totalDecompSize == 0 {
			return nil, fmt.Errorf("totalDecompSize must be set for single non-normal BLTE block")
		}
		if mode == 'N' && totalDecompSize == 0 {
			totalDecompSize = uint64(len(data) - fixedHeaderSize - 1)
		}
		if totalDecompSize > maxDecodedBytes {
			return nil, fmt.Errorf("decoded BLTE block exceeds %d-byte limit", maxDecodedBytes)
		}
		out := make([]byte, totalDecompSize)
		if err := handleDataBlock(mode, data[fixedHeaderSize+1:], out); err != nil {
			return nil, err
		}
		return out, nil
	}

	if headerSize < fixedHeaderSize+4 || headerSize > len(data) {
		return nil, fmt.Errorf("BLTE chunk table bounds %d-byte header against %d-byte file", headerSize, len(data))
	}
	if data[fixedHeaderSize] != 0xF {
		return nil, fmt.Errorf("unexpected BLTE table format 0x%x", data[fixedHeaderSize])
	}
	chunkCount := int(data[fixedHeaderSize+1])<<16 | int(data[fixedHeaderSize+2])<<8 | int(data[fixedHeaderSize+3])
	infoStart := fixedHeaderSize + 4
	const blockInfoSize = 24
	if chunkCount == 0 || chunkCount > (headerSize-infoStart)/blockInfoSize {
		return nil, fmt.Errorf("invalid BLTE chunk count %d for %d-byte header", chunkCount, headerSize)
	}

	if totalDecompSize == 0 {
		o := infoStart + 4
		for range chunkCount {
			if o+4 > headerSize {
				return nil, fmt.Errorf("truncated BLTE chunk table")
			}
			totalDecompSize += uint64(be32(data[o:]))
			o += blockInfoSize
		}
	}
	if totalDecompSize > maxDecodedBytes {
		return nil, fmt.Errorf("decoded BLTE file exceeds %d-byte limit", maxDecodedBytes)
	}

	out := make([]byte, totalDecompSize)
	infoOffset := infoStart
	compOffset := headerSize
	decompOffset := 0

	for chunk := range chunkCount {
		if infoOffset+blockInfoSize > headerSize {
			return nil, fmt.Errorf("BLTE chunk %d metadata exceeds chunk table", chunk)
		}
		compSize := int(be32(data[infoOffset:]))
		decompSize := int(be32(data[infoOffset+4:]))
		if compSize < 1 || decompSize < 0 || compOffset > len(data) || compSize > len(data)-compOffset ||
			decompOffset > len(out) || decompSize > len(out)-decompOffset {
			return nil, fmt.Errorf("BLTE chunk %d out of bounds", chunk)
		}
		if err := handleDataBlock(data[compOffset], data[compOffset+1:compOffset+compSize], out[decompOffset:decompOffset+decompSize]); err != nil {
			return nil, fmt.Errorf("BLTE chunk %d: %w", chunk, err)
		}
		infoOffset += blockInfoSize
		compOffset += compSize
		decompOffset += decompSize
	}
	if decompOffset != len(out) {
		return nil, fmt.Errorf("BLTE chunks decoded %d bytes, expected %d", decompOffset, len(out))
	}
	return out, nil
}

func handleDataBlock(mode byte, compData, out []byte) error {
	switch mode {
	case 'N':
		if len(compData) != len(out) {
			return fmt.Errorf("uncompressed BLTE block has %d bytes, expected %d", len(compData), len(out))
		}
		copy(out, compData)
		return nil
	case 'Z':
		zr, err := zlib.NewReader(bytes.NewReader(compData))
		if err != nil {
			return err
		}
		defer zr.Close()
		if _, err := io.ReadFull(zr, out); err != nil {
			return err
		}
		var extra [1]byte
		n, err := zr.Read(extra[:])
		if n != 0 {
			return fmt.Errorf("decompressed BLTE block exceeds expected output size %d", len(out))
		}
		if err != io.EOF {
			if err == nil {
				return fmt.Errorf("zlib reader did not reach end of block")
			}
			return err
		}
		return nil
	case 'E':
		// Keyless: leave the output range zero-filled.
		return nil
	case 'F':
		return fmt.Errorf("BLTE frame ('F') decompression not implemented (never occurs in this data)")
	default:
		return fmt.Errorf("invalid BLTE chunk mode %q", mode)
	}
}

func be32(b []byte) uint32 {
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}
