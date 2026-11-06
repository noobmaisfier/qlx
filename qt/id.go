package qt

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"math/bits"
	"strconv"
	"sync/atomic"
)

// ID is the id of a row, kept as an integer in the database and shown as a
// short opaque string everywhere else: JSON, URLs, logs, fmt.
//
//	var id db.UserID           // = qt.ID[db.UsersIDSpace]
//	b, _ := json.Marshal(id)   // "7mQp3VxK9cE"
//
// The generator (qlx.yaml: `ids: string`) types every single-column integer
// primary key, and every foreign key that points at one, with the ID of its
// table. The database column does not change: joins, indexes and ordering all
// keep working on the integers, and only the Go side sees strings. A value of
// one table's ID cannot be used where another table's is expected; the
// compiler says so.
//
// The text form is a keyed permutation, not a cipher: it hides the numbering
// (no "how many users are there", no walking id+1) and rejects mistyped or
// forged strings, but it is obfuscation. Keep authorisation checks. See
// SetIDKey for the key and its stability rules.
//
// Domain: 0 <= n < 2^48 (281 trillion). Identity columns never get near it.
// The text is always 11 characters of the base58 alphabet.
type ID[S IDSpace] struct{ n int64 }

// IDSpace is implemented by the marker type of an ID: it names the table the
// ID belongs to. Each space gets its own derived key, so the same number
// encodes differently in different tables.
type IDSpace interface{ IDSpace() string }

// IDOf wraps an integer. It does not check the domain; encoding does.
func IDOf[S IDSpace](n int64) ID[S] { return ID[S]{n: n} }

// Int64 is the integer stored in the database.
func (i ID[S]) Int64() int64 { return i.n }

// IsZero reports whether the id is 0 (the zero value, "no id yet").
func (i ID[S]) IsZero() bool { return i.n == 0 }

// ParseID decodes the text form.
func ParseID[S IDSpace](s string) (ID[S], error) {
	var i ID[S]
	err := i.UnmarshalText([]byte(s))
	return i, err
}

// String is the text form. It panics when no key is set or the integer is
// outside the domain, since both are programming errors; MarshalText reports
// them as errors instead.
func (i ID[S]) String() string {
	var buf [idTextLen]byte
	if err := i.encode(&buf); err != nil {
		panic(err)
	}
	return string(buf[:])
}

// AppendText appends the text form to b without allocating.
func (i ID[S]) AppendText(b []byte) ([]byte, error) {
	var buf [idTextLen]byte
	if err := i.encode(&buf); err != nil {
		return b, err
	}
	return append(b, buf[:]...), nil
}

// MarshalText implements encoding.TextMarshaler, so JSON, YAML and text
// protocols use the string form on their own.
func (i ID[S]) MarshalText() ([]byte, error) {
	var buf [idTextLen]byte
	if err := i.encode(&buf); err != nil {
		return nil, err
	}
	return buf[:], nil
}

// UnmarshalText implements encoding.TextUnmarshaler. Text that was not
// produced by this table's key (a typo, another table's id, a forged
// string) is ErrBadID.
func (i *ID[S]) UnmarshalText(b []byte) error {
	n, err := idDecode(idSpaceName[S](), b)
	if err != nil {
		return err
	}
	i.n = n
	return nil
}

// Value implements driver.Valuer: the integer.
func (i ID[S]) Value() (driver.Value, error) { return i.n, nil }

// QLXArg is the argument form pgx sees: the integer.
func (i ID[S]) QLXArg() any { return i.n }

// Scan implements sql.Scanner for the integer the database returns.
func (i *ID[S]) Scan(src any) error {
	switch v := src.(type) {
	case int64:
		i.n = v
	case int32:
		i.n = int64(v)
	case int16:
		i.n = int64(v)
	case int:
		i.n = int64(v)
	case []byte:
		n, err := strconv.ParseInt(string(v), 10, 64)
		if err != nil {
			return fmt.Errorf("qt: cannot scan %q into ID: %w", v, err)
		}
		i.n = n
	case string:
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return fmt.Errorf("qt: cannot scan %q into ID: %w", v, err)
		}
		i.n = n
	case nil:
		return errors.New("qt: NULL into ID (use Null[ID])")
	default:
		return fmt.Errorf("qt: cannot scan %T into ID", src)
	}
	return nil
}

func (i ID[S]) encode(out *[idTextLen]byte) error {
	return idEncode(idSpaceName[S](), i.n, out)
}

func idSpaceName[S IDSpace]() string {
	var s S
	return s.IDSpace()
}

// ---------------------------------------------------------------------------
// Key
// ---------------------------------------------------------------------------

// Errors reported by the ID codec.
var (
	// ErrIDKey means no key was set: call SetIDKey once at startup.
	ErrIDKey = errors.New("qt: ID key not set; call qt.SetIDKey at startup")
	// ErrBadID is a string that is not a valid id of this table.
	ErrBadID = errors.New("qt: invalid id")
	// ErrIDRange is an integer outside 0 <= n < 2^48.
	ErrIDRange = errors.New("qt: id out of range (0 <= n < 2^48)")
)

type idRoot struct{ k [4]uint64 }

var idKey atomic.Pointer[idRoot]

// SetIDKey sets the secret the ID text forms are derived from. Call it once,
// before any ID is encoded or decoded, with at least 16 random bytes that stay
// the same for the life of the data: changing the key changes every text form
// ever handed out (links, API clients, stored strings), though never the
// integers in the database. Keep it with your other secrets.
//
// Calling it again replaces the key; do that only in tests.
func SetIDKey(secret []byte) error {
	if len(secret) < 16 {
		return errors.New("qt: ID key must be at least 16 bytes")
	}
	// Absorb the secret in 8-byte blocks into four lanes, with a distinct
	// finalizer per lane, so every byte reaches every lane.
	r := &idRoot{k: [4]uint64{0x243f6a8885a308d3, 0x13198a2e03707344, 0xa4093822299f31d0, 0x082efa98ec4e6c89}}
	var blk uint64
	var nb uint
	flush := func(n int) {
		for l := range r.k {
			r.k[l] = fmix64(r.k[l] ^ (blk + uint64(l)*0x9e3779b97f4a7c15 + uint64(n)))
			r.k[l] = r.k[l]<<27 | r.k[l]>>37
		}
		blk, nb = 0, 0
	}
	for _, c := range secret {
		blk |= uint64(c) << (8 * nb)
		nb++
		if nb == 8 {
			flush(len(secret))
		}
	}
	if nb > 0 {
		flush(len(secret))
	}
	for l := range r.k { // final avalanche, lanes feed each other
		r.k[l] = fmix64(r.k[l] ^ r.k[(l+1)&3] ^ uint64(len(secret)))
	}
	idKey.Store(r)
	return nil
}

// MustSetIDKey is SetIDKey that panics on a short key.
func MustSetIDKey(secret []byte) {
	if err := SetIDKey(secret); err != nil {
		panic(err)
	}
}

// IDKeySet reports whether SetIDKey has been called.
func IDKeySet() bool { return idKey.Load() != nil }

// ---------------------------------------------------------------------------
// Codec: 48-bit keyed Feistel permutation, 16-bit keyed tag, base58
// ---------------------------------------------------------------------------

const (
	idTextLen = 11
	idMax     = int64(1) << 48
	idRounds  = 6
	idAlpha   = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
)

func fmix64(x uint64) uint64 {
	x ^= x >> 33
	x *= 0xff51afd7ed558ccd
	x ^= x >> 33
	x *= 0xc4ceb9fe1a85ec53
	x ^= x >> 33
	return x
}

func fmix32(x uint32) uint32 {
	x ^= x >> 16
	x *= 0x85ebca6b
	x ^= x >> 13
	x *= 0xc2b2ae35
	x ^= x >> 16
	return x
}

// idKeys holds what one space needs, derived on the stack from the root key
// and the space name, so there is no table of spaces to maintain or lock.
type idKeys struct {
	rk  [idRounds]uint64
	tag uint64
}

func deriveKeys(root *idRoot, space string) idKeys {
	h := uint64(0xcbf29ce484222325)
	for i := 0; i < len(space); i++ {
		h = (h ^ uint64(space[i])) * 0x100000001b3
	}
	h = fmix64(h ^ uint64(len(space)))
	var k idKeys
	s := root.k[0] ^ h
	for i := range k.rk {
		s += 0x9e3779b97f4a7c15
		k.rk[i] = fmix64(s ^ root.k[1+i%3])
	}
	s += 0x9e3779b97f4a7c15
	k.tag = fmix64(s ^ root.k[3] ^ root.k[0]<<1)
	return k
}

func feistelF(x uint32, k uint64) uint32 {
	return fmix32((x^uint32(k))*0x9e3779b1+uint32(k>>32)) & 0xffffff
}

func permute(v uint64, k *idKeys) uint64 {
	l, r := uint32(v>>24)&0xffffff, uint32(v)&0xffffff
	for i := 0; i < idRounds; i++ {
		l, r = r, l^feistelF(r, k.rk[i])
	}
	return uint64(l)<<24 | uint64(r)
}

func unpermute(v uint64, k *idKeys) uint64 {
	l, r := uint32(v>>24)&0xffffff, uint32(v)&0xffffff
	for i := idRounds - 1; i >= 0; i-- {
		l, r = r^feistelF(l, k.rk[i]), l
	}
	return uint64(l)<<24 | uint64(r)
}

func idTag(c uint64, k *idKeys) uint64 { return fmix64(c^k.tag) >> 48 }

func idEncode(space string, n int64, out *[idTextLen]byte) error {
	root := idKey.Load()
	if root == nil {
		return ErrIDKey
	}
	if n < 0 || n >= idMax {
		return ErrIDRange
	}
	k := deriveKeys(root, space)
	c := permute(uint64(n), &k)
	v := c<<16 | idTag(c, &k)
	for p := idTextLen - 1; p >= 0; p-- {
		out[p] = idAlpha[v%58]
		v /= 58
	}
	return nil
}

var idRev = func() (t [256]int8) {
	for i := range t {
		t[i] = -1
	}
	for i := 0; i < len(idAlpha); i++ {
		t[idAlpha[i]] = int8(i)
	}
	return
}()

func idDecode(space string, b []byte) (int64, error) {
	root := idKey.Load()
	if root == nil {
		return 0, ErrIDKey
	}
	if len(b) != idTextLen {
		return 0, ErrBadID
	}
	var v uint64
	for _, ch := range b {
		d := idRev[ch]
		if d < 0 {
			return 0, ErrBadID
		}
		hi, lo := bits.Mul64(v, 58) // v*58 + d must fit 64 bits
		nv, carry := bits.Add64(lo, uint64(d), 0)
		if hi != 0 || carry != 0 {
			return 0, ErrBadID
		}
		v = nv
	}
	k := deriveKeys(root, space)
	c, tag := v>>16, v&0xffff
	if idTag(c, &k) != tag {
		return 0, ErrBadID
	}
	return int64(unpermute(c, &k)), nil
}
