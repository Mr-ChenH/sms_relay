package main

import (
	"fmt"
	"os"
	"strings"
	"time"
	_ "time/tzdata"
)

func configureTimezone() (string, error) {
	name := strings.TrimSpace(os.Getenv("SMS_HUB_TIMEZONE"))
	if name == "" {
		name = strings.TrimSpace(os.Getenv("TZ"))
	}
	if name == "" {
		name = "Asia/Shanghai"
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return "", fmt.Errorf("invalid TZ %q: %w", name, err)
	}
	time.Local = location
	return name, nil
}
