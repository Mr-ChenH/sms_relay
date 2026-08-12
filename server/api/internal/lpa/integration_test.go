package lpa

import (
	"context"
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

type fakeUpdater struct {
	mu      sync.Mutex
	updates []string
	done    chan struct{}
}

func (f *fakeUpdater) UpdateEsimTask(_ string, status, stage string, _ int) error {
	f.mu.Lock()
	f.updates = append(f.updates, status+":"+stage)
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
