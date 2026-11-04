package pgxexec_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/noobmaisfier/qlx/pgxexec"
	"github.com/noobmaisfier/qlx/qt"
)

// apiError is the one JSON shape every failing endpoint answers with.
type apiError struct {
	Status int    `json:"status"`
	Code   string `json:"code"`
	Detail string `json:"detail,omitempty"`
}

// toAPIError is the mapping of docs/dtos.md: what the caller may know, never
// the text of a database error.
func toAPIError(err error) apiError {
	switch {
	case err == nil:
		return apiError{}
	case errors.Is(err, qt.ErrBadID), errors.Is(err, pgx.ErrNoRows):
		return apiError{http.StatusNotFound, "not_found", ""}
	case isConstraint(err, "23505"): // IsUnique wants a constraint name; a catch-all checks the code
		return apiError{http.StatusConflict, "conflict", ""}
	case pgxexec.IsForeignKey(err, ""):
		return apiError{http.StatusUnprocessableEntity, "invalid_reference", ""}
	case pgxexec.IsNotNull(err, ""), pgxexec.IsCheck(err, ""):
		return apiError{http.StatusUnprocessableEntity, "invalid_value", ""}
	}
	return apiError{http.StatusInternalServerError, "internal", ""}
}

func isConstraint(err error, code string) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == code
}

func TestDTOErrorMapping(t *testing.T) {
	pg := func(code string) error {
		return fmt.Errorf("wrapped: %w", &pgconn.PgError{Code: code, Message: "secret detail: users_email_key"})
	}
	for name, c := range map[string]struct {
		err    error
		status int
	}{
		"forged id":   {qt.ErrBadID, 404},
		"no rows":     {pgx.ErrNoRows, 404},
		"unique":      {pg("23505"), 409},
		"foreign key": {pg("23503"), 422},
		"not null":    {pg("23502"), 422},
		"check":       {pg("23514"), 422},
		"deadlock":    {pg("40P01"), 500},
		"other":       {errors.New("boom"), 500},
	} {
		got := toAPIError(c.err)
		if got.Status != c.status {
			t.Errorf("%s: status %d, want %d", name, got.Status, c.status)
		}
		if got.Detail != "" {
			t.Errorf("%s: the detail of a database error must not reach the client: %q", name, got.Detail)
		}
	}
	if toAPIError(nil).Status != 0 {
		t.Error("nil is not an error")
	}
}
