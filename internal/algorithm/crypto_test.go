package algorithm_test

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/mahiro424/cbs/internal/algorithm"
)

func TestAESCBCEncryptDecryptRoundTrip(t *testing.T) {
	key := []byte("1234567890abcdef")
	iv := []byte("abcdef1234567890")
	plain := []byte("wechat login packet")

	ciphertext, err := algorithm.AESCBCEncrypt(plain, key, iv)
	if err != nil {
		t.Fatalf("加密失败：%v", err)
	}
	if bytes.Equal(ciphertext, plain) {
		t.Fatalf("密文不应等于明文")
	}
	got, err := algorithm.AESCBCDecrypt(ciphertext, key, iv)
	if err != nil {
		t.Fatalf("解密失败：%v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("解密结果 = %q，期望 %q", got, plain)
	}
}

func TestAESGCMEncryptDecryptRoundTrip(t *testing.T) {
	key := []byte("1234567890abcdef")
	nonce := []byte("123456789012")
	plain := []byte("hybrid ecdh response")
	aad := []byte("login")

	ciphertext, err := algorithm.AESGCMEncrypt(plain, key, nonce, aad)
	if err != nil {
		t.Fatalf("GCM 加密失败：%v", err)
	}
	got, err := algorithm.AESGCMDecrypt(ciphertext, key, nonce, aad)
	if err != nil {
		t.Fatalf("GCM 解密失败：%v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("GCM 解密结果 = %q，期望 %q", got, plain)
	}
}

func TestHKDFRFC5869Case1(t *testing.T) {
	ikm := bytes.Repeat([]byte{0x0b}, 22)
	salt, _ := hex.DecodeString("000102030405060708090a0b0c")
	info, _ := hex.DecodeString("f0f1f2f3f4f5f6f7f8f9")
	wantPRK, _ := hex.DecodeString("077709362c2e32df0ddc3f0dc47bba6390b6c73bb50f9c3122ec844ad7c2b3e5")
	wantOKM, _ := hex.DecodeString("3cb25f25faacd57a90434f64d0362f2a2d2d0a90cf1a5a4c5db02d56ecc4c5bf34007208d5b887185865")

	prk := algorithm.HKDFExtractSHA256(salt, ikm)
	if !bytes.Equal(prk, wantPRK) {
		t.Fatalf("PRK = %x，期望 %x", prk, wantPRK)
	}
	okm, err := algorithm.HKDFExpandSHA256(prk, info, 42)
	if err != nil {
		t.Fatalf("HKDF Expand 失败：%v", err)
	}
	if !bytes.Equal(okm, wantOKM) {
		t.Fatalf("OKM = %x，期望 %x", okm, wantOKM)
	}
}

func TestCRC32IEEEKnownVector(t *testing.T) {
	got := algorithm.CRC32IEEE([]byte("123456789"))
	if got != 0xcbf43926 {
		t.Fatalf("CRC32 = %#x，期望 0xcbf43926", got)
	}
}

func TestZlibRoundTrip(t *testing.T) {
	plain := []byte("mock network packet mock network packet")
	compressed, err := algorithm.ZlibCompress(plain)
	if err != nil {
		t.Fatalf("压缩失败：%v", err)
	}
	got, err := algorithm.ZlibDecompress(compressed)
	if err != nil {
		t.Fatalf("解压失败：%v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("解压结果 = %q，期望 %q", got, plain)
	}
}

func TestECDHSharedSecretsMatch(t *testing.T) {
	for _, curve := range []algorithm.CurveKind{algorithm.CurveP256, algorithm.CurveP224} {
		alice, err := algorithm.GenerateECDHKey(curve)
		if err != nil {
			t.Fatalf("生成 Alice 密钥失败：%v", err)
		}
		bob, err := algorithm.GenerateECDHKey(curve)
		if err != nil {
			t.Fatalf("生成 Bob 密钥失败：%v", err)
		}
		aliceSecret, err := alice.SharedSecret(bob.PublicKey)
		if err != nil {
			t.Fatalf("Alice 协商失败：%v", err)
		}
		bobSecret, err := bob.SharedSecret(alice.PublicKey)
		if err != nil {
			t.Fatalf("Bob 协商失败：%v", err)
		}
		if !bytes.Equal(aliceSecret, bobSecret) {
			t.Fatalf("%s 双方共享密钥不一致", curve)
		}
	}
}
