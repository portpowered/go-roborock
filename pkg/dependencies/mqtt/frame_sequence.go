package mqtt

import (
	"math/rand/v2"
	"sync/atomic"

	"github.com/portpowered/go-roborock/internal/protocol"
)

// Match the reference V1 process-wide frame counter independently of RPC IDs
// and native remote-control seqnum. This does not establish native replay policy.
// https://github.com/Python-roborock/python-roborock/blob/main/roborock/roborock_message.py
const (
	v1FrameSequenceMinimum = 100000
	v1FrameSequenceMaximum = 999999
	v1FrameSequenceCount   = v1FrameSequenceMaximum - v1FrameSequenceMinimum + 1
)

type frameSequence struct {
	cursor atomic.Uint32
}

var v1Frames = newFrameSequence() //nolint:gochecknoglobals // One V1 frame namespace across independent MQTT sessions.

func newFrameSequence() *frameSequence {
	sequence := &frameSequence{cursor: atomic.Uint32{}}
	// This initial offset reduces restart collisions, not a security boundary.
	sequence.cursor.Store(uint32(v1FrameSequenceMinimum + rand.IntN(v1FrameSequenceCount))) //nolint:gosec

	return sequence
}

func (sequence *frameSequence) next() uint32 {
	for {
		previous := sequence.cursor.Load()

		next := previous + 1
		if next > v1FrameSequenceMaximum {
			next = v1FrameSequenceMinimum
		}

		if sequence.cursor.CompareAndSwap(previous, next) {
			return next
		}
	}
}

func (s *Session) nextFrameSequence() uint32 {
	if s.config.Protocol == protocol.MQTTVersionV1 {
		return v1Frames.next()
	}

	return s.sequence.Add(1)
}
