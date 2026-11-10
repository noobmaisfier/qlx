// Package pgcrypto has helpers for the pgcrypto extension (CREATE EXTENSION
// pgcrypto): password hashing, HMAC, random bytes and symmetric encryption.
//
// Password hashing is done in the database with crypt(), so the password
// travels as a bound parameter and never appears in the SQL text:
//
//	q.InsertInto(Users).Set(Users.PasswordHash, pgcrypto.HashPassword(q.Val(pw), 12))
//
//	q.Select(Users.ID).From(Users).
//		Where(Users.Email.Eq(email), pgcrypto.PasswordMatches(Users.PasswordHash, q.Val(pw)))
//
// Keys and passwords are parameters like any other value: keep statement
// logging (log_statement, pg_stat_statements with parameter capture, tracing
// of arguments) off for statements that carry them.
//
// The calls are qualified with the schema "public", where CREATE EXTENSION
// pgcrypto puts the functions by default.
package pgcrypto

import (
	"github.com/noobmaisfier/qlx"
)

const schema = "public"

func call[T any](name string, args ...qlx.Value) qlx.Expr[T] {
	return qlx.FuncCall[T](schema, name, args...)
}

var (
	textT  = qlx.NewPgType[string]("pg_catalog", "text")
	byteaT = qlx.NewPgType[[]byte]("pg_catalog", "bytea")
	int4T  = qlx.NewPgType[int32]("pg_catalog", "int4")
)

// HashPassword is crypt(password, gen_salt('bf', cost)): a bcrypt hash with a
// fresh random salt. cost is the bcrypt work factor, 4 to 31 (the server
// refuses other values); 10 to 12 is usual. The result is a 60-character text
// to store as is.
func HashPassword(password qlx.Operand[string], cost int32) qlx.Expr[string] {
	salt := call[string]("gen_salt", qlx.ArgT(qlx.Val("bf"), textT), qlx.ArgT(qlx.Val(cost), int4T))
	return call[string]("crypt", qlx.ArgT(password, textT), salt)
}

// PasswordMatches is crypt(password, hash) = hash: true when password hashes
// to the stored hash. A NULL hash never matches. It cannot use an index; look
// the row up by another column first.
func PasswordMatches(hash, password qlx.Operand[string]) qlx.Pred {
	return call[string]("crypt", qlx.ArgT(password, textT), qlx.ArgT(hash, textT)).EqX(hash)
}

// HMAC is hmac(data, key, algorithm), the keyed hash as bytea. algorithm is
// one of md5, sha1, sha224, sha256, sha384, sha512.
func HMAC(data, key qlx.Operand[[]byte], algorithm string) qlx.Expr[[]byte] {
	return call[[]byte]("hmac", qlx.ArgT(data, byteaT), qlx.ArgT(key, byteaT), qlx.ArgT(qlx.Val(algorithm), textT))
}

// Digest is digest(data, algorithm), a plain hash as bytea (same algorithms as
// HMAC, plus any the server's OpenSSL knows).
func Digest(data qlx.Operand[[]byte], algorithm string) qlx.Expr[[]byte] {
	return call[[]byte]("digest", qlx.ArgT(data, byteaT), qlx.ArgT(qlx.Val(algorithm), textT))
}

// RandomBytes is gen_random_bytes(n): n cryptographically random bytes, for
// tokens and salts. n is 1 to 1024 (the server refuses other values).
func RandomBytes(n int32) qlx.Expr[[]byte] {
	return call[[]byte]("gen_random_bytes", qlx.ArgT(qlx.Val(n), int4T))
}

// Encrypt is pgp_sym_encrypt(data, key): OpenPGP symmetric encryption of text,
// returned as bytea. The result differs on every call (random salt and
// session key), so an encrypted column cannot be compared or indexed.
func Encrypt(data, key qlx.Operand[string]) qlx.Expr[[]byte] {
	return call[[]byte]("pgp_sym_encrypt", qlx.ArgT(data, textT), qlx.ArgT(key, textT))
}

// Decrypt is pgp_sym_decrypt(data, key): the text Encrypt produced. A wrong
// key or damaged data makes the statement fail, with no partial result.
func Decrypt(data qlx.Operand[[]byte], key qlx.Operand[string]) qlx.Expr[string] {
	return call[string]("pgp_sym_decrypt", qlx.ArgT(data, byteaT), qlx.ArgT(key, textT))
}
