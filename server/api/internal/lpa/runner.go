package lpa

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultTimeout          = 10 * time.Minute
	windowsUnsupportedError = "eSIM profile download is not supported on Windows; run the API on Linux or Docker"
)

type Runner struct {
	transport Transport
	updater   TaskUpdater
	binary    string
	timeout   time.Duration
	mu        sync.Mutex
	active    map[string]struct{}
}

type stdioMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type lpaPayload struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type progressPayload struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func NewRunner(transport Transport, updater TaskUpdater) *Runner {
	return &Runner{transport: transport, updater: updater, binary: resolveBinary(), timeout: defaultTimeout, active: make(map[string]struct{})}
}

func resolveBinary() string {
	if configured := strings.TrimSpace(os.Getenv("LPAC_PATH")); configured != "" {
		if absolute, err := filepath.Abs(configured); err == nil {
			return absolute
		}
		return configured
	}
	binaryName := "lpac"
	if runtime.GOOS == "windows" {
		binaryName = "lpac.exe"
	}
	if workingDirectory, err := os.Getwd(); err == nil {
		current := workingDirectory
		for {
			for _, candidate := range []string{
				filepath.Join(current, "tools", "lpac", binaryName),
				filepath.Join(current, "server", "tools", "lpac", binaryName),
			} {
				if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
					if absolute, absErr := filepath.Abs(candidate); absErr == nil {
						return absolute
					}
					return candidate
				}
			}
			parent := filepath.Dir(current)
			if parent == current {
				break
			}
			current = parent
		}
	}
	if found, err := exec.LookPath(binaryName); err == nil {
		return found
	}
	return binaryName
}

func (r *Runner) Available() error {
	if err := platformSupportError(runtime.GOOS); err != nil {
		return err
	}
	if r.transport == nil {
		return errors.New("APDU transport is unavailable")
	}
	_, err := exec.LookPath(r.binary)
	if err != nil {
		return fmt.Errorf("lpac executable not found (%s): %w", r.binary, err)
	}
	return nil
}

func platformSupportError(goos string) error {
	if goos == "windows" {
		return errors.New(windowsUnsupportedError)
	}
	return nil
}

func (r *Runner) Start(taskID, deviceID, activationCode, confirmationCode string) error {
	if err := r.Available(); err != nil {
		return err
	}
	r.mu.Lock()
	if _, exists := r.active[deviceID]; exists {
		r.mu.Unlock()
		return errors.New("another eSIM download is already running on this terminal")
	}
	r.active[deviceID] = struct{}{}
	r.mu.Unlock()
	go func() {
		defer func() {
			r.mu.Lock()
			delete(r.active, deviceID)
			r.mu.Unlock()
		}()
		r.run(taskID, deviceID, activationCode, confirmationCode)
	}()
	return nil
}

func (r *Runner) run(taskID, deviceID, activationCode, confirmationCode string) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	_ = r.updater.UpdateEsimTask(taskID, "running", "正在启动 LPA", 2)

	args := []string{"profile", "download", "-a", activationCode}
	if strings.TrimSpace(confirmationCode) != "" {
		args = append(args, "-c", strings.TrimSpace(confirmationCode))
	}
	cmd := exec.CommandContext(ctx, r.binary, args...)
	pathValue := os.Getenv("PATH")
	binaryDirectory := filepath.Dir(r.binary)
	if binaryDirectory != "." && binaryDirectory != "" {
		pathValue = binaryDirectory + string(os.PathListSeparator) + pathValue
	}
	cmd.Env = environmentWithOverrides(os.Environ(), map[string]string{
		"PATH": pathValue, "LPAC_APDU": "stdio", "LPAC_HTTP": httpBackend(),
	})
	stdin, err := cmd.StdinPipe()
	if err != nil {
		r.fail(taskID, err)
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		r.fail(taskID, err)
		return
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		r.fail(taskID, err)
		return
	}

	finalCode := -1
	finalMessage := "lpac exited without a final result"
	lastExchangeError := ""
	pipeFailed := false
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 4096), 2*1024*1024)
	for scanner.Scan() {
		var message stdioMessage
		if json.Unmarshal(scanner.Bytes(), &message) != nil {
			continue
		}
		switch message.Type {
		case "apdu":
			var request APDURequest
			if json.Unmarshal(message.Payload, &request) != nil {
				continue
			}
			response, exchangeErr := r.transport.Exchange(ctx, deviceID, request)
			if exchangeErr != nil {
				lastExchangeError = exchangeErr.Error()
				response = APDUResponse{ECode: -1, Error: exchangeErr.Error()}
			}
			if err := writeAPDUResponse(stdin, response); err != nil {
				finalMessage = err.Error()
				pipeFailed = true
				if cmd.Process != nil {
					_ = cmd.Process.Kill()
				}
			}
		case "progress":
			var progress progressPayload
			if json.Unmarshal(message.Payload, &progress) == nil {
				stage, percent := taskProgress(progress.Message)
				_ = r.updater.UpdateEsimTask(taskID, "running", stage, percent)
			}
		case "lpa":
			var result lpaPayload
			if json.Unmarshal(message.Payload, &result) == nil {
				finalCode, finalMessage = result.Code, result.Message
			}
		}
		if pipeFailed {
			break
		}
	}
	_ = stdin.Close()
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		r.fail(taskID, fmt.Errorf("LPA download timed out: %w", ctx.Err()))
		return
	}
	if scanErr := scanner.Err(); scanErr != nil {
		r.fail(taskID, scanErr)
		return
	}
	if waitErr != nil || finalCode != 0 {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" && finalMessage != "" && finalMessage != "lpac exited without a final result" {
			detail = finalMessage
		}
		if detail == "" && lastExchangeError != "" {
			detail = lastExchangeError
		}
		if detail == "" && pipeFailed {
			detail = "lpac closed the APDU pipe before the operation completed"
		}
		if detail == "" {
			detail = "lpac exited without a final result"
		}
		if waitErr != nil && !pipeFailed {
			detail = fmt.Sprintf("%s: %v", detail, waitErr)
		}
		r.fail(taskID, errors.New(detail))
		return
	}
	_ = r.updater.UpdateEsimTask(taskID, "succeeded", "Profile 下载并安装完成", 100)
}

func writeAPDUResponse(w io.Writer, response APDUResponse) error {
	payload := map[string]interface{}{"ecode": response.ECode}
	if response.Data != "" {
		payload["data"] = response.Data
	}
	message := map[string]interface{}{"type": "apdu", "payload": payload}
	encoded, err := json.Marshal(message)
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	_, err = w.Write(encoded)
	return err
}

func (r *Runner) fail(taskID string, err error) {
	message := strings.TrimSpace(err.Error())
	if len(message) > 300 {
		message = message[:300]
	}
	_ = r.updater.UpdateEsimTask(taskID, "failed", "下载失败："+message, 0)
}

func environmentWithOverrides(base []string, overrides map[string]string) []string {
	result := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		separator := strings.IndexByte(entry, '=')
		if separator <= 0 {
			result = append(result, entry)
			continue
		}
		key := entry[:separator]
		remove := false
		for overrideKey := range overrides {
			if strings.EqualFold(key, overrideKey) {
				remove = true
				break
			}
		}
		if !remove {
			result = append(result, entry)
		}
	}
	for key, value := range overrides {
		result = append(result, key+"="+value)
	}
	return result
}

func httpBackend() string {
	if configured := strings.TrimSpace(os.Getenv("LPAC_HTTP")); configured != "" {
		return configured
	}
	return "curl"
}

func taskProgress(message string) (string, int) {
	stages := map[string]struct {
		label   string
		percent int
	}{
		"es10b_get_euicc_challenge_and_info": {"读取 eUICC 认证信息", 8},
		"es9p_initiate_authentication":       {"与 SM-DP+ 建立认证会话", 18},
		"es10b_authenticate_server":          {"eUICC 验证 SM-DP+", 30},
		"es9p_authenticate_client":           {"SM-DP+ 验证 eUICC", 42},
		"es10b_prepare_download":             {"eUICC 准备安装 Profile", 55},
		"es9p_get_bound_profile_package":     {"下载 Bound Profile Package", 68},
		"es10b_load_bound_profile_package":   {"向 eUICC 安装 Profile", 82},
	}
	if stage, ok := stages[message]; ok {
		return stage.label, stage.percent
	}
	if step, err := strconv.Atoi(message); err == nil {
		return "正在处理 LPA 步骤", step
	}
	return firstNonEmpty(message, "正在执行 LPA"), 5
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
