package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	HybridPlatformIOS     = "ios"
	HybridPlatformAndroid = "android"

	HybridPackKindIOS     = "hybrid_ecdh_ios_placeholder"
	HybridPackKindAndroid = "hybrid_ecdh_android_placeholder"
)

var ErrUnsupportedHybridPlatform = errors.New("protocol: unsupported hybrid platform")

// HybridRequest 描述一次 mock-first Hybrid ECDH 封包请求。
// 它不是最终微信真实协议结构，只用于固定可替换的协议接缝。
type HybridRequest struct {
	Platform  string `json:"platform,omitempty"`
	Operation string `json:"operation"`
	Payload   []byte `json:"payload"`
	Flags     uint8  `json:"flags"`
	DeviceID  string `json:"device_id,omitempty"`
	LoginKind string `json:"login_kind,omitempty"`
}

// HybridPacket 保存 Hybrid ECDH mock 封包后的安全摘要。
type HybridPacket struct {
	Platform      string     `json:"platform"`
	PackKind      string     `json:"pack_kind"`
	Operation     string     `json:"operation"`
	PayloadSHA256 string     `json:"payload_sha256"`
	PayloadLength int        `json:"payload_length"`
	PackedHex     string     `json:"packed_hex"`
	Debug         FrameDebug `json:"debug"`
}

// HybridECDHPack 按平台分发到 iOS / Android Hybrid ECDH mock 封包接口。
func HybridECDHPack(request HybridRequest) (HybridPacket, BusinessPacket, error) {
	switch strings.ToLower(strings.TrimSpace(request.Platform)) {
	case HybridPlatformIOS:
		return HybridECDHPackIOS(request)
	case HybridPlatformAndroid:
		return HybridECDHPackAndroid(request)
	default:
		return HybridPacket{}, BusinessPacket{}, fmt.Errorf("%w: %s", ErrUnsupportedHybridPlatform, request.Platform)
	}
}

// HybridECDHPackIOS 使用 iOS Hybrid ECDH 占位类型封装业务包。
func HybridECDHPackIOS(request HybridRequest) (HybridPacket, BusinessPacket, error) {
	return packHybrid(HybridPlatformIOS, HybridPackKindIOS, request)
}

// HybridECDHPackAndroid 使用 Android Hybrid ECDH 占位类型封装业务包。
func HybridECDHPackAndroid(request HybridRequest) (HybridPacket, BusinessPacket, error) {
	return packHybrid(HybridPlatformAndroid, HybridPackKindAndroid, request)
}

func packHybrid(platform string, packKind string, request HybridRequest) (HybridPacket, BusinessPacket, error) {
	packed, debug, err := PackBusinessPacket(BusinessPacket{
		Operation: request.Operation,
		Payload:   request.Payload,
		Flags:     request.Flags,
	})
	if err != nil {
		return HybridPacket{}, BusinessPacket{}, err
	}
	unpacked, unpackDebug, err := UnpackBusinessPacket(packed)
	if err != nil {
		return HybridPacket{}, BusinessPacket{}, err
	}
	if unpackDebug.TotalLength != 0 {
		debug = unpackDebug
	}
	digest := sha256.Sum256(request.Payload)
	return HybridPacket{
		Platform:      platform,
		PackKind:      packKind,
		Operation:     unpacked.Operation,
		PayloadSHA256: hex.EncodeToString(digest[:]),
		PayloadLength: len(request.Payload),
		PackedHex:     ToHex(packed),
		Debug:         debug,
	}, unpacked, nil
}

// WriteHybridSample 把一次 Hybrid ECDH mock 封包输入输出保存为 JSON 样本。
func WriteHybridSample(path string, request HybridRequest, hybrid HybridPacket, unpacked BusinessPacket) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	packed, err := FromHex(hybrid.PackedHex)
	if err != nil {
		return err
	}
	sample := map[string]any{
		"request": hybridRequestSample(request),
		"packed": map[string]any{
			"hex":    hybrid.PackedHex,
			"length": len(packed),
		},
		"unpacked": packetSample(unpacked),
		"debug":    hybrid.Debug,
		"hybrid":   hybrid,
	}
	payload, err := json.MarshalIndent(sample, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(payload, '\n'), 0o644)
}

func hybridRequestSample(request HybridRequest) map[string]any {
	return map[string]any{
		"platform":     request.Platform,
		"operation":    request.Operation,
		"payload_utf8": string(request.Payload),
		"payload_hex":  ToHex(request.Payload),
		"flags":        request.Flags,
		"device_id":    request.DeviceID,
		"login_kind":   request.LoginKind,
	}
}
