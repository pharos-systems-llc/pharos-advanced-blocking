package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

var (
	// ErrCredentialsNotFound is returned when the credentials file does not exist.
	ErrCredentialsNotFound = errors.New("credentials file does not exist")

	// ErrWeakerPermissions is returned when the credentials file permissions are weaker than 0600 on Unix systems.
	ErrWeakerPermissions = errors.New("security validation failed: credentials file permissions are too weak (must be strictly 0600, user read/write only)")
)

// VerifyCredentialsFile checks if the credentials file exists at the specified path.
// If it exists, it verifies that the file permissions on Unix systems are strictly 0600
// (Unix user read/write only, meaning group and other permissions are completely disabled, file mode & 0077 must be 0000).
// If the file does not exist, it returns ErrCredentialsNotFound.
func VerifyCredentialsFile(filePath string) error {
	fi, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrCredentialsNotFound
		}
		return err
	}

	if fi.IsDir() {
		return errors.New("credentials path is a directory, not a file")
	}

	// Permissions check on Unix systems
	if runtime.GOOS != "windows" {
		mode := fi.Mode().Perm()
		if mode&0077 != 0 {
			return ErrWeakerPermissions
		}
	}

	return nil
}

// WriteSecretsFile writes raw secrets.json content to filePath with permissions set
// to strictly 0600 (user read/write only) at creation time, rather than writing with
// broader default permissions and chmod'ing afterward. The parent directory is
// created (mode 0700) if it does not already exist.
//
// The caller is responsible for marshaling the secrets payload (e.g. the commands
// package's SecretsConfig) to JSON bytes before calling this helper, since this
// package has no dependency on that type.
func WriteSecretsFile(filePath string, data []byte) error {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory %q: %w", dir, err)
	}

	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to open secrets file %q for writing: %w", filePath, err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("failed to write secrets file %q: %w", filePath, err)
	}

	// Belt-and-suspenders: ensure permissions are exactly 0600 even if an existing
	// file with looser permissions was reused via O_TRUNC.
	if runtime.GOOS != "windows" {
		if err := os.Chmod(filePath, 0600); err != nil {
			return fmt.Errorf("failed to set permissions on secrets file %q: %w", filePath, err)
		}
	}

	return nil
}
