package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/portpowered/go-roborock/pkg/roborock"
)

const profileRandomBytes = 16

var errProfile = errors.New("no usable login profile; run go-roborock login")

func randomIdentity() (string, error) {
	data := make([]byte, profileRandomBytes)

	_, err := rand.Read(data)
	if err != nil {
		return "", fmt.Errorf("generate login identity: %w", err)
	}

	return hex.EncodeToString(data), nil
}

func defaultProfile() (string, error) {
	directory, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find profile directory: %w", err)
	}

	return filepath.Join(directory, "go-roborock", "profile.json"), nil
}

func readProfile(path string) (roborock.AuthContext, error) {
	var value CredentialExport

	err := checkPrivateProfileFile(path)
	if err != nil {
		return roborock.AuthContext{}, errors.Join(errProfile, err)
	}
	//nolint:gosec // Caller-selected profile is checked for private regular-file ownership before opening.
	file, err := os.Open(path)
	if err != nil {
		return roborock.AuthContext{}, fmt.Errorf("open login profile: %w", err)
	}

	defer func() { _ = file.Close() }()

	decoder := json.NewDecoder(io.LimitReader(file, maxInputBytes+1))
	decoder.DisallowUnknownFields()

	err = decoder.Decode(&value)
	if err != nil || value.Auth == nil || value.Auth.Token == "" || value.Auth.ClientID == "" || value.Auth.BaseURL == "" {
		return roborock.AuthContext{}, errProfile
	}

	var extra json.RawMessage
	if !errors.Is(decoder.Decode(&extra), io.EOF) {
		return roborock.AuthContext{}, errProfile
	}

	return *value.Auth, nil
}

func saveProfile(path string, auth roborock.AuthContext) error {
	err := ensurePrivateProfileDir(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("prepare login profile: %w", err)
	}

	_, statErr := os.Lstat(path)
	if statErr == nil {
		err = checkPrivateProfileFile(path)
		if err != nil {
			return fmt.Errorf("check existing profile: %w", err)
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("check login profile: %w", statErr)
	}

	identity, err := randomIdentity()
	if err != nil {
		return err
	}

	temporary := filepath.Join(filepath.Dir(path), ".profile-"+identity)

	file, err := createPrivateProfileFile(temporary)
	if err != nil {
		return fmt.Errorf("create login profile: %w", err)
	}

	defer func() { _ = os.Remove(temporary) }()

	err = json.NewEncoder(file).Encode(CredentialExport{Auth: &auth, Login: nil})
	if err == nil {
		err = file.Sync()
	}

	err = errors.Join(err, file.Close())
	if err != nil {
		return fmt.Errorf("save login profile: %w", err)
	}

	err = os.Rename(temporary, path)
	if err != nil {
		return fmt.Errorf("replace login profile: %w", err)
	}

	return nil
}
