package roborock

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/portpowered/go-roborock/pkg/dependencymodels"
)

func encodeCameraICE(request SendICERequest) (dependencymodels.CameraICEParameters, error) {
	mid, index := "0", uint16(0)
	if request.SdpMid != nil {
		mid = *request.SdpMid
	}

	if request.SdpMLineIndex != nil {
		index = *request.SdpMLineIndex
	}

	data, err := json.Marshal(dependencymodels.CameraICECandidate{
		Candidate: request.Candidate, SdpMid: &mid, SdpMLineIndex: &index,
	})
	if err != nil {
		return dependencymodels.CameraICEParameters{}, cameraProtocol(string(dependencymodels.RPCCameraSendICE), err)
	}

	return dependencymodels.CameraICEParameters{AppIce: base64.StdEncoding.EncodeToString(data)}, nil
}

func decodeCameraICE(raw json.RawMessage) (DeviceICEResult, error) {
	var wire dependencymodels.CameraDeviceICE

	err := json.Unmarshal(raw, &wire)
	if err != nil {
		return DeviceICEResult{}, cameraProtocol(string(dependencymodels.RPCCameraGetICE), err)
	}

	encoded, retry, err := cameraICEBatch(wire)
	if err != nil {
		return DeviceICEResult{}, cameraProtocol(string(dependencymodels.RPCCameraGetICE), err)
	}

	result := DeviceICEResult{Candidate: "", Candidates: make([]ICECandidate, 0, len(encoded)), Retry: retry}

	for _, item := range encoded {
		candidate, err := decodeCameraCandidate(item)
		if err != nil {
			return DeviceICEResult{}, cameraProtocol(string(dependencymodels.RPCCameraGetICE), err)
		}

		result.Candidates = append(result.Candidates, candidate)
	}

	if len(result.Candidates) > 0 {
		result.Candidate = result.Candidates[0].Candidate
	}

	return result, nil
}

func cameraICEBatch(wire dependencymodels.CameraDeviceICE) ([]string, bool, error) {
	if wire.DevIce != nil {
		var batch []string

		err := json.Unmarshal(*wire.DevIce, &batch)
		if err == nil {
			return batch, false, nil
		}

		var value string

		err = json.Unmarshal(*wire.DevIce, &value)
		if err != nil {
			return nil, false, fmt.Errorf("decode camera candidate batch: %w", err)
		}

		if value == string(dependencymodels.CameraSDPRetryPending) {
			return nil, true, nil
		}

		return []string{value}, false, nil
	}

	if wire.Ice != nil {
		return []string{*wire.Ice}, false, nil
	}

	return nil, false, errResultShape
}

func decodeCameraCandidate(encoded string) (ICECandidate, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return ICECandidate{}, fmt.Errorf("decode camera candidate encoding: %w", err)
	}

	var candidate dependencymodels.CameraICECandidate

	err = json.Unmarshal(data, &candidate)
	if err != nil {
		// Retain compatibility with the scalar, raw-candidate response accepted by earlier SDK versions.
		if strings.HasPrefix(string(data), "candidate:") {
			return ICECandidate{Candidate: string(data), SdpMid: nil, SdpMLineIndex: nil}, nil
		}

		return ICECandidate{}, fmt.Errorf("decode camera candidate envelope: %w", err)
	}

	if candidate.Candidate == "" {
		return ICECandidate{}, errResultShape
	}

	return ICECandidate{
		Candidate: candidate.Candidate, SdpMid: candidate.SdpMid, SdpMLineIndex: candidate.SdpMLineIndex,
	}, nil
}
