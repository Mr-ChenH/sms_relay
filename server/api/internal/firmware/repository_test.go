package firmware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"
)

func TestRepositorySaveAndAuthorize(t *testing.T) {
	repo, err := NewRepository(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	payload := append([]byte{0xe9, 1, 2, 3, 4}, firmwareMetadata("1.2.3", "ESP32-C3 + ML307A")...)
	image, err := repo.Save(bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	if image.Size != int64(len(payload)) || image.SHA256 != hex.EncodeToString(sum[:]) || image.HardwareModel != "ESP32-C3 + ML307A" {
		t.Fatalf("unexpected image: %+v", image)
	}
	current, ok := repo.Current()
	if !ok || current.Version != "1.2.3" {
		t.Fatalf("current image: %+v, %t", current, ok)
	}
	token, _, err := repo.IssueToken("terminal-1", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !repo.Authorize(token, "terminal-1") {
		t.Fatal("expected matching terminal token to be authorized")
	}
	if repo.Authorize(token, "terminal-1") {
		t.Fatal("firmware token must be single use")
	}
	token, _, err = repo.IssueToken("terminal-1", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if repo.Authorize(token, "terminal-2") {
		t.Fatal("token must be bound to a terminal")
	}
}

func TestRepositoryRejectsInvalidInput(t *testing.T) {
	repo, err := NewRepository(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Save(bytes.NewReader([]byte{1})); err == nil {
		t.Fatal("expected image without version metadata to be rejected")
	}
	oversized := append(firmwareMetadata("1.0.0", "ESP32-C3 + ML307A"), make([]byte, MaxImageSize+1)...)
	if _, err := repo.Save(bytes.NewReader(oversized)); err == nil {
		t.Fatal("expected oversized image to be rejected")
	}
}

func firmwareMetadata(version, hardware string) []byte {
	metadata := make([]byte, 72)
	copy(metadata, []byte("SMSHUBFW"))
	copy(metadata[8:40], []byte(version))
	copy(metadata[40:72], []byte(hardware))
	return metadata
}
