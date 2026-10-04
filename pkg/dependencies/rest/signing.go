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

	"github.com/portpowered/go-roborock/internal/protocol"
	"github.com/portpowered/go-roborock/pkg/dependencymodels"
	"github.com/portpowered/go-roborock/pkg/roborockerrors"
)

func (c *Client) mercyNonce() (string, error) {
	nonceBytes := make([]byte, protocol.RESTMercyNonceLength)

	limit := big.NewInt(int64(len(protocol.RESTMercyCharacters)))

	for index := range nonceBytes {
		n, err := rand.Int(c.random, limit)
		if err != nil {
			return "", roborockerrors.New(roborockerrors.Unavailable, "sign_key", "entropy unavailable", err)
		}

		nonceBytes[index] = protocol.RESTMercyCharacters[n.Int64()]
	}

	return string(nonceBytes), nil
}
func validMercyNonce(s string) bool {
	if len(s) != protocol.RESTMercyNonceLength {
		return false
	}

	for _, character := range s {
		if !strings.ContainsRune(protocol.RESTMercyCharacters, character) {
			return false
		}
	}

	return true
}

func (c *Client) hawk(request dependencymodels.RRiot, path string) (string, error) {
	if request.U == "" || request.S == "" || request.H == "" || strings.ContainsAny(request.U+request.S, "\"\r\n\\") {
		return "", roborockerrors.New(roborockerrors.InvalidArgument, "hawk", "invalid Hawk credentials", nil)
	}

	entropy := make([]byte, protocol.RESTHawkEntropyBytes)

	_, err := io.ReadFull(c.random, entropy)
	if err != nil {
		return "", roborockerrors.New(roborockerrors.Unavailable, "hawk", "entropy unavailable", err)
	}

	nonce := base64.RawURLEncoding.EncodeToString(entropy)
	timestamp := strconv.FormatInt(c.clock().Unix(), 10)
	digest := md5.Sum([]byte(path))
	input := fmt.Sprintf(protocol.RESTHawkMACInputFormat,
		request.U, request.S, nonce, timestamp, hex.EncodeToString(digest[:]))
	mac := hmac.New(sha256.New, []byte(request.H))
	_, _ = mac.Write([]byte(input))

	return fmt.Sprintf(protocol.RESTHawkHeaderFormat,
		request.U, request.S, timestamp, nonce, base64.StdEncoding.EncodeToString(mac.Sum(nil))), nil
}
