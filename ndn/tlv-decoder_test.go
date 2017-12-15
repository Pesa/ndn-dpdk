package ndn

import (
	"testing"
)

func TestReadVarNum(t *testing.T) {
	assert, require := makeAR(t)

	tests := []struct {
		input  string
		ok     bool
		output uint64
	}{
		{"", false, 0},
		{"00", true, 0x00},
		{"FC", true, 0xFC},
		{"FD", false, 0},
		{"FD 00", false, 0},
		{"FD 01 00", true, 0x0100},
		{"FD FF FF", true, 0xFFFF},
		{"FE 00 00 00", false, 0},
		{"FE 01 00 00 00", true, 0x01000000},
		{"FE FF FF FF FF", true, 0xFFFFFFFF},
		{"FF 00 00 00 00 00 00 00", false, 0},
		{"FF 01 00 00 00 00 00 00 00", true, 0x0100000000000000},
		{"FF FF FF FF FF FF FF FF FF", true, 0xFFFFFFFFFFFFFFFF},
	}
	for _, tt := range tests {
		pkt := packetFromHex(tt.input)
		require.True(pkt.IsValid(), tt.input)
		defer pkt.Close()
		d := NewTlvDecoder(pkt)

		v, e := d.ReadVarNum()
		if tt.ok {
			if assert.NoError(e, tt.input) {
				assert.Equal(tt.output, v, tt.input)
			}
		} else {
			assert.Error(e, tt.input)
		}
	}
}