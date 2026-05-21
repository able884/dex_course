package cpmm

import (
	"encoding/base64"
	"github.com/davecgh/go-spew/spew"
	"testing"
)

// DecodeBase64 解码 base64 字符串并返回解码后的字节
func DecodeBase64(data string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(data)
}

func TestDecodeBase64(t *testing.T) {
	tests := []struct {
		input       string
		expected    []byte
		shouldError bool
	}{
		{
			input: "QMbN6CYIceKuUsZRbcwdGyBK5IJfxfjPDvA6F/Y1DpE4eV42gTEjRUt3mBAAAAAAEzCZercPAACAhB4AAAAAAC9QHCAcAAAAAAAAAAAAAAAAAAAAAAAAAAE=",
			// input: "QMbN6CYIceIx4rZAspvpc+zUMnjYONM3OePbaDvqmbK2RXpuK2Qa/NteEnr4/MUBWBW85FUAAADw159U2DQAANr49wkAAAAAAAAAAAAAAAAAAAAAAAAAAAE=",
			// input:       "QMbN6CYIceLNI95I/uosBS1r69X/4qQ2JSpgIqRhiHbrCIXUS2v4fkJ4BLufrv8A+JtA7/wDAAB/8LCF8AMAAEi0sA8AAAAAAAAAAAAAAAAAAAAAAAAAAAE=", // https://solscan.io/tx/24c7dwdMREs7WdU6ZaVmdXJgX2MFFrSBiZGcF71SBzLDGAgMZ6vXwVVRwTT2YmB4kjnr3Qb5KAm7eu7TmagMYTtQ
			expected:    []byte{0x6f, 0x69, 0x18, 0x54, 0xf4, 0xe3, 0x08, 0x90, 0x24, 0x93, 0x5a, 0x76, 0x0a, 0x4f, 0xb1, 0xa2, 0x9e, 0x02, 0xa1, 0x09, 0xce, 0xe3, 0x16, 0x34, 0xb3, 0x81, 0x4a, 0x56, 0x51, 0xc4, 0x5f, 0x90, 0xb0, 0x5b, 0x5f, 0xd2, 0xef, 0x09, 0x54, 0x07, 0xe7, 0x8b, 0xfe, 0xea, 0x49, 0xd0, 0xfd},
			shouldError: false,
		},
	}

	for _, test := range tests {
		result, _ := DecodeBase64(test.input)
		spew.Dump(result)
		swapEvent, err := DeserializeSwapEvent(result[8:])
		if err != nil {
			t.Fatal(err)
		}
		spew.Dump(swapEvent)
	}
}
