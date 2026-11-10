package pgcrypto_test

import (
	"strings"
	"testing"

	"github.com/noobmaisfier/qlx"
	"github.com/noobmaisfier/qlx/qext/pgcrypto"
)

func TestRenderedShapes(t *testing.T) {
	pw := qlx.NewParam[string]()
	h := qlx.NewParam[string]()
	for name, tc := range map[string]struct {
		st   qlx.Statement
		want string
	}{
		"hash":    {qlx.Select(pgcrypto.HashPassword(pw, 12)), `"public"."crypt"(`},
		"match":   {qlx.Select(qlx.Test(pgcrypto.PasswordMatches(h, pw))), `"public"."crypt"(`},
		"random":  {qlx.Select(pgcrypto.RandomBytes(16)), `"public"."gen_random_bytes"(`},
		"encrypt": {qlx.Select(pgcrypto.Encrypt(pw, h)), `"public"."pgp_sym_encrypt"(`},
	} {
		r, err := qlx.Render(qlx.Dialect{Version: qlx.PG18}, tc.st)
		if err != nil || !strings.Contains(r.SQL, tc.want) {
			t.Errorf("%s: %v %s", name, err, r.SQL)
		}
	}
}
