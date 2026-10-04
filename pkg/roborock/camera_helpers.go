package roborock

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/portpowered/go-roborock/pkg/dependencymodels"
)

func normalizePatternPassword(password string) string {
	compact := strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(password), "-", ""), " ", "")
	if len(compact) == md5.Size*2 {
		_, err := hex.DecodeString(compact)
		if err == nil {
			return strings.ToLower(compact)
		}
	}

	digest := md5.Sum([]byte(compact))

	return hex.EncodeToString(digest[:])
}

func firstString(values ...*string) string {
	for _, value := range values {
		if value != nil && *value != "" {
			return *value
		}
	}

	return ""
}

func decodeCameraSDP(raw json.RawMessage) (string, bool, error) {
	var wire dependencymodels.CameraDeviceSDP

	err := json.Unmarshal(raw, &wire)
	if err != nil {
		return "", false, fmt.Errorf("decode camera SDP: %w", err)
	}

	if wire.Sdp != nil && *wire.Sdp != "" {
		return *wire.Sdp, false, nil
	}

	if wire.DevSdp == nil || *wire.DevSdp == "" {
		return "", false, errResultShape
	}

	if *wire.DevSdp == string(dependencymodels.CameraSDPRetryPending) {
		return "", true, nil
	}

	return decodeCameraAnswer(*wire.DevSdp)
}

func decodeCameraAnswer(encoded string) (string, bool, error) {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", false, fmt.Errorf("decode camera SDP: %w", err)
	}

	var envelope dependencymodels.CameraSDPEnvelope

	err = json.Unmarshal(decoded, &envelope)
	if err != nil {
		return "", false, fmt.Errorf("decode camera SDP: %w", err)
	}

	if envelope.Type != dependencymodels.CameraSDPTypeAnswer || envelope.Sdp == "" {
		return "", false, errResultShape
	}

	return envelope.Sdp, false, nil
}
