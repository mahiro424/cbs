package protocol_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/mahiro424/cbs/internal/protocol"
)

func TestAESGCMUnpackDecryptsGoldenResponse(t *testing.T) {
	request := protocol.AESGCMUnpackRequest{
		Operation:  "Login.GetQR.Response",
		Key:        []byte("1234567890abcdef"),
		Nonce:      []byte("123456789012"),
		AAD:        []byte("login"),
		Ciphertext: mustFromHex(t, "0adcec478f28559836283bc8e90a898f4319319e7fa7fd9a003d984fc18909f2811250c2"),
	}

	result, err := protocol.AESGCMUnpack(request)
	if err != nil {
		t.Fatalf("AESGCMUnpack 返回错误：%v", err)
	}

	if result.Operation != request.Operation {
		t.Fatalf("operation = %s，期望 %s", result.Operation, request.Operation)
	}
	if string(result.Plaintext) != "hybrid ecdh response" || result.PlaintextHex != "687962726964206563646820726573706f6e7365" {
		t.Fatalf("明文 = %q / %s，期望还原 golden 明文", result.Plaintext, result.PlaintextHex)
	}
	if result.PlaintextSHA256 != "bea5c4aecf2ca9b0518a453487f5ceb2f8616b26b9c9aa824a34c983adad59bb" {
		t.Fatalf("plaintext sha256 = %s，期望 golden 摘要", result.PlaintextSHA256)
	}
	if result.CiphertextSHA256 != "778fe94c2fa71f49f7706339272b451bfe299e4a4edd44e05e482b3d7e7fa8e7" {
		t.Fatalf("ciphertext sha256 = %s，期望 golden 摘要", result.CiphertextSHA256)
	}
	if result.Debug.KeyLength != 16 || result.Debug.NonceLength != 12 || result.Debug.AADLength != 5 || result.Debug.CiphertextLength != len(request.Ciphertext) || result.Debug.PlaintextLength != len(result.Plaintext) {
		t.Fatalf("debug = %+v，期望记录 key/nonce/aad/ciphertext/plaintext 长度", result.Debug)
	}

	aliasResult, err := protocol.UnpackBusinessPacketWithAESGCM(request)
	if err != nil {
		t.Fatalf("UnpackBusinessPacketWithAESGCM 返回错误：%v", err)
	}
	if aliasResult.PlaintextHex != result.PlaintextHex || aliasResult.PlaintextSHA256 != result.PlaintextSHA256 {
		t.Fatalf("语义别名结果 = %+v，期望与 AESGCMUnpack 一致", aliasResult)
	}
}

func TestAESGCMUnpackHexDecryptsHexInputs(t *testing.T) {
	result, err := protocol.AESGCMUnpackHex(protocol.AESGCMUnpackHexRequest{
		Operation:     "Login.GetQR.Response",
		KeyHex:        "31323334353637383930616263646566",
		NonceHex:      "313233343536373839303132",
		AADHex:        "6c6f67696e",
		CiphertextHex: "0adcec478f28559836283bc8e90a898f4319319e7fa7fd9a003d984fc18909f2811250c2",
	})
	if err != nil {
		t.Fatalf("AESGCMUnpackHex 返回错误：%v", err)
	}
	if string(result.Plaintext) != "hybrid ecdh response" || result.PlaintextHex != "687962726964206563646820726573706f6e7365" {
		t.Fatalf("hex 解包明文 = %q / %s，期望还原 golden 明文", result.Plaintext, result.PlaintextHex)
	}
	if result.Debug.KeyLength != 16 || result.Debug.NonceLength != 12 || result.Debug.AADLength != 5 {
		t.Fatalf("hex 解包 debug = %+v，期望长度来自 hex 解码后的字节", result.Debug)
	}
}

func TestAESGCMUnpackRejectsAuthenticationFailureAndBadNonce(t *testing.T) {
	request := protocol.AESGCMUnpackRequest{
		Operation:  "Login.GetQR.Response",
		Key:        []byte("1234567890abcdef"),
		Nonce:      []byte("123456789012"),
		AAD:        []byte("login"),
		Ciphertext: mustFromHex(t, "0adcec478f28559836283bc8e90a898f4319319e7fa7fd9a003d984fc18909f2811250c2"),
	}
	request.Ciphertext[len(request.Ciphertext)-1] ^= 0xff
	if _, err := protocol.AESGCMUnpack(request); err == nil {
		t.Fatalf("篡改认证标签后 AESGCMUnpack 未返回错误")
	}

	request.Ciphertext = mustFromHex(t, "0adcec478f28559836283bc8e90a898f4319319e7fa7fd9a003d984fc18909f2811250c2")
	request.Nonce = []byte("short")
	if _, err := protocol.AESGCMUnpack(request); err == nil {
		t.Fatalf("nonce 长度错误时 AESGCMUnpack 未返回错误")
	}
}

func TestWriteAESGCMUnpackSampleContainsRequestDecryptedAndDebug(t *testing.T) {
	request := protocol.AESGCMUnpackRequest{
		Operation:  "Login.GetQR.Response",
		Key:        []byte("1234567890abcdef"),
		Nonce:      []byte("123456789012"),
		AAD:        []byte("login"),
		Ciphertext: mustFromHex(t, "0adcec478f28559836283bc8e90a898f4319319e7fa7fd9a003d984fc18909f2811250c2"),
	}
	result, err := protocol.AESGCMUnpack(request)
	if err != nil {
		t.Fatalf("AESGCMUnpack 返回错误：%v", err)
	}

	samplePath := t.TempDir() + string(os.PathSeparator) + "aesgcm-sample.json"
	if err := protocol.WriteAESGCMUnpackSample(samplePath, request, result); err != nil {
		t.Fatalf("写 AES-GCM 解包样本失败：%v", err)
	}
	raw, err := os.ReadFile(samplePath)
	if err != nil {
		t.Fatalf("读取 AES-GCM 解包样本失败：%v", err)
	}
	var sample map[string]any
	if err := json.Unmarshal(raw, &sample); err != nil {
		t.Fatalf("AES-GCM 解包样本不是 JSON：%v", err)
	}
	for _, key := range []string{"request", "decrypted", "debug"} {
		if _, ok := sample[key]; !ok {
			t.Fatalf("AES-GCM 解包样本缺少字段 %s：%+v", key, sample)
		}
	}
	decrypted, ok := sample["decrypted"].(map[string]any)
	if !ok || decrypted["plaintext_hex"] != result.PlaintextHex || decrypted["plaintext_utf8"] != "hybrid ecdh response" {
		t.Fatalf("decrypted 样本 = %+v，期望记录明文 hex/utf8", sample["decrypted"])
	}
	requestSample, ok := sample["request"].(map[string]any)
	if !ok || requestSample["ciphertext_hex"] == "" || requestSample["key_hex"] == "" {
		t.Fatalf("request 样本 = %+v，期望记录可复现的 hex 输入", sample["request"])
	}
}

func mustFromHex(t *testing.T, value string) []byte {
	t.Helper()
	out, err := protocol.FromHex(value)
	if err != nil {
		t.Fatalf("hex 解码失败：%v", err)
	}
	return out
}
