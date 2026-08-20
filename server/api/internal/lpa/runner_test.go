package lpa

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestWriteAPDUResponse(t *testing.T) {
	var output bytes.Buffer
	if err := writeAPDUResponse(&output, APDUResponse{ECode: 0, Data: "9000"}); err != nil {
		t.Fatal(err)
	}
	var message struct {
		Type    string `json:"type"`
		Payload struct {
			ECode int    `json:"ecode"`
			Data  string `json:"data"`
			Error string `json:"error"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(output.Bytes(), &message); err != nil {
		t.Fatal(err)
	}
	if message.Type != "apdu" || message.Payload.ECode != 0 || message.Payload.Data != "9000" {
		t.Fatalf("unexpected response: %+v", message)
	}

	output.Reset()
	if err := writeAPDUResponse(&output, APDUResponse{ECode: -1, Error: "logic channel open failed"}); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(output.Bytes(), &message); err != nil {
		t.Fatal(err)
	}
	if message.Payload.ECode != -1 || message.Payload.Error != "logic channel open failed" {
		t.Fatalf("unexpected error response: %+v", message)
	}
}

func TestTaskProgress(t *testing.T) {
	stage, progress := taskProgress("es10b_load_bound_profile_package")
	if stage != "向 eUICC 安装 Profile" || progress != 82 {
		t.Fatalf("taskProgress() = %q, %d", stage, progress)
	}
	stage, progress = taskProgress("es9p_cancel_session")
	if stage != "正在清理失败会话" || progress != 5 {
		t.Fatalf("cancel taskProgress() = %q, %d", stage, progress)
	}
}

func TestPlatformSupportError(t *testing.T) {
	if err := platformSupportError("linux"); err != nil {
		t.Fatalf("linux should be supported: %v", err)
	}
	if err := platformSupportError("windows"); err == nil || err.Error() != windowsUnsupportedError {
		t.Fatalf("windows error = %v", err)
	}
}

func TestEnvironmentWithOverridesRemovesDuplicateKeys(t *testing.T) {
	environment := environmentWithOverrides(
		[]string{"Path=old", "LPAC_APDU=pcsc", "OTHER=value"},
		map[string]string{"PATH": "new", "LPAC_APDU": "stdio"},
	)
	counts := map[string]int{}
	values := map[string]string{}
	for _, entry := range environment {
		parts := bytes.SplitN([]byte(entry), []byte("="), 2)
		key := string(bytes.ToUpper(parts[0]))
		counts[key]++
		if len(parts) == 2 {
			values[key] = string(parts[1])
		}
	}
	if counts["PATH"] != 1 || values["PATH"] != "new" || counts["LPAC_APDU"] != 1 || values["LPAC_APDU"] != "stdio" {
		t.Fatalf("unexpected environment: %v", environment)
	}
}
