package publicapi

import (
	"encoding/base64"
	"encoding/json"
	"time"

	"aihot-server/internal/items"
)

// cursorPayload is the internal, NON-CONTRACT wire form of a cursor. The encoding
// is opaque to clients and may change without notice.
type cursorPayload struct {
	S int64  `json:"s"` // sort key, unix micro
	I string `json:"i"` // id
}

// encodeCursor turns a typed cursor into an opaque base64url token. Nil → "".
func encodeCursor(c *items.Cursor) string {
	if c == nil {
		return ""
	}
	b, err := json.Marshal(cursorPayload{S: c.SortKey.UnixMicro(), I: c.ID})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// decodeCursor is lenient: any malformed / empty / incomplete token returns nil,
// which the caller treats as "no cursor → first page" (per the opaque-token contract).
func decodeCursor(tok string) *items.Cursor {
	if tok == "" {
		return nil
	}
	b, err := base64.RawURLEncoding.DecodeString(tok)
	if err != nil {
		return nil
	}
	var p cursorPayload
	if err := json.Unmarshal(b, &p); err != nil {
		return nil
	}
	if p.I == "" {
		return nil
	}
	return &items.Cursor{SortKey: time.UnixMicro(p.S).UTC(), ID: p.I}
}
