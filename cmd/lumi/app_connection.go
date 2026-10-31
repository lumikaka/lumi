package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Tickets authorize no REST reads or writes, just one project's WS hints.
// A fresh instance nonce invalidates every ticket after a CLI restart.
func appConnection(identity instance, projectUUID string, now time.Time) any {
	expires := now.Add(time.Hour).Unix()
	payload := projectUUID + ":" + strconv.FormatInt(expires, 10)
	mac := hmac.New(sha256.New, []byte(identity.Nonce))
	_, _ = mac.Write([]byte(payload))
	ticket := base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return map[string]any{"project_uuid": projectUUID, "websocket_url": "ws" + strings.TrimPrefix(identity.Endpoint, "http") + "/api/v1/ws?app_ticket=" + url.QueryEscape(ticket), "expires_at": time.Unix(expires, 0).UTC().Format(time.RFC3339)}
}

func verifyAppTicket(secret, ticket string, now time.Time) (string, time.Time, error) {
	bad := fmt.Errorf("invalid or expired App ticket")
	parts := strings.Split(ticket, ".")
	if len(parts) != 2 {
		return "", time.Time{}, bad
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", time.Time{}, bad
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", time.Time{}, bad
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payload)
	if !hmac.Equal(mac.Sum(nil), sig) {
		return "", time.Time{}, bad
	}
	values := strings.Split(string(payload), ":")
	if len(values) != 2 {
		return "", time.Time{}, bad
	}
	parsed, err := uuid.Parse(values[0])
	if err != nil || parsed.Version() != 7 {
		return "", time.Time{}, bad
	}
	unix, err := strconv.ParseInt(values[1], 10, 64)
	expires := time.Unix(unix, 0)
	if err != nil || !expires.After(now) || expires.After(now.Add(time.Hour)) {
		return "", time.Time{}, bad
	}
	return values[0], expires, nil
}
