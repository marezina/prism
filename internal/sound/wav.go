package sound

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

// pcm is decoded 16-bit little-endian PCM audio.
type pcm struct {
	data       []byte
	sampleRate int
	channels   int
}

// decodeWAV parses a RIFF/WAVE file holding 16-bit integer PCM, which is the
// only format PRism ships (assets/README.md).
func decodeWAV(b []byte) (pcm, error) {
	if len(b) < 12 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return pcm{}, errors.New("not a RIFF/WAVE file")
	}
	var (
		out    pcm
		gotFmt bool
	)
	for off := 12; off+8 <= len(b); {
		id := string(b[off : off+4])
		size := int(binary.LittleEndian.Uint32(b[off+4 : off+8]))
		body := off + 8
		if body+size > len(b) {
			return pcm{}, fmt.Errorf("chunk %q overruns file", id)
		}
		switch id {
		case "fmt ":
			if size < 16 {
				return pcm{}, errors.New("short fmt chunk")
			}
			format := binary.LittleEndian.Uint16(b[body:])
			bits := binary.LittleEndian.Uint16(b[body+14:])
			if format != 1 || bits != 16 {
				return pcm{}, fmt.Errorf("unsupported WAV format %d / %d bits (want PCM 16-bit)", format, bits)
			}
			out.channels = int(binary.LittleEndian.Uint16(b[body+2:]))
			out.sampleRate = int(binary.LittleEndian.Uint32(b[body+4:]))
			gotFmt = true
		case "data":
			if !gotFmt {
				return pcm{}, errors.New("data chunk before fmt chunk")
			}
			out.data = b[body : body+size]
			return out, nil
		}
		off = body + size + size%2 // chunks are word-aligned
	}
	return pcm{}, errors.New("no data chunk")
}

// scaled returns a copy of p with every sample multiplied by volume (0–1).
func (p pcm) scaled(volume float64) []byte {
	volume = math.Max(0, math.Min(1, volume))
	out := make([]byte, len(p.data))
	for i := 0; i+1 < len(p.data); i += 2 {
		s := int16(binary.LittleEndian.Uint16(p.data[i:]))
		binary.LittleEndian.PutUint16(out[i:], uint16(int16(math.Round(float64(s)*volume))))
	}
	return out
}
