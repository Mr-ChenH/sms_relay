package httpapi

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"sms-forwarding/server/api/internal/notify"
	"sms-forwarding/server/api/internal/store"
)

func TestFirmwareUploadAndDownloadAuthorization(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SMS_HUB_FIRMWARE_DIR", filepath.Join(root, "firmware"))
	db, err := store.NewSQLiteStore(filepath.Join(root, "smshub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := New(db, notify.NewClient("")).Handler()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("version", "1.2.3"); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("firmware", "terminal.bin")
	if err != nil {
		t.Fatal(err)
	}
	image := append([]byte{0xe9, 1, 2, 3, 4}, testFirmwareMetadata("1.2.3", "ESP32-C3 + ML307A")...)
	if _, err := part.Write(image); err != nil {
		t.Fatal(err)
	}
	writer.Close()

	upload := httptest.NewRequest(http.MethodPost, "/api/admin/firmware", &body)
	upload.Header.Set("Content-Type", writer.FormDataContentType())
	uploadResult := httptest.NewRecorder()
	handler.ServeHTTP(uploadResult, upload)
	if uploadResult.Code != http.StatusCreated {
		t.Fatalf("upload status=%d body=%s", uploadResult.Code, uploadResult.Body.String())
	}
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Version string `json:"version"`
			Size    int64  `json:"size"`
		} `json:"data"`
	}
	if err := json.Unmarshal(uploadResult.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Success || response.Data.Version != "1.2.3" || response.Data.Size != int64(len(image)) {
		t.Fatalf("unexpected upload response: %+v", response)
	}

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/firmware/download?deviceId=x&token=bad", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("download status=%d, want 401", unauthorized.Code)
	}
	if _, err := os.Stat(filepath.Join(root, "firmware", "firmware-1.2.3.bin")); err != nil {
		t.Fatal(err)
	}
}

func testFirmwareMetadata(version, hardware string) []byte {
	metadata := make([]byte, 72)
	copy(metadata, []byte("SMSHUBFW"))
	copy(metadata[8:40], []byte(version))
	copy(metadata[40:72], []byte(hardware))
	return metadata
}
