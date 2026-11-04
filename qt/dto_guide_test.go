package qt_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/noobmaisfier/qlx/qt"
)

// These tests compile and run the snippets of docs/dtos.md.

type dtoSpace struct{}

func (dtoSpace) IDSpace() string { return "dto_users" }

type dtoUserID = qt.ID[dtoSpace]

// A row as `qlx rows` writes it, and the response DTO built from it.
type dtoUserRow struct {
	ID       dtoUserID
	Email    string
	Nickname qt.Null[string]
	Deleted  qt.Null[qt.Timestamptz]
}

type dtoUserResp struct {
	ID       dtoUserID       `json:"id"`
	Email    string          `json:"email"`
	Nickname *string         `json:"nickname"`
	Deleted  *qt.Timestamptz `json:"deleted_at,omitempty"`
}

func toDTOUserResp(r dtoUserRow) dtoUserResp {
	return dtoUserResp{ID: r.ID, Email: r.Email, Nickname: r.Nickname.Ptr(), Deleted: r.Deleted.Ptr()}
}

func TestDTOResponse(t *testing.T) {
	if !qt.IDKeySet() {
		qt.MustSetIDKey([]byte("0123456789abcdef-dto-guide"))
	}
	resp := toDTOUserResp(dtoUserRow{ID: qt.IDOf[dtoSpace](7), Email: "a@b.c"})
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]any
	_ = json.Unmarshal(b, &back)
	if s, _ := back["id"].(string); len(s) != 11 {
		t.Errorf("id must be an 11-character string, got %v in %s", back["id"], b)
	}
	if v, present := back["nickname"]; !present || v != nil {
		t.Errorf("a NULL nickname must be an explicit null: %s", b)
	}
	if _, present := back["deleted_at"]; present {
		t.Errorf("omitempty on a nil pointer must drop the key: %s", b)
	}
	var in dtoUserResp
	if err := json.Unmarshal(b, &in); err != nil || in.ID.Int64() != 7 {
		t.Errorf("id round trip: %+v %v", in, err)
	}
	if err := json.Unmarshal([]byte(`{"id":"notanid"}`), &in); err == nil {
		t.Error("a forged id was accepted")
	} else if !errors.Is(err, qt.ErrBadID) {
		t.Logf("forged id error: %v", err)
	}
}

// A PATCH body, and the assignments built from it. In a project the patch is
// the generated UserPatch and SetPatch does the loop below.
type dtoPatchBody struct {
	Email    qt.Opt[string]          `json:"email,omitzero"`
	Nickname qt.Opt[qt.Null[string]] `json:"nickname,omitzero"`
}

func (p dtoPatchBody) assignments() (cols []string, vals []any) {
	if p.Email.Set {
		cols, vals = append(cols, "email"), append(vals, p.Email.V)
	}
	if p.Nickname.Set {
		cols, vals = append(cols, "nickname"), append(vals, p.Nickname.V.QLXArg())
	}
	return
}

func TestDTOPatchBody(t *testing.T) {
	for body, want := range map[string]struct {
		cols []string
		vals []any
	}{
		`{}`:                                 {nil, nil},
		`{"email":"a@b.c"}`:                  {[]string{"email"}, []any{"a@b.c"}},
		`{"nickname":null}`:                  {[]string{"nickname"}, []any{nil}},
		`{"nickname":"ada","email":"x@y.z"}`: {[]string{"email", "nickname"}, []any{"x@y.z", "ada"}},
	} {
		var p dtoPatchBody
		if err := json.Unmarshal([]byte(body), &p); err != nil {
			t.Fatalf("%s: %v", body, err)
		}
		cols, vals := p.assignments()
		if len(cols) != len(want.cols) {
			t.Errorf("%s: columns %v, want %v", body, cols, want.cols)
			continue
		}
		for i := range cols {
			if cols[i] != want.cols[i] || vals[i] != want.vals[i] {
				t.Errorf("%s: %v %v, want %v %v", body, cols, vals, want.cols, want.vals)
			}
		}
	}
}
