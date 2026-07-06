package protocol_test

import (
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/mahiro424/cbs/internal/protocol"
)

func TestPackBusinessPacketProducesStableMockFrameAndUnpacks(t *testing.T) {
	packet := protocol.BusinessPacket{
		Operation: "Msg.SendTxt",
		Payload:   []byte("hello mock protocol"),
		Flags:     3,
	}

	packed, debug, err := protocol.PackBusinessPacket(packet)
	if err != nil {
		t.Fatalf("PackBusinessPacket 返回错误：%v", err)
	}
	const wantHex = "434253310103000b00000013c463dfb64d73672e53656e6454787468656c6c6f206d6f636b2070726f746f636f6c"
	if got := protocol.ToHex(packed); got != wantHex {
		t.Fatalf("封包 hex = %s，期望 %s", got, wantHex)
	}
	if debug.Magic != "CBS1" || debug.Version != 1 || debug.OperationLength != len(packet.Operation) || debug.PayloadLength != len(packet.Payload) || debug.CRC32Hex != "c463dfb6" {
		t.Fatalf("debug = %+v，期望记录 magic/version/长度/crc", debug)
	}

	unpacked, unpackDebug, err := protocol.UnpackBusinessPacket(packed)
	if err != nil {
		t.Fatalf("UnpackBusinessPacket 返回错误：%v", err)
	}
	if unpacked.Operation != packet.Operation || string(unpacked.Payload) != string(packet.Payload) || unpacked.Flags != packet.Flags {
		t.Fatalf("unpacked = %+v，期望还原原始 packet", unpacked)
	}
	if unpackDebug.CRC32Hex != debug.CRC32Hex || unpackDebug.TotalLength != len(packed) {
		t.Fatalf("unpack debug = %+v，期望 crc 和总长度匹配", unpackDebug)
	}
}

func TestBusinessPacketHexRoundTripAndSampleFile(t *testing.T) {
	packet := protocol.BusinessPacket{Operation: "Login.Newinit", Payload: []byte(`{"wxid":"wxid_sample"}`), Flags: 1}
	packed, debug, err := protocol.PackBusinessPacket(packet)
	if err != nil {
		t.Fatalf("封包失败：%v", err)
	}
	fromHex, err := protocol.FromHex(protocol.ToHex(packed))
	if err != nil {
		t.Fatalf("hex 解码失败：%v", err)
	}
	unpacked, _, err := protocol.UnpackBusinessPacket(fromHex)
	if err != nil {
		t.Fatalf("hex 往返解包失败：%v", err)
	}
	if unpacked.Operation != packet.Operation || string(unpacked.Payload) != string(packet.Payload) {
		t.Fatalf("hex 往返结果 = %+v，期望还原 packet", unpacked)
	}

	samplePath := t.TempDir() + string(os.PathSeparator) + "pack-sample.json"
	if err := protocol.WritePacketSample(samplePath, packet, packed, unpacked, debug); err != nil {
		t.Fatalf("写样本失败：%v", err)
	}
	raw, err := os.ReadFile(samplePath)
	if err != nil {
		t.Fatalf("读取样本失败：%v", err)
	}
	var sample map[string]any
	if err := json.Unmarshal(raw, &sample); err != nil {
		t.Fatalf("样本不是 JSON：%v", err)
	}
	for _, key := range []string{"request", "packed", "unpacked", "debug"} {
		if _, ok := sample[key]; !ok {
			t.Fatalf("样本缺少字段 %s：%+v", key, sample)
		}
	}
	packedSample, ok := sample["packed"].(map[string]any)
	if !ok || packedSample["hex"] != protocol.ToHex(packed) {
		t.Fatalf("样本 packed = %+v，期望记录 hex", sample["packed"])
	}
}

func TestUnpackBusinessPacketRejectsCorruptFrames(t *testing.T) {
	packet := protocol.BusinessPacket{Operation: "Msg.Sync", Payload: []byte("payload"), Flags: 0}
	packed, _, err := protocol.PackBusinessPacket(packet)
	if err != nil {
		t.Fatalf("封包失败：%v", err)
	}

	badMagic := append([]byte(nil), packed...)
	badMagic[0] = 'X'
	if _, _, err := protocol.UnpackBusinessPacket(badMagic); !errors.Is(err, protocol.ErrBadMagic) {
		t.Fatalf("bad magic 错误 = %v，期望 ErrBadMagic", err)
	}

	truncated := packed[:protocol.HeaderLength-1]
	if _, _, err := protocol.UnpackBusinessPacket(truncated); !errors.Is(err, protocol.ErrFrameTooShort) {
		t.Fatalf("truncated 错误 = %v，期望 ErrFrameTooShort", err)
	}

	badCRC := append([]byte(nil), packed...)
	badCRC[len(badCRC)-1] ^= 0xff
	if _, _, err := protocol.UnpackBusinessPacket(badCRC); !errors.Is(err, protocol.ErrCRC32Mismatch) {
		t.Fatalf("bad crc 错误 = %v，期望 ErrCRC32Mismatch", err)
	}
}
