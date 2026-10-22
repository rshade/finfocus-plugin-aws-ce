package pricing

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
)

const carbonTokenLimit = 16 * 1024

var errCarbonCursor = errors.New("invalid or expired carbon continuation token")

type carbonCursor struct {
	Query   string   `json:"q"`
	Caller  string   `json:"c"`
	Token   string   `json:"t"`
	Pages   int      `json:"p"`
	Expires int64    `json:"e"`
	Seen    []string `json:"s"`
}

func carbonDigest(value []byte) string { d := sha256.Sum256(value); return hex.EncodeToString(d[:]) }
func (c *Calculator) signCarbon(s carbonCursor) (string, error) {
	body, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, []byte(c.carbonKey))
	mac.Write(body)
	token := base64.RawURLEncoding.EncodeToString(body) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if len(token) > carbonTokenLimit {
		return "", errCarbonCursor
	}
	return token, nil
}
func (c *Calculator) readCarbon(token, query, caller string) (carbonCursor, error) {
	s, err := c.readCarbonQuery(token, query)
	if err != nil || s.Caller != caller {
		return carbonCursor{}, errCarbonCursor
	}
	return s, nil
}

// readCarbonQuery authenticates continuation state before any credential lookup.
// Caller binding is checked separately once the effective base keys are resolved.
func (c *Calculator) readCarbonQuery(token, query string) (carbonCursor, error) {
	invalid := carbonCursor{}
	if len(token) > carbonTokenLimit {
		return invalid, errCarbonCursor
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return invalid, errCarbonCursor
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return invalid, errCarbonCursor
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return invalid, errCarbonCursor
	}
	mac := hmac.New(sha256.New, []byte(c.carbonKey))
	mac.Write(body)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return invalid, errCarbonCursor
	}
	var s carbonCursor
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if dec.Decode(&s) != nil || dec.Decode(new(any)) != io.EOF {
		return invalid, errCarbonCursor
	}
	if s.Query != query || s.Expires <= c.currentTime().Unix() || s.Expires > c.currentTime().Add(15*time.Minute).Unix() || s.Pages < 1 || s.Pages >= 100 || len(s.Seen) != s.Pages || len(s.Token) == 0 || len(s.Token) > 2000 {
		return invalid, errCarbonCursor
	}
	seen := map[string]bool{}
	for _, v := range s.Seen {
		if len(v) != 64 || seen[v] {
			return invalid, errCarbonCursor
		}
		seen[v] = true
	}
	if !seen[carbonDigest([]byte(s.Token))] {
		return invalid, errCarbonCursor
	}
	return s, nil
}
