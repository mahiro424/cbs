package protocol_test

import (
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/mahiro424/cbs/internal/protocol"
)

func TestHybridECDHPackIOSBuildsStableMockFrame(t *testing.T) {
	request := protocol.HybridRequest{
		Operation: "Login.GetQR",
		Payload:   []byte(`{"device_id":"ios-001","scene":"qr"}`),
		Flags:     5,
		DeviceID:  "ios-001",
		LoginKind: "qr",
	}

	hybrid, unpacked, err := protocol.HybridECDHPackIOS(request)
	if err != nil {
		t.Fatalf("HybridECDHPackIOS 返回错误：%v", err)
	}

	const wantHex = "434253310105000b0000002486339f3b4c6f67696e2e47657451527b226465766963655f6964223a22696f732d303031222c227363656e65223a227172227d"
	const wantSHA256 = "539b43b7995fe55792da263d50ce96db6ecc525b365b08af97f85b0ac9b28a57"
	if hybrid.Platform != "ios" || hybrid.PackKind != "hybrid_ecdh_ios_placeholder" {
		t.Fatalf("hybrid 平台/类型 = %+v，期望 iOS mock 占位", hybrid)
	}
	if hybrid.Operation != request.Operation || hybrid.PayloadSHA256 != wantSHA256 || hybrid.PackedHex != wantHex {
		t.Fatalf("hybrid 摘要 = %+v，期望稳定 operation/sha256/hex", hybrid)
	}
	if hybrid.Debug.PayloadLength != len(request.Payload) || hybrid.Debug.CRC32Hex != "86339f3b" {
		t.Fatalf("hybrid debug = %+v，期望记录 payload 长度和 CRC", hybrid.Debug)
	}
	frame, err := protocol.FromHex(hybrid.PackedHex)
	if err != nil {
		t.Fatalf("hybrid packed hex 解码失败：%v", err)
	}
	fromHex, _, err := protocol.UnpackBusinessPacket(frame)
	if err != nil {
		t.Fatalf("hybrid packed hex 解包失败：%v", err)
	}
	if fromHex.Operation != request.Operation || string(fromHex.Payload) != string(request.Payload) || fromHex.Flags != request.Flags {
		t.Fatalf("hybrid hex 往返 = %+v，期望还原请求", fromHex)
	}
	if unpacked.Operation != request.Operation || string(unpacked.Payload) != string(request.Payload) || unpacked.Flags != request.Flags {
		t.Fatalf("unpacked = %+v，期望还原 Hybrid 请求", unpacked)
	}
}

func TestHybridECDHPackAndroidBuildsStableMockFrame(t *testing.T) {
	request := protocol.HybridRequest{
		Operation: "Login.A16Data",
		Payload:   []byte(`{"a16":"mock-a16","device_id":"android-001"}`),
		Flags:     7,
		DeviceID:  "android-001",
		LoginKind: "a16",
	}

	hybrid, unpacked, err := protocol.HybridECDHPackAndroid(request)
	if err != nil {
		t.Fatalf("HybridECDHPackAndroid 返回错误：%v", err)
	}

	const wantHex = "434253310107000d0000002cfcfd91864c6f67696e2e413136446174617b22613136223a226d6f636b2d613136222c226465766963655f6964223a22616e64726f69642d303031227d"
	const wantSHA256 = "6d97c5c22844f10a6e2ebde756c29a3825f52eee459072b37980ea8f1c68af96"
	if hybrid.Platform != "android" || hybrid.PackKind != "hybrid_ecdh_android_placeholder" {
		t.Fatalf("hybrid 平台/类型 = %+v，期望 Android mock 占位", hybrid)
	}
	if hybrid.Operation != request.Operation || hybrid.PayloadSHA256 != wantSHA256 || hybrid.PackedHex != wantHex {
		t.Fatalf("hybrid 摘要 = %+v，期望稳定 operation/sha256/hex", hybrid)
	}
	if hybrid.Debug.PayloadLength != len(request.Payload) || hybrid.Debug.CRC32Hex != "fcfd9186" {
		t.Fatalf("hybrid debug = %+v，期望记录 payload 长度和 CRC", hybrid.Debug)
	}
	if unpacked.Operation != request.Operation || string(unpacked.Payload) != string(request.Payload) || unpacked.Flags != request.Flags {
		t.Fatalf("unpacked = %+v，期望还原 Android Hybrid 请求", unpacked)
	}
}

func TestHybridECDHPackRejectsUnsupportedPlatformAndEmptyOperation(t *testing.T) {
	_, _, err := protocol.HybridECDHPack(protocol.HybridRequest{
		Platform:  "windows-phone",
		Operation: "Login.GetQR",
		Payload:   []byte("{}"),
	})
	if !errors.Is(err, protocol.ErrUnsupportedHybridPlatform) {
		t.Fatalf("不支持平台错误 = %v，期望 ErrUnsupportedHybridPlatform", err)
	}

	_, _, err = protocol.HybridECDHPackIOS(protocol.HybridRequest{
		Payload: []byte("{}"),
	})
	if !errors.Is(err, protocol.ErrEmptyOperation) {
		t.Fatalf("空 operation 错误 = %v，期望 ErrEmptyOperation", err)
	}
}

func TestHybridECDHSampleFileContainsRequestPackedUnpackedDebugAndHybrid(t *testing.T) {
	request := protocol.HybridRequest{
		Platform:  "ios",
		Operation: "Login.GetQR",
		Payload:   []byte(`{"device_id":"ios-sample"}`),
		Flags:     2,
		DeviceID:  "ios-sample",
		LoginKind: "qr",
	}
	hybrid, unpacked, err := protocol.HybridECDHPack(request)
	if err != nil {
		t.Fatalf("HybridECDHPack 返回错误：%v", err)
	}

	samplePath := t.TempDir() + string(os.PathSeparator) + "hybrid-sample.json"
	if err := protocol.WriteHybridSample(samplePath, request, hybrid, unpacked); err != nil {
		t.Fatalf("写 Hybrid 样本失败：%v", err)
	}
	raw, err := os.ReadFile(samplePath)
	if err != nil {
		t.Fatalf("读取 Hybrid 样本失败：%v", err)
	}
	var sample map[string]any
	if err := json.Unmarshal(raw, &sample); err != nil {
		t.Fatalf("Hybrid 样本不是 JSON：%v", err)
	}
	for _, key := range []string{"request", "packed", "unpacked", "debug", "hybrid"} {
		if _, ok := sample[key]; !ok {
			t.Fatalf("Hybrid 样本缺少字段 %s：%+v", key, sample)
		}
	}
	hybridSample, ok := sample["hybrid"].(map[string]any)
	if !ok || hybridSample["pack_kind"] != "hybrid_ecdh_ios_placeholder" || hybridSample["packed_hex"] != hybrid.PackedHex {
		t.Fatalf("hybrid 样本 = %+v，期望记录 pack_kind 和 packed_hex", sample["hybrid"])
	}
	packedSample, ok := sample["packed"].(map[string]any)
	if !ok || packedSample["hex"] != hybrid.PackedHex {
		t.Fatalf("packed 样本 = %+v，期望复用 Hybrid packed hex", sample["packed"])
	}
}
