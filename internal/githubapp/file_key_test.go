package githubapp

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileKeySourceLoadsRelativeRSAKey(t *testing.T) {
	configDir := t.TempDir()
	keyDir := filepath.Join(configDir, "og")
	if err := os.Mkdir(keyDir, 0o700); err != nil {
		t.Fatalf("mkdir key dir: %v", err)
	}
	keyPath := filepath.Join(keyDir, "github-app.pem")
	writeRSAKey(t, keyPath, 0o600)

	source, err := NewKeySource(Config{KeySource: "file", KeyRef: "og/github-app.pem"}, configDir)
	if err != nil {
		t.Fatalf("NewKeySource: %v", err)
	}
	key, err := source.PrivateKey()
	if err != nil {
		t.Fatalf("PrivateKey: %v", err)
	}
	if key.N.BitLen() < 2048 {
		t.Fatalf("RSA key size = %d", key.N.BitLen())
	}
}

func TestFileKeySourceRejectsUnsafeFiles(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*testing.T, string, string)
		wantErr string
	}{
		{
			name: "loose parent mode",
			prepare: func(t *testing.T, dir, path string) {
				writeRSAKey(t, path, 0o600)
				if err := os.Chmod(dir, 0o755); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "0700",
		},
		{
			name:    "loose key mode",
			prepare: func(t *testing.T, _ string, path string) { writeRSAKey(t, path, 0o644) },
			wantErr: "permissions",
		},
		{
			name: "symlink",
			prepare: func(t *testing.T, dir, path string) {
				target := filepath.Join(dir, "target.pem")
				writeRSAKey(t, target, 0o600)
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "symlink",
		},
		{
			name: "oversized",
			prepare: func(t *testing.T, _ string, path string) {
				if err := os.WriteFile(path, make([]byte, maxPrivateKeyBytes+1), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "too large",
		},
		{
			name: "invalid PEM does not leak contents",
			prepare: func(t *testing.T, _ string, path string) {
				if err := os.WriteFile(path, []byte("PRIVATE-TEST-SECRET"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "PEM",
		},
		{
			name: "non RSA key",
			prepare: func(t *testing.T, _ string, path string) {
				_, key, err := ed25519.GenerateKey(rand.Reader)
				if err != nil {
					t.Fatal(err)
				}
				der, err := x509.MarshalPKCS8PrivateKey(key)
				if err != nil {
					t.Fatal(err)
				}
				writePEM(t, path, "PRIVATE KEY", der, 0o600)
			},
			wantErr: "RSA",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configDir := t.TempDir()
			keyDir := filepath.Join(configDir, "og")
			if err := os.Mkdir(keyDir, 0o700); err != nil {
				t.Fatal(err)
			}
			keyPath := filepath.Join(keyDir, "key.pem")
			tt.prepare(t, keyDir, keyPath)
			source, err := NewKeySource(Config{KeySource: "file", KeyRef: "og/key.pem"}, configDir)
			if err == nil {
				_, err = source.PrivateKey()
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("key error = %v, want containing %q", err, tt.wantErr)
			}
			if strings.Contains(err.Error(), "PRIVATE-TEST-SECRET") {
				t.Fatalf("error leaked key contents: %v", err)
			}
		})
	}
}

func TestFileKeySourceRejectsKeyOwnedByAnotherUser(t *testing.T) {
	configDir := t.TempDir()
	keyDir := filepath.Join(configDir, "og")
	if err := os.Mkdir(keyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(keyDir, "key.pem")
	writeRSAKey(t, keyPath, 0o600)
	source := newFileKeySource(configDir, "og/key.pem")
	source.currentUID = os.Getuid() + 1

	_, err := source.PrivateKey()
	if err == nil || !strings.Contains(err.Error(), "current user") {
		t.Fatalf("PrivateKey error = %v", err)
	}
}

func TestFileKeySourceLoadsPKCS8RSAKey(t *testing.T) {
	configDir := t.TempDir()
	keyDir := filepath.Join(configDir, "og")
	if err := os.Mkdir(keyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	writePEM(t, filepath.Join(keyDir, "key.pem"), "PRIVATE KEY", der, 0o600)
	source := newFileKeySource(configDir, "og/key.pem")
	if _, err := source.PrivateKey(); err != nil {
		t.Fatalf("PrivateKey: %v", err)
	}
}

func writeRSAKey(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	writePEM(t, path, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(key), mode)
}

func writePEM(t *testing.T, path, blockType string, der []byte, mode os.FileMode) {
	t.Helper()
	data := pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der})
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatalf("write PEM: %v", err)
	}
}
