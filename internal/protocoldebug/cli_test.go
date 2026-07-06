package protocoldebug_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/mahiro424/cbs/internal/protocoldebug"
)

const sampleFrameHex = "434253310103000b00000013c463dfb64d73672e53656e6454787468656c6c6f206d6f636b2070726f746f636f6c"
const differentFlagsFrameHex = "434253310104000b00000013c463dfb64d73672e53656e6454787468656c6c6f206d6f636b2070726f746f636f6c"

func TestProtocolDebugCLIInspectWritesJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := protocoldebug.Run([]string{"inspect", "--hex", sampleFrameHex}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d，stderr=%s", exitCode, stderr.String())
	}
	var out map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("stdout 不是 JSON：%v，内容：%s", err, stdout.String())
	}
	if out["status"] != "ok" || out["length"] != float64(46) {
		t.Fatalf("inspect 输出 = %+v，期望 ok 且 length=46", out)
	}
	packet, ok := out["packet"].(map[string]any)
	if !ok || packet["operation"] != "Msg.SendTxt" || packet["flags"] != float64(3) {
		t.Fatalf("inspect packet = %+v，期望还原 Msg.SendTxt flags=3", out["packet"])
	}
}

func TestProtocolDebugCLICompareWritesStructuredDifference(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := protocoldebug.Run([]string{"compare", "--expected", sampleFrameHex, "--actual", differentFlagsFrameHex}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d，stderr=%s", exitCode, stderr.String())
	}
	var out map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("stdout 不是 JSON：%v，内容：%s", err, stdout.String())
	}
	if out["status"] != "ok" || out["equal"] != false {
		t.Fatalf("compare 输出 = %+v，期望 ok 且 equal=false", out)
	}
	diff, ok := out["first_difference"].(map[string]any)
	if !ok || diff["offset"] != float64(5) || diff["expected_hex"] != "03" || diff["actual_hex"] != "04" {
		t.Fatalf("first_difference = %+v，期望 offset=5 expected=03 actual=04", out["first_difference"])
	}
}
