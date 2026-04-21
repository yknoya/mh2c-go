package wire

import (
	"encoding/binary"
	"fmt"
)

const FrameHeaderLength = 9

func AppendUint24(dst []byte, v uint32) ([]byte, error) {
	if v > 0x00ff_ffff {
		return dst, fmt.Errorf("uint24 overflow: %d", v)
	}
	return append(dst, byte(v>>16), byte(v>>8), byte(v)), nil
}

func ReadUint24(src []byte) (uint32, error) {
	if len(src) < 3 {
		return 0, fmt.Errorf("uint24 requires 3 bytes, got %d", len(src))
	}
	return uint32(src[0])<<16 | uint32(src[1])<<8 | uint32(src[2]), nil
}

func AppendUint32(dst []byte, v uint32) []byte {
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], v)
	return append(dst, buf[:]...)
}

func ReadUint32(src []byte) (uint32, error) {
	if len(src) < 4 {
		return 0, fmt.Errorf("uint32 requires 4 bytes, got %d", len(src))
	}
	return binary.BigEndian.Uint32(src), nil
}
