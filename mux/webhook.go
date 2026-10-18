package mux

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

// SignatureHeader carries Mux's webhook signature: "t=<unix>,v1=<hex hmac>".
const SignatureHeader = "Mux-Signature"

// WebhookTolerance is how old a signed webhook may be before it is refused.
const WebhookTolerance = 5 * time.Minute

var (
	ErrBadSignature = errors.New("mux webhook signature invalid")
	ErrStaleWebhook = errors.New("mux webhook timestamp outside tolerance")
)

// VerifyWebhook checks a webhook body against its Mux-Signature header: an
// HMAC-SHA256 of "<t>.<body>" with the webhook signing secret, at most
// WebhookTolerance old.
func VerifyWebhook(secret, header string, body []byte, now time.Time) error {
	if secret == "" {
		return ErrNotConfigured
	}

	var ts string
	var sigs []string
	for _, part := range strings.Split(header, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch k {
		case "t":
			ts = v
		case "v1":
			sigs = append(sigs, v)
		}
	}
	if ts == "" || len(sigs) == 0 {
		return ErrBadSignature
	}

	unix, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return ErrBadSignature
	}
	age := now.Sub(time.Unix(unix, 0))
	if age > WebhookTolerance || age < -WebhookTolerance {
		return ErrStaleWebhook
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "."))
	mac.Write(body)
	want := mac.Sum(nil)

	for _, s := range sigs {
		got, err := hex.DecodeString(s)
		if err == nil && hmac.Equal(got, want) {
			return nil
		}
	}
	return ErrBadSignature
}

// Event is a webhook delivery. Data is the object the event is about (an
// asset, upload or track) and is decoded according to Type.
type Event struct {
	ID     string          `json:"id"`
	Type   string          `json:"type"`
	Object EventObject     `json:"object"`
	Data   json.RawMessage `json:"data"`
}

// EventObject names the resource an event concerns.
type EventObject struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}
