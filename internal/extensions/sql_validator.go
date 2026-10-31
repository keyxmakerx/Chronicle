package extensions

import (
	"fmt"
	"strings"
)

// An extension migration is checked and run as the same token stream: the SQL
// is tokenized once, each statement is matched against an allowlist, and the
// runner executes each statement's text with its comments blanked out. A
// comment, quote or keyword the checker reads one way can therefore never be
// read another way by the database.
//
// Allowed statements, each naming only tables of this extension:
//
//	CREATE TABLE [IF NOT EXISTS] t (...) | CREATE TABLE t LIKE t2
//	ALTER TABLE t ...
//	DROP TABLE [IF EXISTS] t [, t2 ...]
//	CREATE [UNIQUE|FULLTEXT|SPATIAL] INDEX [IF NOT EXISTS] i ON t (...)
//	DROP INDEX [IF EXISTS] i ON t
//	INSERT [IGNORE] INTO t ...
//	UPDATE [IGNORE] t SET ...
//	DELETE FROM t [WHERE|ORDER BY|LIMIT ...]
//
// No statement may read another table or reach the server's files: SELECT,
// UNION, a TABLE or INTO outside the statement's head, file functions,
// variables, CONNECTION and DIRECTORY options are refused anywhere, and a
// table's ENGINE must be a local one. A foreign key may reference only this
// extension's tables.
// Double-quoted text is refused, because the server may read it as a string
// or as a name depending on its SQL mode; use single quotes for strings and
// backticks for names, including a column named like a keyword.

type sqlTokenKind int

const (
	tokWord   sqlTokenKind = iota // bare word: keyword, name or number
	tokQuoted                     // `backticked name`
	tokString                     // 'string'
	tokPunct                      // one character of punctuation
)

type sqlToken struct {
	kind sqlTokenKind
	raw  string // as written, quotes included
	val  string // a word upper-cased, a quoted name unquoted
}

// ExtensionTablePrefix is the prefix every table of the extension with this
// slug carries.
func ExtensionTablePrefix(slug string) string {
	return "ext_" + slug + "_"
}

// ParseExtensionSQL checks an extension migration against the allowlist and
// returns its statements as they must be executed. The first violation is
// returned as the error.
func ParseExtensionSQL(slug, sql string) ([]string, error) {
	if slug == "" {
		return nil, fmt.Errorf("extension slug is required")
	}
	stmts, err := tokenizeSQL(sql)
	if err != nil {
		return nil, err
	}
	prefix := ExtensionTablePrefix(slug)
	out := make([]string, 0, len(stmts))
	for _, st := range stmts {
		if err := checkStatement(st.toks, prefix); err != nil {
			return nil, err
		}
		out = append(out, strings.TrimSpace(st.text))
	}
	return out, nil
}

// ValidateExtensionSQL reports whether an extension migration passes the
// allowlist.
func ValidateExtensionSQL(slug, sql string) error {
	_, err := ParseExtensionSQL(slug, sql)
	return err
}

// sqlStatement is one statement's tokens and its text as it will run: the
// source with every comment replaced by a space.
type sqlStatement struct {
	toks []sqlToken
	text string
}

// tokenizeSQL splits sql into statements.
func tokenizeSQL(sql string) ([]sqlStatement, error) {
	var stmts []sqlStatement
	var cur []sqlToken
	var text strings.Builder
	flush := func() {
		if len(cur) > 0 {
			stmts = append(stmts, sqlStatement{toks: cur, text: text.String()})
		}
		cur = nil
		text.Reset()
	}
	for i := 0; i < len(sql); {
		ch := sql[i]
		start := i
		comment := false
		switch {
		case isSQLSpace(ch):
			i++
		case ch == '#':
			i = skipLine(sql, i)
			comment = true
		case ch == '-' && i+1 < len(sql) && sql[i+1] == '-':
			// MySQL reads "--" as a comment only before whitespace; "1--1"
			// is arithmetic. Refuse the second form rather than guess.
			if i+2 < len(sql) && !isSQLSpace(sql[i+2]) {
				return nil, fmt.Errorf(`"--" must be followed by a space to start a comment`)
			}
			i = skipLine(sql, i)
			comment = true
		case ch == '/' && i+1 < len(sql) && sql[i+1] == '*':
			rest := sql[i+2:]
			if strings.HasPrefix(rest, "!") || strings.HasPrefix(rest, "M!") || strings.HasPrefix(rest, "+") {
				return nil, fmt.Errorf("executable comments and optimizer hints are not allowed")
			}
			end := strings.Index(rest, "*/")
			if end < 0 {
				return nil, fmt.Errorf("unterminated comment")
			}
			i += 2 + end + 2
			comment = true
		case ch == ';':
			flush()
			i++
			continue
		case ch == '\'':
			j, err := scanQuoted(sql, i, '\'', true)
			if err != nil {
				return nil, err
			}
			cur = append(cur, sqlToken{kind: tokString, raw: sql[i:j]})
			i = j
		case ch == '`':
			j, err := scanQuoted(sql, i, '`', false)
			if err != nil {
				return nil, err
			}
			raw := sql[i:j]
			cur = append(cur, sqlToken{kind: tokQuoted, raw: raw, val: strings.ReplaceAll(raw[1:len(raw)-1], "``", "`")})
			i = j
		case ch == '"':
			return nil, fmt.Errorf("double-quoted text is not allowed: use 'single quotes' for strings and `backticks` for names")
		case isWordByte(ch):
			j := i
			for j < len(sql) && isWordByte(sql[j]) {
				j++
			}
			cur = append(cur, sqlToken{kind: tokWord, raw: sql[i:j], val: strings.ToUpper(sql[i:j])})
			i = j
		case ch >= 0x80 || ch < 0x20:
			return nil, fmt.Errorf("unexpected character %q outside a string", ch)
		default:
			cur = append(cur, sqlToken{kind: tokPunct, raw: string(ch), val: string(ch)})
			i++
		}
		if comment {
			text.WriteByte(' ')
		} else {
			text.WriteString(sql[start:i])
		}
	}
	flush()
	return stmts, nil
}

func isSQLSpace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '\f' || ch == '\v'
}

func isWordByte(ch byte) bool {
	return ch == '_' || ch == '$' || (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func skipLine(sql string, i int) int {
	if end := strings.IndexByte(sql[i:], '\n'); end >= 0 {
		return i + end + 1
	}
	return len(sql)
}

// scanQuoted returns the index just past the quote that closes the one at i.
// A doubled quote stands for itself, and in strings so does a backslash
// escape, as the server reads them by default.
func scanQuoted(sql string, i int, q byte, backslash bool) (int, error) {
	for j := i + 1; j < len(sql); j++ {
		switch sql[j] {
		case '\\':
			if backslash {
				j++
			}
		case q:
			if j+1 < len(sql) && sql[j+1] == q {
				j++
				continue
			}
			return j + 1, nil
		}
	}
	return 0, fmt.Errorf("unterminated quoted text")
}

// refusedAnywhere are words that would let a statement read or write outside
// its own table, or reach the server's files or another server.
var refusedAnywhere = map[string]bool{
	"SELECT": true, "UNION": true, "OUTFILE": true, "DUMPFILE": true, "LOAD_FILE": true,
	"CONNECTION": true, "DIRECTORY": true,
}

// localEngines are the storage engines a table may use; the others (FEDERATED,
// CONNECT, MERGE and the like) can reach other tables, files or servers.
var localEngines = map[string]bool{"INNODB": true, "ARIA": true, "MYISAM": true, "MEMORY": true}

// checkStatement matches one statement against the allowlist.
func checkStatement(toks []sqlToken, prefix string) error {
	p := &stmtParser{toks: toks, prefix: prefix}
	head, err := p.head()
	if err != nil {
		return err
	}
	// Everything after the head is checked for words that reach other
	// tables or the server, and for every foreign key target.
	for i := head; i < len(toks); i++ {
		t := toks[i]
		if t.kind == tokPunct && t.val == "@" {
			return fmt.Errorf("variables are not allowed")
		}
		if t.kind != tokWord {
			continue
		}
		switch {
		case refusedAnywhere[t.val]:
			return fmt.Errorf("%s is not allowed in an extension migration", t.val)
		case t.val == "TABLE" || t.val == "TABLES" || t.val == "INTO":
			return fmt.Errorf("%s is allowed only at the start of a statement", t.val)
		case t.val == "ENGINE":
			j := i + 1
			if p.punct(j) == "=" {
				j++
			}
			if !localEngines[p.word(j)] {
				return fmt.Errorf("only the InnoDB, Aria, MyISAM and MEMORY engines are allowed")
			}
		case t.val == "REFERENCES":
			if err := p.tableAt(i + 1); err != nil {
				return fmt.Errorf("foreign key: %w", err)
			}
		case t.val == "RENAME" && p.isAlter:
			if err := p.renameTarget(i + 1); err != nil {
				return err
			}
		}
	}
	return nil
}

type stmtParser struct {
	toks    []sqlToken
	prefix  string
	isAlter bool
}

func (p *stmtParser) word(i int) string {
	if i < len(p.toks) && p.toks[i].kind == tokWord {
		return p.toks[i].val
	}
	return ""
}

func (p *stmtParser) punct(i int) string {
	if i < len(p.toks) && p.toks[i].kind == tokPunct {
		return p.toks[i].val
	}
	return ""
}

// skipWords returns i advanced past the given words, if they appear in order.
func (p *stmtParser) skipWords(i int, words ...string) int {
	for k, w := range words {
		if p.word(i+k) != w {
			return i
		}
	}
	return i + len(words)
}

// tableAt checks that the name at i is one of this extension's tables, and
// not a name qualified with a database.
func (p *stmtParser) tableAt(i int) error {
	if i >= len(p.toks) || (p.toks[i].kind != tokWord && p.toks[i].kind != tokQuoted) {
		return fmt.Errorf("expected a table name")
	}
	name := p.toks[i].raw
	if p.toks[i].kind == tokQuoted {
		name = p.toks[i].val
	}
	if p.punct(i+1) == "." {
		return fmt.Errorf("table %q: names qualified with a database are not allowed", name)
	}
	lower := strings.ToLower(name)
	if !strings.HasPrefix(lower, p.prefix) || len(lower) == len(p.prefix) {
		return fmt.Errorf("table %q is not allowed: extension tables must be prefixed with %q", name, p.prefix)
	}
	return nil
}

// renameTarget checks ALTER TABLE ... RENAME [TO|AS] t, leaving a column,
// index or key rename alone.
func (p *stmtParser) renameTarget(i int) error {
	switch p.word(i) {
	case "COLUMN", "INDEX", "KEY":
		return nil
	case "TO", "AS":
		i++
	}
	return p.tableAt(i)
}

// head checks the statement's opening, the part that names what it acts on,
// and returns the index of the first token after it.
func (p *stmtParser) head() (int, error) {
	switch p.word(0) {
	case "CREATE":
		i := 1
		switch p.word(i) {
		case "UNIQUE", "FULLTEXT", "SPATIAL":
			i++
		}
		switch p.word(i) {
		case "TABLE":
			i = p.skipWords(i+1, "IF", "NOT", "EXISTS")
			if err := p.tableAt(i); err != nil {
				return 0, err
			}
			i++
			if p.word(i) == "LIKE" {
				if err := p.tableAt(i + 1); err != nil {
					return 0, err
				}
				i += 2
			}
			return i, nil
		case "INDEX":
			return p.indexOn(p.skipWords(i+1, "IF", "NOT", "EXISTS"))
		}
	case "ALTER":
		if p.word(1) == "TABLE" {
			if err := p.tableAt(2); err != nil {
				return 0, err
			}
			p.isAlter = true
			return 3, nil
		}
	case "DROP":
		switch p.word(1) {
		case "TABLE":
			i := p.skipWords(2, "IF", "EXISTS")
			for {
				if err := p.tableAt(i); err != nil {
					return 0, err
				}
				i++
				if p.punct(i) != "," {
					break
				}
				i++
			}
			switch p.word(i) {
			case "RESTRICT", "CASCADE":
				i++
			}
			if i != len(p.toks) {
				return 0, fmt.Errorf("unexpected text after DROP TABLE")
			}
			return i, nil
		case "INDEX":
			i, err := p.indexOn(p.skipWords(2, "IF", "EXISTS"))
			if err == nil && i != len(p.toks) {
				err = fmt.Errorf("unexpected text after DROP INDEX")
			}
			return i, err
		}
	case "INSERT":
		i := p.skipWords(1, "IGNORE")
		if p.word(i) != "INTO" {
			return 0, fmt.Errorf("INSERT must be INSERT [IGNORE] INTO")
		}
		if err := p.tableAt(i + 1); err != nil {
			return 0, err
		}
		return i + 2, nil
	case "UPDATE":
		i := p.skipWords(1, "IGNORE")
		if err := p.tableAt(i); err != nil {
			return 0, err
		}
		// One table only: no alias, list or join before SET.
		if p.word(i+1) != "SET" {
			return 0, fmt.Errorf("UPDATE may change only one table: expected SET after the table name")
		}
		return i + 2, nil
	case "DELETE":
		if p.word(1) != "FROM" {
			return 0, fmt.Errorf("DELETE must be DELETE FROM one table")
		}
		if err := p.tableAt(2); err != nil {
			return 0, err
		}
		switch p.word(3) {
		case "WHERE", "ORDER", "LIMIT":
		default:
			if len(p.toks) > 3 {
				return 0, fmt.Errorf("DELETE may change only one table")
			}
		}
		return 3, nil
	}
	return 0, fmt.Errorf("statement %q is not allowed in an extension migration", headText(p.toks))
}

// indexOn checks "<index name> ON <table>" starting at i.
func (p *stmtParser) indexOn(i int) (int, error) {
	if i >= len(p.toks) || (p.toks[i].kind != tokWord && p.toks[i].kind != tokQuoted) {
		return 0, fmt.Errorf("expected an index name")
	}
	if p.word(i+1) != "ON" {
		return 0, fmt.Errorf("expected ON <table> after the index name")
	}
	if err := p.tableAt(i + 2); err != nil {
		return 0, err
	}
	return i + 3, nil
}

// headText is the first few words of a statement, for an error message.
func headText(toks []sqlToken) string {
	n := len(toks)
	if n > 3 {
		n = 3
	}
	parts := make([]string, n)
	for i := range parts {
		parts[i] = toks[i].raw
	}
	return strings.Join(parts, " ")
}
