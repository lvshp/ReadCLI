package core

import (
	"fmt"
	"strings"
	"time"
)

func setStatus(kind statusKind, message string) {
	if app == nil {
		return
	}
	message = strings.TrimSpace(message)
	app.statusMessage = message
	app.statusMessageKind = kind
	app.statusMessageGeneration++
	app.statusMessageUntil = statusExpiry(kind, message)
}

func setStatusf(kind statusKind, format string, args ...interface{}) {
	setStatus(kind, fmt.Sprintf(format, args...))
}

func clearStatus() {
	if app == nil {
		return
	}
	app.statusMessage = ""
	app.statusMessageKind = statusInfo
	app.statusMessageUntil = time.Time{}
	app.statusMessageGeneration++
}

func statusExpiry(kind statusKind, message string) time.Time {
	if message == "" {
		return time.Time{}
	}
	switch kind {
	case statusError:
		return time.Now().Add(8 * time.Second)
	case statusProgress, statusPersistent:
		return time.Time{}
	default:
		return time.Now().Add(3 * time.Second)
	}
}
