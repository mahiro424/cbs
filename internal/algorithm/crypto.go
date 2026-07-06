package algorithm

import (
	"bytes"
	"compress/zlib"
	"crypto/aes"
	"crypto/cipher"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
)

type CurveKind string

const (
	CurveP256 CurveKind = "P-256"
	CurveP224 CurveKind = "P-224"
)

type ECDHKey struct {
	Curve      CurveKind
	PrivateKey []byte
	PublicKey  []byte
}

func AESCBCEncrypt(plain, key, iv []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(iv) != block.BlockSize() {
		return nil, fmt.Errorf("iv ??? %d??? %d", len(iv), block.BlockSize())
	}
	padded := pkcs7Pad(plain, block.BlockSize())
	out := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(out, padded)
	return out, nil
}

func AESCBCDecrypt(ciphertext, key, iv []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(iv) != block.BlockSize() {
		return nil, fmt.Errorf("iv ??? %d??? %d", len(iv), block.BlockSize())
	}
	if len(ciphertext) == 0 || len(ciphertext)%block.BlockSize() != 0 {
		return nil, errors.New("AES-CBC ??????????????")
	}
	out := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(out, ciphertext)
	return pkcs7Unpad(out, block.BlockSize())
}

func AESECBEncrypt(plain, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	padded := pkcs7Pad(plain, block.BlockSize())
	out := make([]byte, len(padded))
	for start := 0; start < len(padded); start += block.BlockSize() {
		block.Encrypt(out[start:start+block.BlockSize()], padded[start:start+block.BlockSize()])
	}
	return out, nil
}

func AESECBDecrypt(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) == 0 || len(ciphertext)%block.BlockSize() != 0 {
		return nil, errors.New("AES-ECB ??????????????")
	}
	out := make([]byte, len(ciphertext))
	for start := 0; start < len(ciphertext); start += block.BlockSize() {
		block.Decrypt(out[start:start+block.BlockSize()], ciphertext[start:start+block.BlockSize()])
	}
	return pkcs7Unpad(out, block.BlockSize())
}

func AESGCMEncrypt(plain, key, nonce, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, fmt.Errorf("nonce ??? %d??? %d", len(nonce), gcm.NonceSize())
	}
	return gcm.Seal(nil, nonce, plain, aad), nil
}

func AESGCMDecrypt(ciphertext, key, nonce, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, fmt.Errorf("nonce ??? %d??? %d", len(nonce), gcm.NonceSize())
	}
	return gcm.Open(nil, nonce, ciphertext, aad)
}

func HKDFExtractSHA256(salt, ikm []byte) []byte {
	if salt == nil {
		salt = make([]byte, sha256.Size)
	}
	h := hmac.New(sha256.New, salt)
	h.Write(ikm)
	return h.Sum(nil)
}

func HKDFExpandSHA256(prk, info []byte, length int) ([]byte, error) {
	if length < 0 {
		return nil, errors.New("HKDF ????????")
	}
	if length > 255*sha256.Size {
		return nil, errors.New("HKDF ??????")
	}
	var result []byte
	var previous []byte
	counter := byte(1)
	for len(result) < length {
		h := hmac.New(sha256.New, prk)
		h.Write(previous)
		h.Write(info)
		h.Write([]byte{counter})
		previous = h.Sum(nil)
		result = append(result, previous...)
		counter++
	}
	return result[:length], nil
}

func MD5(data []byte) []byte {
	sum := md5.Sum(data)
	return sum[:]
}

func SHA256(data []byte) []byte {
	sum := sha256.Sum256(data)
	return sum[:]
}

func CRC32IEEE(data []byte) uint32 {
	return crc32.ChecksumIEEE(data)
}

func ZlibCompress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func ZlibDecompress(data []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

func GenerateECDHKey(kind CurveKind) (*ECDHKey, error) {
	curve, err := curveFor(kind)
	if err != nil {
		return nil, err
	}
	priv, x, y, err := elliptic.GenerateKey(curve, rand.Reader)
	if err != nil {
		return nil, err
	}
	return &ECDHKey{Curve: kind, PrivateKey: priv, PublicKey: elliptic.Marshal(curve, x, y)}, nil
}

func (k *ECDHKey) SharedSecret(peerPublic []byte) ([]byte, error) {
	curve, err := curveFor(k.Curve)
	if err != nil {
		return nil, err
	}
	x, y := elliptic.Unmarshal(curve, peerPublic)
	if x == nil || y == nil {
		return nil, errors.New("??? ECDH ??")
	}
	sx, _ := curve.ScalarMult(x, y, k.PrivateKey)
	if sx == nil {
		return nil, errors.New("ECDH ????")
	}
	out := make([]byte, (curve.Params().BitSize+7)/8)
	return sx.FillBytes(out), nil
}

func curveFor(kind CurveKind) (elliptic.Curve, error) {
	switch kind {
	case CurveP256:
		return elliptic.P256(), nil
	case CurveP224:
		return elliptic.P224(), nil
	default:
		return nil, fmt.Errorf("???????%s", kind)
	}
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	return append(append([]byte{}, data...), bytes.Repeat([]byte{byte(padding)}, padding)...)
}

func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, errors.New("PKCS7 ??????")
	}
	padding := int(data[len(data)-1])
	if padding == 0 || padding > blockSize || padding > len(data) {
		return nil, errors.New("PKCS7 ????")
	}
	for _, b := range data[len(data)-padding:] {
		if int(b) != padding {
			return nil, errors.New("PKCS7 ??????")
		}
	}
	return append([]byte{}, data[:len(data)-padding]...), nil
}
