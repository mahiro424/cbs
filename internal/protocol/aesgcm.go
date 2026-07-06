package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/mahiro424/cbs/internal/algorithm"
)

// AESGCMUnpackRequest 描述一次协议层 AES-GCM 解包请求。
type AESGCMUnpackRequest struct {
	Operation  string `json:"operation"`
	Key        []byte `json:"key"`
	Nonce      []byte `json:"nonce"`
	AAD        []byte `json:"aad,omitempty"`
	Ciphertext []byte `json:"ciphertext"`
}

// AESGCMUnpackHexRequest 描述十六进制样本形式的 AES-GCM 解包请求。
type AESGCMUnpackHexRequest struct {
	Operation     string `json:"operation"`
	KeyHex        string `json:"key_hex"`
	NonceHex      string `json:"nonce_hex"`
	AADHex        string `json:"aad_hex,omitempty"`
	CiphertextHex string `json:"ciphertext_hex"`
}

// AESGCMUnpackDebug 保存 AES-GCM 解包时可安全输出的结构化摘要。
type AESGCMUnpackDebug struct {
	KeyLength        int `json:"key_length"`
	NonceLength      int `json:"nonce_length"`
	AADLength        int `json:"aad_length"`
	CiphertextLength int `json:"ciphertext_length"`
	PlaintextLength  int `json:"plaintext_length"`
}

// AESGCMUnpackResult 保存协议层 AES-GCM 解包结果和安全摘要。
type AESGCMUnpackResult struct {
	Operation        string            `json:"operation"`
	Plaintext        []byte            `json:"plaintext"`
	PlaintextHex     string            `json:"plaintext_hex"`
	PlaintextSHA256  string            `json:"plaintext_sha256"`
	CiphertextSHA256 string            `json:"ciphertext_sha256"`
	PlaintextLength  int               `json:"plaintext_length"`
	Debug            AESGCMUnpackDebug `json:"debug"`
}

// AESGCMUnpack 使用基础算法层解密 AES-GCM 响应，并返回协议层摘要。
func AESGCMUnpack(request AESGCMUnpackRequest) (AESGCMUnpackResult, error) {
	plaintext, err := algorithm.AESGCMDecrypt(request.Ciphertext, request.Key, request.Nonce, request.AAD)
	if err != nil {
		return AESGCMUnpackResult{}, err
	}
	plainDigest := sha256.Sum256(plaintext)
	cipherDigest := sha256.Sum256(request.Ciphertext)
	return AESGCMUnpackResult{
		Operation:        request.Operation,
		Plaintext:        plaintext,
		PlaintextHex:     ToHex(plaintext),
		PlaintextSHA256:  hex.EncodeToString(plainDigest[:]),
		CiphertextSHA256: hex.EncodeToString(cipherDigest[:]),
		PlaintextLength:  len(plaintext),
		Debug: AESGCMUnpackDebug{
			KeyLength:        len(request.Key),
			NonceLength:      len(request.Nonce),
			AADLength:        len(request.AAD),
			CiphertextLength: len(request.Ciphertext),
			PlaintextLength:  len(plaintext),
		},
	}, nil
}

// UnpackBusinessPacketWithAESGCM 是协议层命名的 AES-GCM 响应解包入口。
func UnpackBusinessPacketWithAESGCM(request AESGCMUnpackRequest) (AESGCMUnpackResult, error) {
	return AESGCMUnpack(request)
}

// AESGCMUnpackHex 将十六进制样本解码后执行 AES-GCM 解包。
func AESGCMUnpackHex(request AESGCMUnpackHexRequest) (AESGCMUnpackResult, error) {
	key, err := FromHex(request.KeyHex)
	if err != nil {
		return AESGCMUnpackResult{}, err
	}
	nonce, err := FromHex(request.NonceHex)
	if err != nil {
		return AESGCMUnpackResult{}, err
	}
	aad, err := FromHex(request.AADHex)
	if err != nil {
		return AESGCMUnpackResult{}, err
	}
	ciphertext, err := FromHex(request.CiphertextHex)
	if err != nil {
		return AESGCMUnpackResult{}, err
	}
	return AESGCMUnpack(AESGCMUnpackRequest{
		Operation:  request.Operation,
		Key:        key,
		Nonce:      nonce,
		AAD:        aad,
		Ciphertext: ciphertext,
	})
}

// WriteAESGCMUnpackSample 把一次 AES-GCM 解包输入输出保存为 JSON 样本。
func WriteAESGCMUnpackSample(path string, request AESGCMUnpackRequest, result AESGCMUnpackResult) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	sample := map[string]any{
		"request":   aesGCMUnpackRequestSample(request),
		"decrypted": aesGCMUnpackResultSample(result),
		"debug":     result.Debug,
	}
	payload, err := json.MarshalIndent(sample, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(payload, '\n'), 0o644)
}

func aesGCMUnpackRequestSample(request AESGCMUnpackRequest) map[string]any {
	return map[string]any{
		"operation":      request.Operation,
		"key_hex":        ToHex(request.Key),
		"nonce_hex":      ToHex(request.Nonce),
		"aad_hex":        ToHex(request.AAD),
		"ciphertext_hex": ToHex(request.Ciphertext),
	}
}

func aesGCMUnpackResultSample(result AESGCMUnpackResult) map[string]any {
	return map[string]any{
		"operation":         result.Operation,
		"plaintext_utf8":    string(result.Plaintext),
		"plaintext_hex":     result.PlaintextHex,
		"plaintext_sha256":  result.PlaintextSHA256,
		"ciphertext_sha256": result.CiphertextSHA256,
		"plaintext_length":  result.PlaintextLength,
	}
}
