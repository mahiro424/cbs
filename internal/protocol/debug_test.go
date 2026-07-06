package protocol_test

import (
	"testing"

	"github.com/mahiro424/cbs/internal/protocol"
)

func TestInspectBusinessPacketHexReturnsStructuredPacketAndDebug(t *testing.T) {
	const frameHex = "434253310103000b00000013c463dfb64d73672e53656e6454787468656c6c6f206d6f636b2070726f746f636f6c"

	inspection, err := protocol.InspectBusinessPacketHex(frameHex)
	if err != nil {
		t.Fatalf("InspectBusinessPacketHex 返回错误：%v", err)
	}

	if inspection.Status != "ok" || inspection.Hex != frameHex || inspection.Length != 46 {
		t.Fatalf("inspection 基本字段 = %+v，期望 ok/hex/length", inspection)
	}
	if inspection.Packet.Operation != "Msg.SendTxt" || inspection.Packet.PayloadUTF8 != "hello mock protocol" || inspection.Packet.PayloadHex != "68656c6c6f206d6f636b2070726f746f636f6c" || inspection.Packet.Flags != 3 {
		t.Fatalf("packet = %+v，期望还原 operation/payload/flags", inspection.Packet)
	}
	if inspection.Debug.Magic != "CBS1" || inspection.Debug.CRC32Hex != "c463dfb6" || inspection.Debug.TotalLength != 46 {
		t.Fatalf("debug = %+v，期望包含 mock 帧摘要", inspection.Debug)
	}
}

func TestCompareBusinessPacketHexReportsEqualMismatchAndMissingSample(t *testing.T) {
	const expectedHex = "434253310103000b00000013c463dfb64d73672e53656e6454787468656c6c6f206d6f636b2070726f746f636f6c"
	const actualHex = "434253310104000b00000013c463dfb64d73672e53656e6454787468656c6c6f206d6f636b2070726f746f636f6c"

	equal, err := protocol.CompareBusinessPacketHex(expectedHex, expectedHex)
	if err != nil {
		t.Fatalf("相同样本 compare 返回错误：%v", err)
	}
	if equal.Status != "ok" || !equal.Equal || equal.FirstDifference != nil || equal.ExpectedLength != 46 || equal.ActualLength != 46 {
		t.Fatalf("相同样本 compare = %+v，期望 equal=true 且无差异", equal)
	}

	mismatch, err := protocol.CompareBusinessPacketHex(expectedHex, actualHex)
	if err != nil {
		t.Fatalf("不同样本 compare 返回错误：%v", err)
	}
	if mismatch.Status != "ok" || mismatch.Equal {
		t.Fatalf("不同样本 compare = %+v，期望 equal=false", mismatch)
	}
	if mismatch.FirstDifference == nil || mismatch.FirstDifference.Offset != 5 || mismatch.FirstDifference.ExpectedHex != "03" || mismatch.FirstDifference.ActualHex != "04" {
		t.Fatalf("首个差异 = %+v，期望 offset=5 expected=03 actual=04", mismatch.FirstDifference)
	}
	if mismatch.Expected.Packet.Flags != 3 || mismatch.Actual.Packet.Flags != 4 {
		t.Fatalf("expected/actual inspect = %+v / %+v，期望保留双方解包摘要", mismatch.Expected, mismatch.Actual)
	}

	skipped, err := protocol.CompareBusinessPacketHex(expectedHex, "")
	if err != nil {
		t.Fatalf("缺少 actual 样本不应返回硬错误：%v", err)
	}
	if skipped.Status != "skipped" || skipped.SkipReason == "" || skipped.Equal {
		t.Fatalf("缺少样本 compare = %+v，期望 skipped 且包含 skip_reason", skipped)
	}
}
