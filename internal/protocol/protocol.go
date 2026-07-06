package protocol

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
)

const (
	// HeaderLength 是 mock-first 协议帧头长度：magic(4) + version(1) + flags(1) + opLen(2) + payloadLen(4) + crc32(4)。
	HeaderLength = 16
	frameMagic   = "CBS1"
	frameVersion = uint8(1)
)

var (
	ErrBadMagic       = errors.New("protocol: bad magic")
	ErrFrameTooShort  = errors.New("protocol: frame too short")
	ErrFrameLength    = errors.New("protocol: frame length mismatch")
	ErrCRC32Mismatch  = errors.New("protocol: crc32 mismatch")
	ErrEmptyOperation = errors.New("protocol: empty operation")
)

// BusinessPacket 是第一版 mock 协议边界。它不表示最终微信真实协议，
// 只用于固定 Pack/Unpack 的可测试接缝，后续真实协议还原应复用同一层接口。
type BusinessPacket struct {
	Operation string
	Payload   []byte
	Flags     uint8
}

// FrameDebug 保存封包/解包时可安全输出的结构化摘要。
type FrameDebug struct {
	Magic           string `json:"magic"`
	Version         uint8  `json:"version"`
	Flags           uint8  `json:"flags"`
	HeaderLength    int    `json:"header_length"`
	OperationLength int    `json:"operation_length"`
	PayloadLength   int    `json:"payload_length"`
	TotalLength     int    `json:"total_length"`
	CRC32Hex        string `json:"crc32_hex"`
}

// PackBusinessPacket 把业务操作和 payload 打成稳定 mock 帧。
func PackBusinessPacket(packet BusinessPacket) ([]byte, FrameDebug, error) {
	operation := strings.TrimSpace(packet.Operation)
	if operation == "" {
		return nil, FrameDebug{}, ErrEmptyOperation
	}
	operationBytes := []byte(operation)
	if len(operationBytes) > 0xffff {
		return nil, FrameDebug{}, fmt.Errorf("protocol: operation too long: %d", len(operationBytes))
	}
	payloadLength := len(packet.Payload)
	frameLength := HeaderLength + len(operationBytes) + payloadLength
	frame := make([]byte, frameLength)
	copy(frame[0:4], []byte(frameMagic))
	frame[4] = frameVersion
	frame[5] = packet.Flags
	binary.BigEndian.PutUint16(frame[6:8], uint16(len(operationBytes)))
	binary.BigEndian.PutUint32(frame[8:12], uint32(payloadLength))
	checksum := crc32.ChecksumIEEE(packet.Payload)
	binary.BigEndian.PutUint32(frame[12:16], checksum)
	copy(frame[HeaderLength:HeaderLength+len(operationBytes)], operationBytes)
	copy(frame[HeaderLength+len(operationBytes):], packet.Payload)
	return frame, debugFor(packet.Flags, len(operationBytes), payloadLength, frameLength, checksum), nil
}

// UnpackBusinessPacket 校验 mock 帧并还原业务操作和 payload。
func UnpackBusinessPacket(frame []byte) (BusinessPacket, FrameDebug, error) {
	if len(frame) < HeaderLength {
		return BusinessPacket{}, FrameDebug{}, ErrFrameTooShort
	}
	if string(frame[0:4]) != frameMagic {
		return BusinessPacket{}, FrameDebug{}, ErrBadMagic
	}
	version := frame[4]
	flags := frame[5]
	operationLength := int(binary.BigEndian.Uint16(frame[6:8]))
	payloadLength := int(binary.BigEndian.Uint32(frame[8:12]))
	wantLength := HeaderLength + operationLength + payloadLength
	if len(frame) != wantLength {
		return BusinessPacket{}, FrameDebug{}, fmt.Errorf("%w: got %d want %d", ErrFrameLength, len(frame), wantLength)
	}
	checksum := binary.BigEndian.Uint32(frame[12:16])
	operationStart := HeaderLength
	operationEnd := operationStart + operationLength
	payloadStart := operationEnd
	payload := append([]byte(nil), frame[payloadStart:]...)
	if got := crc32.ChecksumIEEE(payload); got != checksum {
		return BusinessPacket{}, debugFor(flags, operationLength, payloadLength, len(frame), checksum), fmt.Errorf("%w: got %08x want %08x", ErrCRC32Mismatch, got, checksum)
	}
	packet := BusinessPacket{
		Operation: string(frame[operationStart:operationEnd]),
		Payload:   payload,
		Flags:     flags,
	}
	debug := debugFor(flags, operationLength, payloadLength, len(frame), checksum)
	debug.Version = version
	return packet, debug, nil
}

func debugFor(flags uint8, operationLength int, payloadLength int, totalLength int, checksum uint32) FrameDebug {
	return FrameDebug{
		Magic:           frameMagic,
		Version:         frameVersion,
		Flags:           flags,
		HeaderLength:    HeaderLength,
		OperationLength: operationLength,
		PayloadLength:   payloadLength,
		TotalLength:     totalLength,
		CRC32Hex:        fmt.Sprintf("%08x", checksum),
	}
}

func ToHex(payload []byte) string {
	return hex.EncodeToString(payload)
}

func FromHex(value string) ([]byte, error) {
	compact := strings.Join(strings.Fields(value), "")
	return hex.DecodeString(compact)
}

// WritePacketSample 把一次 Pack/Unpack 的输入输出保存为 JSON 样本。
func WritePacketSample(path string, request BusinessPacket, packed []byte, unpacked BusinessPacket, debug FrameDebug) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	sample := map[string]any{
		"request": packetSample(request),
		"packed": map[string]any{
			"hex":    ToHex(packed),
			"length": len(packed),
		},
		"unpacked": packetSample(unpacked),
		"debug":    debug,
	}
	payload, err := json.MarshalIndent(sample, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(payload, '\n'), 0o644)
}

func packetSample(packet BusinessPacket) map[string]any {
	return map[string]any{
		"operation":    packet.Operation,
		"payload_utf8": string(packet.Payload),
		"payload_hex":  ToHex(packet.Payload),
		"flags":        packet.Flags,
	}
}
