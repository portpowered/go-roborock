package rest

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"math/big"
	"strconv"
	"strings"

	"github.com/portpowered/go-roborock/pkg/dependencymodels"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

const mercyCharacters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
const mercyNonceLength = 16
const hawkEntropyBytes = 6

func (c *Client) mercyNonce() (string, error) {
	nonceBytes := make([]byte, mercyNonceLength)

	limit := big.NewInt(int64(len(mercyCharacters)))

	for index := range nonceBytes {
		n, err := rand.Int(c.random, limit)
		if err != nil {
			return "", roborockerrors.New(roborockerrors.Unavailable, "sign_key", "entropy unavailable", err)
		}

		nonceBytes[index] = mercyCharacters[n.Int64()]
	}

	return string(nonceBytes), nil
}
func validMercyNonce(s string) bool {
	if len(s) != mercyNonceLength {
		return false
	}

	for _, character := range s {
		if !strings.ContainsRune(mercyCharacters, character) {
			return false
		}
	}

	return true
}

func (c *Client) hawk(request dependencymodels.RRiot, path string) (string, error) {
	if request.U == "" || request.S == "" || request.H == "" || strings.ContainsAny(request.U+request.S, "\"\r\n\\") {
		return "", roborockerrors.New(roborockerrors.InvalidArgument, "hawk", "invalid Hawk credentials", nil)
	}

	entropy := make([]byte, hawkEntropyBytes)
	_, err := io.ReadFull(c.random, entropy)
	if err != nil {
		return "", roborockerrors.New(roborockerrors.Unavailable, "hawk", "entropy unavailable", err)
	}

	nonce := base64.RawURLEncoding.EncodeToString(entropy)
	timestamp := strconv.FormatInt(c.clock().Unix(), 10)
	digest := md5.Sum([]byte(path))
	input := strings.Join([]string{request.U, request.S, nonce, timestamp, hex.EncodeToString(digest[:]), "", ""}, ":")
	mac := hmac.New(sha256.New, []byte(request.H))
	_, _ = mac.Write([]byte(input))

	return fmt.Sprintf(`Hawk id="%s",s="%s",ts="%s",nonce="%s",mac="%s"`,
		request.U, request.S, timestamp, nonce, base64.StdEncoding.EncodeToString(mac.Sum(nil))), nil
}
