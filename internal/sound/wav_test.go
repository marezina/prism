package sound

import (
	"encoding/binary"
	"slices"
	"testing"
)

func TestEmbeddedSounds(t *testing.T) {
	want := []string{"alert", "ding", "pop", "soft-chime"}
	if got := Names(); !slices.Equal(got, want) {
		t.Fatalf("Names() = %v, want %v", got, want)
	}
	sounds, err := load()
	if err != nil {
		t.Fatal(err)
	}
	for name, p := range sounds {
		if p.sampleRate != 44100 || p.channels != 1 {
			t.Errorf("%s: %d Hz × %d ch, want 44100 Hz mono", name, p.sampleRate, p.channels)
		}
		if secs := float64(len(p.data)) / 2 / float64(p.sampleRate); secs <= 0 || secs > 1.01 {
			t.Errorf("%s: duration %.2fs, want (0, 1]", name, secs)
		}
	}
}

func TestDecodeWAVErrors(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
	}{
		{"empty", nil},
		{"not riff", []byte("RIFX\x00\x00\x00\x00WAVE")},
		{"no data", wavFile(t, 1, 16, nil)[:36]},
		{"float format", wavFile(t, 3, 32, []byte{0, 0, 0, 0})},
		{"8-bit", wavFile(t, 1, 8, []byte{0})},
	}
	for _, tt := range tests {
		if _, err := decodeWAV(tt.in); err == nil {
			t.Errorf("%s: want error", tt.name)
		}
	}
}

func TestScaled(t *testing.T) {
	samples := []int16{0, 1000, -1000, 32767, -32768}
	data := make([]byte, 2*len(samples))
	for i, s := range samples {
		binary.LittleEndian.PutUint16(data[2*i:], uint16(s))
	}
	p, err := decodeWAV(wavFile(t, 1, 16, data))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		volume float64
		want   []int16
	}{
		{1, samples},
		{0.5, []int16{0, 500, -500, 16384, -16384}},
		{0, []int16{0, 0, 0, 0, 0}},
		{2, samples}, // clamped to 1
		{-1, []int16{0, 0, 0, 0, 0}},
	}
	for _, tt := range tests {
		out := p.scaled(tt.volume)
		for i, w := range tt.want {
			if got := int16(binary.LittleEndian.Uint16(out[2*i:])); got != w {
				t.Errorf("volume %v sample %d: got %d, want %d", tt.volume, i, got, w)
			}
		}
	}
}

// wavFile builds a minimal mono 44.1 kHz WAV with the given format and data.
func wavFile(t *testing.T, format, bits uint16, data []byte) []byte {
	t.Helper()
	b := []byte("RIFF\x00\x00\x00\x00WAVEfmt ")
	b = binary.LittleEndian.AppendUint32(b, 16)
	b = binary.LittleEndian.AppendUint16(b, format)
	b = binary.LittleEndian.AppendUint16(b, 1)
	b = binary.LittleEndian.AppendUint32(b, 44100)
	b = binary.LittleEndian.AppendUint32(b, 44100*uint32(bits)/8)
	b = binary.LittleEndian.AppendUint16(b, bits/8)
	b = binary.LittleEndian.AppendUint16(b, bits)
	b = append(b, "data"...)
	b = binary.LittleEndian.AppendUint32(b, uint32(len(data)))
	return append(b, data...)
}
