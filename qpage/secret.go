package qpage

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"sync/atomic"
)

// ErrCursorKey means no secret was set: call SetSecret once at startup. It is
// a programming error (answer 500), not a bad request.
var ErrCursorKey = errors.New("qpage: cursor secret not set; call qpage.SetSecret at startup")

const (
	minSecret = 16
	macLen    = 16 // truncated HMAC-SHA256: 128 bits are plenty for a cursor
)

type secrets struct {
	current  []byte
	previous [][]byte
}

var secret atomic.Pointer[secrets]

// SetSecret sets the key that signs cursors. Call it once at startup, before
// the first request, as with qt.SetIDKey; the declarations of sorts may come
// earlier (package-level variables), because the secret is read when a cursor
// is made or checked.
//
// current signs every cursor. previous lists the secrets still accepted when
// checking, so a secret can be rotated without invalidating the cursors in
// flight: SetSecret(newSecret, oldSecret), then drop oldSecret a few days
// later. Every secret must be at least 16 bytes.
func SetSecret(current []byte, previous ...[]byte) error {
	if len(current) < minSecret {
		return errors.New("qpage: the secret must be at least 16 bytes")
	}
	s := &secrets{current: append([]byte(nil), current...)}
	for _, p := range previous {
		if len(p) < minSecret {
			return errors.New("qpage: a previous secret is shorter than 16 bytes")
		}
		s.previous = append(s.previous, append([]byte(nil), p...))
	}
	secret.Store(s)
	return nil
}

// CheckSecret returns ErrCursorKey when SetSecret has not been called. Call it
// at startup to fail at launch rather than at the first request. Window makes
// the same check on every request, the first page included, so a program that
// forgot the secret fails on its first list, not on its second page.
func CheckSecret() error {
	if secret.Load() == nil {
		return ErrCursorKey
	}
	return nil
}

func mac(key, msg []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(msg)
	return h.Sum(nil)[:macLen]
}

func sign(msg []byte) ([]byte, error) {
	s := secret.Load()
	if s == nil {
		return nil, ErrCursorKey
	}
	return mac(s.current, msg), nil
}

// verify reports whether tag signs msg under the current or a previous secret.
// Every secret is tried, so the time taken does not tell which one matched.
func verify(msg, tag []byte) (bool, error) {
	s := secret.Load()
	if s == nil {
		return false, ErrCursorKey
	}
	ok := hmac.Equal(tag, mac(s.current, msg))
	for _, p := range s.previous {
		ok = hmac.Equal(tag, mac(p, msg)) || ok
	}
	return ok, nil
}
