package lpa

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

type fakeTransport struct {
	mu       sync.Mutex
	requests []APDURequest
}

func (f *fakeTransport) Exchange(_ context.Context, _ string, request APDURequest) (APDUResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, request)
	response := APDUResponse{ECode: 0}
	if request.Func == "logic_channel_open" {
		response.ECode = 1
	}
	if request.Func == "transmit" {
		response.Data = "9000"
	}
	return response, nil
}

type failingTransport struct {
	err error
}

func (f *failingTransport) Exchange(_ context.Context, _ string, _ APDURequest) (APDUResponse, error) {
	return APDUResponse{ECode: -1, Error: f.err.Error()}, f.err
}

type fakeUpdater struct {
	mu         sync.Mutex
	updates    []string
	progresses []int
	done       chan struct{}
}

func (f *fakeUpdater) UpdateEsimTask(_ string, status, stage string, progress int) error {
	f.mu.Lock()
	f.updates = append(f.updates, status+":"+stage)
	f.progresses = append(f.progresses, progress)
	f.mu.Unlock()
	if status == "succeeded" || status == "failed" {
		select {
		case <-f.done:
		default:
			close(f.done)
		}
	}
	return nil
}

func TestLPAResultDetail(t *testing.T) {
	if got := lpaResultDetail([]byte(`"EID doesn't match the expected value"`)); got != "EID doesn't match the expected value" {
		t.Fatalf("lpaResultDetail() = %q", got)
	}
	for _, value := range []json.RawMessage{nil, []byte(`null`), []byte(`{"iccid":"sensitive"}`)} {
		if got := lpaResultDetail(value); got != "" {
			t.Fatalf("lpaResultDetail(%s) = %q, want empty", value, got)
		}
	}
}

func TestRunnerReportsLPAErrorDetailWithoutExitStatus(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test fixture uses a POSIX shell script")
	}
	script := filepath.Join(t.TempDir(), "fake-lpac")
	contents := `#!/bin/sh
printf '%s\n' '{"type":"lpa","payload":{"code":-1,"message":"es9p_authenticate_client","data":"MatchingID is refused"}}'
exit 255
`
	if err := os.WriteFile(script, []byte(contents), 0700); err != nil {
		t.Fatal(err)
	}
	updater := &fakeUpdater{done: make(chan struct{})}
	runner := NewRunner(&fakeTransport{}, updater)
	runner.binary = script
	runner.timeout = 5 * time.Second
	if err := runner.Start("task-1", "device-1", "LPA:1$example$matching", ""); err != nil {
		t.Fatal(err)
	}
	select {
	case <-updater.done:
	case <-time.After(6 * time.Second):
		t.Fatal("runner did not finish")
	}
	updater.mu.Lock()
	defer updater.mu.Unlock()
	got := updater.updates[len(updater.updates)-1]
	if got != "failed:下载失败：es9p_authenticate_client：MatchingID is refused" {
		t.Fatalf("unexpected final update: %s; all=%v", got, updater.updates)
	}
	if progress := updater.progresses[len(updater.progresses)-1]; progress != 2 {
		t.Fatalf("final progress = %d, want 2", progress)
	}
}

func TestRunnerPreservesProgressOnAuthenticationFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test fixture uses a POSIX shell script")
	}
	script := filepath.Join(t.TempDir(), "fake-lpac")
	contents := `#!/bin/sh
printf '%s\n' '{"type":"progress","payload":{"code":0,"message":"es9p_authenticate_client"}}'
printf '%s\n' '{"type":"progress","payload":{"code":0,"message":"es10b_cancel_session"}}'
printf '%s\n' '{"type":"lpa","payload":{"code":-1,"message":"es9p_authenticate_client","data":"Not allowed"}}'
exit 1
`
	if err := os.WriteFile(script, []byte(contents), 0700); err != nil {
		t.Fatal(err)
	}
	updater := &fakeUpdater{done: make(chan struct{})}
	runner := NewRunner(&fakeTransport{}, updater)
	runner.binary = script
	runner.timeout = 5 * time.Second
	if err := runner.Start("task-1", "device-1", "LPA:1$example$matching", ""); err != nil {
		t.Fatal(err)
	}
	select {
	case <-updater.done:
	case <-time.After(6 * time.Second):
		t.Fatal("runner did not finish")
	}
	updater.mu.Lock()
	defer updater.mu.Unlock()
	if progress := updater.progresses[len(updater.progresses)-1]; progress != 42 {
		t.Fatalf("final progress = %d, want 42; all=%v", progress, updater.progresses)
	}
}

func TestRunnerReportsLPAErrorInsteadOfBrokenPipe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test fixture uses a POSIX shell script")
	}
	script := filepath.Join(t.TempDir(), "fake-lpac")
	contents := `#!/bin/sh
exec 0<&-
printf '%s\n' '{"type":"apdu","payload":{"func":"connect","param":null}}'
printf '%s\n' '{"type":"lpa","payload":{"code":-1,"message":"euicc_init","data":""}}'
sleep 0.1
exit 1
`
	if err := os.WriteFile(script, []byte(contents), 0700); err != nil {
		t.Fatal(err)
	}
	updater := &fakeUpdater{done: make(chan struct{})}
	runner := NewRunner(&fakeTransport{}, updater)
	runner.binary = script
	runner.timeout = 5 * time.Second
	if err := runner.Start("task-1", "device-1", "LPA:1$example$matching", ""); err != nil {
		t.Fatal(err)
	}
	select {
	case <-updater.done:
	case <-time.After(6 * time.Second):
		t.Fatal("runner did not finish")
	}
	updater.mu.Lock()
	defer updater.mu.Unlock()
	got := updater.updates[len(updater.updates)-1]
	if got != "failed:下载失败：euicc_init" {
		t.Fatalf("unexpected final update: %s; all=%v", got, updater.updates)
	}
}

func TestRunnerIncludesTerminalAPDUErrorWithEuiccInit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test fixture uses a POSIX shell script")
	}
	script := filepath.Join(t.TempDir(), "fake-lpac")
	contents := `#!/bin/sh
printf '%s\n' '{"type":"apdu","payload":{"func":"connect","param":null}}'
IFS= read -r response
printf '%s\n' '{"type":"lpa","payload":{"code":-1,"message":"euicc_init","data":""}}'
exit 1
`
	if err := os.WriteFile(script, []byte(contents), 0700); err != nil {
		t.Fatal(err)
	}
	updater := &fakeUpdater{done: make(chan struct{})}
	runner := NewRunner(&failingTransport{err: errors.New("eUICC initialization failed: logic channel open failed: +MATREADY")}, updater)
	runner.binary = script
	runner.timeout = 5 * time.Second
	if err := runner.Start("task-1", "device-1", "LPA:1$example$matching", ""); err != nil {
		t.Fatal(err)
	}
	select {
	case <-updater.done:
	case <-time.After(6 * time.Second):
		t.Fatal("runner did not finish")
	}
	updater.mu.Lock()
	defer updater.mu.Unlock()
	got := updater.updates[len(updater.updates)-1]
	want := "failed:下载失败：euicc_init；终端 APDU：eUICC initialization failed: logic channel open failed: +MATREADY"
	if got != want {
		t.Fatalf("unexpected final update: %s; want=%s; all=%v", got, want, updater.updates)
	}
}
func TestRunnerEndToEndWithStdioDriver(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test fixture uses a POSIX shell script")
	}
	script := filepath.Join(t.TempDir(), "fake-lpac")
	contents := `#!/bin/sh
request() {
  printf '%s\n' "$1"
  IFS= read -r response
}
request '{"type":"apdu","payload":{"func":"connect","param":null}}'
request '{"type":"apdu","payload":{"func":"logic_channel_open","param":"A0000005591010FFFFFFFF8900000100"}}'
printf '%s\n' '{"type":"progress","payload":{"code":0,"message":"es9p_initiate_authentication","data":"example"}}'
request '{"type":"apdu","payload":{"func":"transmit","param":"81E2910000"}}'
request '{"type":"apdu","payload":{"func":"logic_channel_close","param":"01"}}'
request '{"type":"apdu","payload":{"func":"disconnect","param":null}}'
printf '%s\n' '{"type":"lpa","payload":{"code":0,"message":"success","data":{}}}'
`
	if err := os.WriteFile(script, []byte(contents), 0700); err != nil {
		t.Fatal(err)
	}
	transport := &fakeTransport{}
	updater := &fakeUpdater{done: make(chan struct{})}
	runner := NewRunner(transport, updater)
	runner.binary = script
	runner.timeout = 5 * time.Second
	if err := runner.Start("task-1", "device-1", "LPA:1$example$matching", ""); err != nil {
		t.Fatal(err)
	}
	select {
	case <-updater.done:
	case <-time.After(6 * time.Second):
		t.Fatal("runner did not finish")
	}
	updater.mu.Lock()
	defer updater.mu.Unlock()
	if got := updater.updates[len(updater.updates)-1]; got != "succeeded:Profile 下载并安装完成" {
		t.Fatalf("unexpected final update: %s; all=%v", got, updater.updates)
	}
	transport.mu.Lock()
	defer transport.mu.Unlock()
	if len(transport.requests) != 5 {
		t.Fatalf("expected 5 APDU operations, got %d", len(transport.requests))
	}
}
