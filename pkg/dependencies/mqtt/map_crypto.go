package mqtt

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5" //nolint:gosec // Q7 map key derivation is mandated by the reference protocol.
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"github.com/portpowered/go-roborock/internal/protocol"
)

const (
	maxMapData         = 32 * 1024 * 1024
	base64QuantumChars = 4
)

func decodeV1Map(encrypted []byte, nonce string) ([]byte, error) {
	key, err := hex.DecodeString(nonce)
	if err != nil {
		return nil, transportError("V1 map nonce", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, transportError("V1 map cipher", err)
	}

	if len(encrypted) == 0 || len(encrypted)%aes.BlockSize != 0 {
		return nil, transportError("V1 map", errInvalidCiphertextLength)
	}

	plain := make([]byte, len(encrypted))
	cipher.NewCBCDecrypter(block, make([]byte, aes.BlockSize)).CryptBlocks(plain, encrypted)

	plain, err = unpadPayload(plain)
	if err != nil {
		return nil, transportError("V1 map padding", err)
	}

	reader, err := gzip.NewReader(bytes.NewReader(plain))
	if err != nil {
		return nil, transportError("V1 map gzip", err)
	}

	return inflateMap(reader)
}

func q7MapKey(serial, model string) ([]byte, error) {
	parts := strings.Split(model, ".")

	suffix := parts[len(parts)-1]
	if serial == "" || suffix == "" || !asciiText(serial+suffix) {
		return nil, invalid("Q7 map", "serial and model must be nonempty ASCII")
	}

	paddedModel := suffix + strings.Repeat(protocol.B01Q7ModelKeyPadding, protocol.B01Q7ModelKeyBytes)
	modelKey := paddedModel[:protocol.B01Q7ModelKeyBytes]

	block, err := aes.NewCipher([]byte(modelKey))
	if err != nil {
		return nil, transportError("Q7 model key", err)
	}

	material := []byte(fmt.Sprintf(protocol.B01Q7MapKeyInputFormat, serial, suffix, serial))
	padding := aes.BlockSize - len(material)%aes.BlockSize
	material = append(material, bytes.Repeat([]byte{byte(padding)}, padding)...)

	encrypted := make([]byte, len(material))

	for offset := 0; offset < len(material); offset += aes.BlockSize {
		block.Encrypt(encrypted[offset:offset+aes.BlockSize], material[offset:offset+aes.BlockSize])
	}

	digest := md5.Sum([]byte(base64.StdEncoding.EncodeToString(encrypted))) //nolint:gosec // Vendor map-key derivation.

	return []byte(hex.EncodeToString(digest[:])[protocol.B01Q7MapKeyHexStart:protocol.B01Q7MapKeyHexEnd]), nil
}

func decodeQ7Map(payload, key []byte) ([]byte, error) {
	text := strings.TrimSpace(string(payload))
	if remainder := len(text) % base64QuantumChars; remainder != 0 {
		text += strings.Repeat("=", base64QuantumChars-remainder)
	}

	encrypted, err := base64.StdEncoding.Strict().DecodeString(text)
	if err != nil {
		return nil, transportError("Q7 map base64", err)
	}

	if len(encrypted) == 0 || len(encrypted)%aes.BlockSize != 0 {
		return nil, transportError("Q7 map", errInvalidCiphertextLength)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, transportError("Q7 map cipher", err)
	}

	plain := make([]byte, len(encrypted))
	for offset := 0; offset < len(encrypted); offset += aes.BlockSize {
		block.Decrypt(plain[offset:offset+aes.BlockSize], encrypted[offset:offset+aes.BlockSize])
	}

	plain, err = unpadPayload(plain)
	if err != nil {
		return nil, transportError("Q7 map padding", err)
	}

	compressed, err := hex.DecodeString(string(plain))
	if err != nil {
		return nil, transportError("Q7 map hex", err)
	}

	reader, err := zlib.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, transportError("Q7 map zlib", err)
	}

	return inflateMap(reader)
}

func inflateMap(reader io.ReadCloser) ([]byte, error) {
	defer func() { _ = reader.Close() }()

	data, err := io.ReadAll(io.LimitReader(reader, maxMapData+1))
	if err != nil {
		return nil, transportError("map decompression", err)
	}

	if len(data) > maxMapData {
		return nil, invalid("map", "decompressed map exceeds size limit")
	}

	return data, nil
}

func asciiText(text string) bool {
	for _, char := range text {
		if char > 127 || char < 32 {
			return false
		}
	}

	return true
}
