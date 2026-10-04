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
	b := make([]byte, mercyNonceLength)
	max := big.NewInt(int64(len(mercyCharacters)))
	for i := range b {
		n, err := rand.Int(c.random, max)
		if err != nil {
			return "", roborockerrors.New(roborockerrors.Unavailable, "sign_key", "entropy unavailable", err)
		}
		b[i] = mercyCharacters[n.Int64()]
	}
	return string(b), nil
}
func validMercyNonce(s string) bool {
	if len(s) != mercyNonceLength {
		return false
	}
	for _, r := range s {
		if !strings.ContainsRune(mercyCharacters, r) {
			return false
		}
	}
	return true
}

func (c *Client) hawk(r dependencymodels.RRiot, path string) (string, error) {
	if r.U == "" || r.S == "" || r.H == "" || strings.ContainsAny(r.U+r.S, "\"\r\n\\") {
		return "", roborockerrors.New(roborockerrors.InvalidArgument, "hawk", "invalid Hawk credentials", nil)
	}
	entropy := make([]byte, hawkEntropyBytes)
	if _, err := io.ReadFull(c.random, entropy); err != nil {
		return "", roborockerrors.New(roborockerrors.Unavailable, "hawk", "entropy unavailable", err)
	}
	nonce := base64.RawURLEncoding.EncodeToString(entropy)
	ts := strconv.FormatInt(c.clock().Unix(), 10)
	digest := md5.Sum([]byte(path))
	input := strings.Join([]string{r.U, r.S, nonce, ts, hex.EncodeToString(digest[:]), "", ""}, ":")
	mac := hmac.New(sha256.New, []byte(r.H))
	_, _ = mac.Write([]byte(input))
	return fmt.Sprintf(`Hawk id="%s",s="%s",ts="%s",nonce="%s",mac="%s"`,
		r.U, r.S, ts, nonce, base64.StdEncoding.EncodeToString(mac.Sum(nil))), nil
}
