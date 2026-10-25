package sound

import (
	"bytes"
	"fmt"
	"sync"

	"github.com/ebitengine/oto/v3"
)

// OtoPlayer plays sounds through ebitengine/oto (WASAPI on Windows, ALSA on
// Linux). The audio device is opened on the first Play.
type OtoPlayer struct {
	sounds map[string]pcm

	once   sync.Once
	ctx    *oto.Context
	ctxErr error
}

// NewOtoPlayer decodes the embedded sounds. All of them must share one sample
// rate and channel count, because oto opens the device with a fixed format.
func NewOtoPlayer() (*OtoPlayer, error) {
	sounds, err := load()
	if err != nil {
		return nil, err
	}
	var first *pcm
	for name, p := range sounds {
		if first == nil {
			first = &p
			continue
		}
		if p.sampleRate != first.sampleRate || p.channels != first.channels {
			return nil, fmt.Errorf("sound %s: %d Hz × %d ch differs from %d Hz × %d ch",
				name, p.sampleRate, p.channels, first.sampleRate, first.channels)
		}
	}
	return &OtoPlayer{sounds: sounds}, nil
}

// Play starts playback and returns immediately.
func (o *OtoPlayer) Play(name string, volume float64) error {
	p, ok := o.sounds[name]
	if !ok {
		return fmt.Errorf("unknown sound %q", name)
	}
	if err := o.open(p); err != nil {
		return err
	}
	// oto keeps a playing player alive and releases it when it finishes.
	o.ctx.NewPlayer(bytes.NewReader(p.scaled(volume))).Play()
	return nil
}

func (o *OtoPlayer) open(p pcm) error {
	o.once.Do(func() {
		ctx, ready, err := oto.NewContext(&oto.NewContextOptions{
			SampleRate:   p.sampleRate,
			ChannelCount: p.channels,
			Format:       oto.FormatSignedInt16LE,
		})
		if err != nil {
			o.ctxErr = fmt.Errorf("open audio device: %w", err)
			return
		}
		<-ready
		o.ctx = ctx
	})
	return o.ctxErr
}
