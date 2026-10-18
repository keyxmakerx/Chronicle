package tags

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"net/http"
	"testing"
)

// zeroRowsDriver mimics MariaDB without clientFoundRows: every UPDATE reports
// 0 rows affected, and the EXISTS probe answers from `exists`.
type zeroRowsDriver struct{ exists bool }

func (d zeroRowsDriver) Open(string) (driver.Conn, error) { return zeroRowsConn(d), nil }

type zeroRowsConn struct{ exists bool }

func (c zeroRowsConn) Prepare(q string) (driver.Stmt, error) { return zeroRowsStmt(c), nil }
func (c zeroRowsConn) Close() error                          { return nil }
func (c zeroRowsConn) Begin() (driver.Tx, error)             { return nil, driver.ErrSkip }

type zeroRowsStmt struct{ exists bool }

func (s zeroRowsStmt) Close() error  { return nil }
func (s zeroRowsStmt) NumInput() int { return -1 }
func (s zeroRowsStmt) Exec([]driver.Value) (driver.Result, error) {
	return driver.RowsAffected(0), nil
}
func (s zeroRowsStmt) Query([]driver.Value) (driver.Rows, error) {
	return &existsRows{v: s.exists}, nil
}

type existsRows struct {
	v    bool
	done bool
}

func (r *existsRows) Columns() []string { return []string{"exists"} }
func (r *existsRows) Close() error      { return nil }
func (r *existsRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	dest[0] = r.v
	return nil
}

func TestUpdateUnchangedRow_Tags(t *testing.T) {
	for _, tt := range []struct {
		name     string
		exists   bool
		wantCode int
	}{
		{"unchanged values still succeed", true, 0},
		{"missing row is not found", false, http.StatusNotFound},
	} {
		t.Run(tt.name, func(t *testing.T) {
			drvName := "zero-rows-tags-" + tt.name
			sql.Register(drvName, zeroRowsDriver{exists: tt.exists})
			db, err := sql.Open(drvName, "")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			repo := &tagRepository{db: db}
			err = repo.Update(context.Background(), &Tag{ID: 1, Name: "a", Slug: "a", Color: "#000000"})
			if tt.wantCode == 0 {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			assertAppError(t, err, tt.wantCode)
		})
	}
}
