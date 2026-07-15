package core

import (
	"encoding/binary"
	"fmt"
)

const (
	frameMagic   = "CHLM"
	frameVersion = 1
)

// EncodeFrame turns a normalized payload into a protocol-safe binary frame.
func EncodeFrame(profile BehaviorProfile, payload []byte, originalLen int) ([]byte, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("payload must not be empty")
	}
	if originalLen <= 0 || originalLen > len(payload) {
		return nil, fmt.Errorf("original payload length must be positive and not exceed normalized payload")
	}

	profileID := uint16(0)
	switch profile {
	case ProfileWebRTC:
		profileID = 1
	case ProfileHTTP3:
		profileID = 2
	case ProfileGaming:
		profileID = 3
	default:
		profileID = 0
	}

	frame := make([]byte, 16+len(payload))
	copy(frame[:4], frameMagic)
	frame[4] = byte(frameVersion)
	frame[5] = 0
	binary.BigEndian.PutUint16(frame[6:8], profileID)
	binary.BigEndian.PutUint32(frame[8:12], uint32(originalLen))
	binary.BigEndian.PutUint32(frame[12:16], uint32(len(payload)))
	copy(frame[16:], payload)

	return frame, nil
}

// DecodeFrame validates and extracts the original payload from a binary frame.
func DecodeFrame(frame []byte) ([]byte, error) {
	if len(frame) < 16 {
		return nil, fmt.Errorf("frame too short")
	}
	if string(frame[:4]) != frameMagic {
		return nil, fmt.Errorf("invalid magic header")
	}
	if frame[4] != byte(frameVersion) {
		return nil, fmt.Errorf("unsupported frame version")
	}

	originalLen := int(binary.BigEndian.Uint32(frame[8:12]))
	normalizedLen := int(binary.BigEndian.Uint32(frame[12:16]))
	if originalLen <= 0 || originalLen > normalizedLen || normalizedLen > len(frame)-16 {
		return nil, fmt.Errorf("invalid payload length")
	}

	payload := append([]byte(nil), frame[16:16+normalizedLen]...)
	return payload[:originalLen], nil
}
