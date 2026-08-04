package githubapp

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

const maxPrivateKeyBytes = 1024 * 1024

type fileKeySource struct {
	path       string
	currentUID int
}

func newFileKeySource(configDir, keyRef string) *fileKeySource {
	path := keyRef
	if !filepath.IsAbs(path) {
		path = filepath.Join(configDir, path)
	}
	return &fileKeySource{path: filepath.Clean(path), currentUID: os.Getuid()}
}

func (s *fileKeySource) PrivateKey() (*rsa.PrivateKey, error) {
	if err := validateKeyDirectory(filepath.Dir(s.path)); err != nil {
		return nil, err
	}
	info, err := validateKeyFile(s.path, s.currentUID)
	if err != nil {
		return nil, err
	}
	data, err := readUnchangedFile(s.path, info)
	if err != nil {
		return nil, err
	}
	return parseRSAKey(data, s.path)
}

func validateKeyDirectory(parent string) error {
	parentInfo, err := os.Lstat(parent)
	if err != nil {
		return fmt.Errorf("inspect GitHub App key directory %s: %w", parent, err)
	}
	if !parentInfo.IsDir() || parentInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("GitHub App key directory %s must be a regular directory", parent)
	}
	if parentInfo.Mode().Perm() != 0o700 {
		return fmt.Errorf("GitHub App key directory %s must have mode 0700", parent)
	}
	return nil
}

func validateKeyFile(path string, currentUID int) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect GitHub App private key %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("GitHub App private key %s must not be a symlink", path)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("GitHub App private key %s must be a regular file", path)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("GitHub App private key %s permissions must not allow group or other access", path)
	}
	if err := requireOwner(info, path, currentUID); err != nil {
		return nil, err
	}
	if info.Size() > maxPrivateKeyBytes {
		return nil, fmt.Errorf("GitHub App private key %s is too large", path)
	}
	return info, nil
}

func readUnchangedFile(path string, expectedInfo os.FileInfo) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open GitHub App private key %s: %w", path, err)
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect open GitHub App private key %s: %w", path, err)
	}
	if !os.SameFile(expectedInfo, openedInfo) {
		return nil, fmt.Errorf("GitHub App private key %s changed while opening", path)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxPrivateKeyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read GitHub App private key %s: %w", path, err)
	}
	if len(data) > maxPrivateKeyBytes {
		return nil, fmt.Errorf("GitHub App private key %s is too large", path)
	}
	return data, nil
}

func requireOwner(info os.FileInfo, path string, expectedUID int) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("cannot verify owner of GitHub App key path %s", path)
	}
	if int(stat.Uid) != expectedUID {
		return fmt.Errorf("GitHub App key path %s must be owned by the current user", path)
	}
	return nil
}

func parseRSAKey(data []byte, path string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("GitHub App private key %s does not contain valid PEM", path)
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("GitHub App private key %s contains invalid private-key PEM", path)
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("GitHub App private key %s must contain an RSA key", path)
	}
	return rsaKey, nil
}
