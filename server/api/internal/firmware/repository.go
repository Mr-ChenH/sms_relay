package firmware

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const MaxImageSize int64 = 1408 * 1024

var versionPattern = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._-]{0,31}$`)

type Image struct {
	HardwareModel string    `json:"hardwareModel"`
	Version       string    `json:"version"`
	Filename      string    `json:"filename"`
	Size          int64     `json:"size"`
	SHA256        string    `json:"sha256"`
	UploadedAt    time.Time `json:"uploadedAt"`
}

type token struct {
	DeviceID string
	Expires  time.Time
}

type Repository struct {
	dir    string
	mu     sync.Mutex
	tokens map[string]token
}

func NewRepository(dir string) (*Repository, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	return &Repository{dir: dir, tokens: make(map[string]token)}, nil
}

func (r *Repository) Current() (Image, bool) {
	raw, err := os.ReadFile(filepath.Join(r.dir, "current.json"))
	if err != nil {
		return Image{}, false
	}
	var image Image
	if json.Unmarshal(raw, &image) != nil || image.Filename == "" {
		return Image{}, false
	}
	if _, err := os.Stat(filepath.Join(r.dir, image.Filename)); err != nil {
		return Image{}, false
	}
	return image, true
}

func parseMetadata(path string) (string, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	const magic = "SMSHUBFW"
	index := bytes.Index(data, []byte(magic))
	if index < 0 || index+72 > len(data) || bytes.Index(data[index+8:], []byte(magic)) >= 0 {
		return "", "", errors.New("firmware does not contain unique SMS Hub version metadata")
	}
	readField := func(field []byte) string {
		if end := bytes.IndexByte(field, 0); end >= 0 {
			field = field[:end]
		}
		return strings.TrimSpace(string(field))
	}
	version := readField(data[index+8 : index+40])
	hardware := readField(data[index+40 : index+72])
	if !versionPattern.MatchString(version) || hardware == "" {
		return "", "", errors.New("firmware version metadata is invalid")
	}
	return version, hardware, nil
}

func (r *Repository) Save(src io.Reader) (Image, error) {
	name := "firmware-upload.bin"
	tmp, err := os.CreateTemp(r.dir, ".firmware-*.tmp")
	if err != nil {
		return Image{}, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(src, MaxImageSize+1))
	closeErr := tmp.Close()
	if copyErr != nil {
		return Image{}, copyErr
	}
	if closeErr != nil {
		return Image{}, closeErr
	}
	if written == 0 {
		return Image{}, errors.New("firmware image is empty")
	}
	if written > MaxImageSize {
		return Image{}, fmt.Errorf("firmware image exceeds OTA slot size (%d bytes)", MaxImageSize)
	}

	version, hardware, err := parseMetadata(tmpName)
	if err != nil {
		return Image{}, err
	}
	name = "firmware-" + version + ".bin"
	finalPath := filepath.Join(r.dir, name)
	if err := os.Rename(tmpName, finalPath); err != nil {
		return Image{}, err
	}
	image := Image{HardwareModel: hardware, Version: version, Filename: name, Size: written, SHA256: hex.EncodeToString(hash.Sum(nil)), UploadedAt: time.Now().UTC()}
	metadata, _ := json.MarshalIndent(image, "", "  ")
	metadataTmp := filepath.Join(r.dir, ".current.json.tmp")
	if err := os.WriteFile(metadataTmp, metadata, 0644); err != nil {
		return Image{}, err
	}
	if err := os.Rename(metadataTmp, filepath.Join(r.dir, "current.json")); err != nil {
		return Image{}, err
	}
	return image, nil
}

func (r *Repository) IssueToken(deviceID string, lifetime time.Duration) (string, time.Time, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", time.Time{}, err
	}
	expires := time.Now().Add(lifetime)
	encoded := hex.EncodeToString(value)
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, item := range r.tokens {
		if time.Now().After(item.Expires) {
			delete(r.tokens, key)
		}
	}
	r.tokens[encoded] = token{DeviceID: deviceID, Expires: expires}
	return encoded, expires, nil
}

func (r *Repository) Authorize(value, deviceID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.tokens[value]
	if !ok || item.DeviceID != deviceID || time.Now().After(item.Expires) {
		delete(r.tokens, value)
		return false
	}
	delete(r.tokens, value)
	return true
}

func (r *Repository) OpenCurrent() (Image, *os.File, error) {
	image, ok := r.Current()
	if !ok {
		return Image{}, nil, os.ErrNotExist
	}
	file, err := os.Open(filepath.Join(r.dir, image.Filename))
	return image, file, err
}
