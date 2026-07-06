package protocol

import (
	"bytes"
	"strings"
)

// PacketSample 是二进制调试工具输出的业务包摘要。
type PacketSample struct {
	Operation   string `json:"operation"`
	PayloadUTF8 string `json:"payload_utf8"`
	PayloadHex  string `json:"payload_hex"`
	Flags       uint8  `json:"flags"`
}

// PacketInspection 保存一次 mock-first 业务帧 inspect 结果。
type PacketInspection struct {
	Status string       `json:"status"`
	Hex    string       `json:"hex"`
	Length int          `json:"length"`
	Packet PacketSample `json:"packet"`
	Debug  FrameDebug   `json:"debug"`
}

// ByteDifference 描述 expected / actual 的首个字节差异。
type ByteDifference struct {
	Offset      int    `json:"offset"`
	ExpectedHex string `json:"expected_hex,omitempty"`
	ActualHex   string `json:"actual_hex,omitempty"`
}

// PacketComparison 保存两段十六进制业务帧的字节级对拍结果。
type PacketComparison struct {
	Status          string            `json:"status"`
	Equal           bool              `json:"equal"`
	SkipReason      string            `json:"skip_reason,omitempty"`
	ExpectedLength  int               `json:"expected_length"`
	ActualLength    int               `json:"actual_length"`
	FirstDifference *ByteDifference   `json:"first_difference,omitempty"`
	Expected        *PacketInspection `json:"expected,omitempty"`
	Actual          *PacketInspection `json:"actual,omitempty"`
}

// InspectBusinessPacketHex 将十六进制 mock-first 业务帧解包为结构化调试摘要。
func InspectBusinessPacketHex(frameHex string) (PacketInspection, error) {
	frame, err := FromHex(frameHex)
	if err != nil {
		return PacketInspection{}, err
	}
	packet, debug, err := UnpackBusinessPacket(frame)
	if err != nil {
		return PacketInspection{}, err
	}
	return PacketInspection{
		Status: "ok",
		Hex:    ToHex(frame),
		Length: len(frame),
		Packet: PacketSample{
			Operation:   packet.Operation,
			PayloadUTF8: string(packet.Payload),
			PayloadHex:  ToHex(packet.Payload),
			Flags:       packet.Flags,
		},
		Debug: debug,
	}, nil
}

// CompareBusinessPacketHex 对 expected / actual 两段十六进制 mock-first 业务帧做字节级对拍。
func CompareBusinessPacketHex(expectedHex string, actualHex string) (PacketComparison, error) {
	expectedHex = strings.TrimSpace(expectedHex)
	actualHex = strings.TrimSpace(actualHex)
	if expectedHex == "" {
		return PacketComparison{Status: "skipped", SkipReason: "expected hex sample is missing"}, nil
	}
	if actualHex == "" {
		return PacketComparison{Status: "skipped", SkipReason: "actual hex sample is missing"}, nil
	}

	expectedBytes, err := FromHex(expectedHex)
	if err != nil {
		return PacketComparison{}, err
	}
	actualBytes, err := FromHex(actualHex)
	if err != nil {
		return PacketComparison{}, err
	}

	expectedInspection, err := InspectBusinessPacketHex(expectedHex)
	if err != nil {
		return PacketComparison{}, err
	}
	actualInspection, err := InspectBusinessPacketHex(actualHex)
	if err != nil {
		return PacketComparison{}, err
	}

	comparison := PacketComparison{
		Status:         "ok",
		Equal:          bytes.Equal(expectedBytes, actualBytes),
		ExpectedLength: len(expectedBytes),
		ActualLength:   len(actualBytes),
		Expected:       &expectedInspection,
		Actual:         &actualInspection,
	}
	if !comparison.Equal {
		comparison.FirstDifference = firstByteDifference(expectedBytes, actualBytes)
	}
	return comparison, nil
}

func firstByteDifference(expected []byte, actual []byte) *ByteDifference {
	limit := len(expected)
	if len(actual) < limit {
		limit = len(actual)
	}
	for i := 0; i < limit; i++ {
		if expected[i] != actual[i] {
			return &ByteDifference{Offset: i, ExpectedHex: ToHex(expected[i : i+1]), ActualHex: ToHex(actual[i : i+1])}
		}
	}
	if len(expected) == len(actual) {
		return nil
	}
	diff := &ByteDifference{Offset: limit}
	if len(expected) > limit {
		diff.ExpectedHex = ToHex(expected[limit : limit+1])
	}
	if len(actual) > limit {
		diff.ActualHex = ToHex(actual[limit : limit+1])
	}
	return diff
}
