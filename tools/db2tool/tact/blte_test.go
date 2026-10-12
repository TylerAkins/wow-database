package tact

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"testing"
)

func TestBLTEDecodeUncompressedSingleBlock(t *testing.T) {
	input := append([]byte("BLTE\x00\x00\x00\x00N"), []byte("client-db2")...)
	got, err := blteDecode(input, 0)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "client-db2" {
		t.Fatalf("decoded %q", got)
	}
	if _, err := blteDecode(input, uint64(len(got)+1)); err == nil {
		t.Fatal("mismatched uncompressed block size should fail")
	}
}

func TestBLTEDecodeZlibChunkTable(t *testing.T) {
	const plain = "quest data"
	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	if _, err := zw.Write([]byte(plain)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	chunk := append([]byte{'Z'}, compressed.Bytes()...)
	const tableEnd = 8 + 4 + 24
	input := make([]byte, tableEnd+len(chunk))
	copy(input, "BLTE")
	binary.BigEndian.PutUint32(input[4:8], tableEnd)
	input[8] = 0x0F
	input[11] = 1
	binary.BigEndian.PutUint32(input[12:16], uint32(len(chunk)))
	binary.BigEndian.PutUint32(input[16:20], uint32(len(plain)))
	copy(input[tableEnd:], chunk)

	got, err := blteDecode(input, 0)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != plain {
		t.Fatalf("decoded %q, want %q", got, plain)
	}
}

func TestBLTERejectsMalformedChunkMetadata(t *testing.T) {
	input := make([]byte, 12)
	copy(input, "BLTE")
	binary.BigEndian.PutUint32(input[4:8], 12)
	input[8] = 0x0F
	if _, err := blteDecode(input, 0); err == nil {
		t.Fatal("empty multi-chunk BLTE should fail")
	}

	if _, err := blteDecode([]byte("BLTE\x00\x00\x00\x00"), 0); err == nil {
		t.Fatal("truncated single-block header should fail")
	}
}

func TestBLTESkipsEncryptedSingleBlockWithoutKeys(t *testing.T) {
	input := []byte("BLTE\x00\x00\x00\x00Eencrypted")
	got, err := blteDecode(input, 4)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte{0, 0, 0, 0}) {
		t.Fatalf("encrypted payload output = %v", got)
	}
}
