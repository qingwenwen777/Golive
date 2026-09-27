package wallet_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// SQL that changes a wallet column or the ledger. Reads (SELECT, WHERE
// coin_balance >= ?, SELECT ... FOR UPDATE) do not match.
var (
	// A wallet column assigned, quoted or not: SET `coin_balance` = ...
	walletSQLAssign = regexp.MustCompile("(?i)\\b(coin_balance|frozen_coins)[`\"\\]]?\\s*=([^=]|$)")
	// A wallet column or the ledger named in a statement that changes rows:
	// INSERT INTO users (..., coin_balance), DELETE FROM coin_transactions.
	walletSQLName = regexp.MustCompile(`(?i)\b(coin_balance|frozen_coins|coin_transactions)\b`)
	sqlWriteVerb  = regexp.MustCompile(`(?i)\b(insert|update|delete|truncate)\b|\breplace\s+[^\s(]`)
	// Clauses that contain a write verb but do not write: row locks and
	// foreign-key actions.
	sqlNotWrite = regexp.MustCompile(`(?i)\bfor\s+update\b|\bon\s+(update|delete)\b`)
	sqlComment  = regexp.MustCompile(`(?s)/\*.*?\*/|--[^\n]*|#[^\n]*`)
)

var walletColumns = map[string]bool{"coin_balance": true, "frozen_coins": true}
var walletFields = map[string]bool{"CoinBalance": true, "FrozenCoins": true}

// gormWrites are the GORM methods that insert, update or delete rows.
var gormWrites = map[string]bool{
	"Create": true, "CreateInBatches": true, "FirstOrCreate": true, "Save": true, "Delete": true,
	"Update": true, "Updates": true, "UpdateColumn": true, "UpdateColumns": true,
}

// readOnlyCalls name a table or column (Table, Omit, Order, Group, Pluck) or
// take a destination to read into (Scan) without writing either, so their
// arguments may name the ledger or a wallet column, or point at a wallet field.
var readOnlyCalls = map[string]bool{"Table": true, "Omit": true, "Order": true, "Group": true, "Pluck": true, "Scan": true}

// walletOps are the pkg/wallet functions; each returns the ledger row it wrote.
var walletOps = map[string]bool{"Debit": true, "AdminDebit": true, "Credit": true, "Freeze": true, "Unfreeze": true}

// walletWrite is one place outside pkg/wallet that writes the coin wallet.
type walletWrite struct {
	pos  token.Position
	fn   string // enclosing function, Type.Method for methods; "" at package level
	what string
}

func (w walletWrite) String() string {
	if w.fn == "" {
		return fmt.Sprintf("%s: %s", w.pos, w.what)
	}
	return fmt.Sprintf("%s: in %s: %s", w.pos, w.fn, w.what)
}

// walletWriteAllowlist is every finding allowed outside pkg/wallet, by file,
// enclosing function and finding. Only two kinds belong here: a new
// account's opening balance, set in the User literal that creates its row
// (there is no balance yet for pkg/wallet to change), and reads the scanner
// cannot tell from writes. Each entry allows one finding and must match one,
// so it can neither go stale nor quietly cover a new write.
var walletWriteAllowlist = []struct{ file, fn, what, why string }{
	{"app/user-service/cmd/main.go", "seedDemoUser", "CoinBalance set in a composite literal",
		"opening balance of the bootstrap demo account"},
	{"app/user-service/cmd/main.go", "seedAdminUser", "CoinBalance set in a composite literal",
		"opening balance of the bootstrap admin account"},
	{"app/user-service/internal/handler/creator.go", "AdminHandler.CreateAdmin", "CoinBalance set in a composite literal",
		"opening balance of an admin account created from the admin console"},
	{"app/user-service/internal/service/auth.go", "newLocalUser", "CoinBalance set in a composite literal",
		"opening balance of a registered account"},
	{"app/user-service/internal/service/google_auth.go", "newGoogleUser", "CoinBalance set in a composite literal",
		"opening balance of an account created by Google sign-in"},
	{"app/user-service/internal/model/user.go", "User.Public", "CoinBalance set in a composite literal",
		"read: copies the stored balance into the API response"},
	{"app/user-service/internal/model/user.go", "User.Public", "FrozenCoins set in a composite literal",
		"read: copies the stored frozen coins into the API response"},
}

// walletScanner finds wallet writes in one file. The code is not
// type-checked, so types are recognized by name (CoinTransaction, User, and
// the CoinBalance and FrozenCoins fields of any struct) and variables are
// followed by name within a function.
type walletScanner struct {
	fset      *token.FileSet
	walletPkg string            // local name of the pkg/wallet import
	fn        string            // function being scanned, as in walletWrite
	rows      map[string]bool   // variables holding ledger rows: a CoinTransaction, or a pointer or slice of them
	users     map[string]bool   // variables holding a User, or a pointer or slice of them
	ledgerDBs map[string]bool   // variables holding a GORM query on coin_transactions
	seen      map[ast.Node]bool // strings already checked as part of a concatenation, and struct tags
	readOnly  map[ast.Node]bool // arguments of readOnlyCalls
	found     []walletWrite
}

// scanWalletWrites reports every place in file that writes the coin wallet
// without going through pkg/wallet: SQL that writes a wallet column or the
// ledger; a wallet column, field or the ledger table named in a string; a
// wallet field set in a literal, by assignment or through a pointer; a
// CoinTransaction built or handed to a GORM write; a GORM write aimed at
// coin_transactions; and a Save or Updates of a whole User.
func scanWalletWrites(fset *token.FileSet, file *ast.File) []walletWrite {
	s := &walletScanner{
		fset: fset, walletPkg: "wallet",
		rows: map[string]bool{}, users: map[string]bool{}, ledgerDBs: map[string]bool{},
		seen: map[ast.Node]bool{}, readOnly: map[ast.Node]bool{},
	}
	for _, imp := range file.Imports {
		if imp.Name != nil && strings.HasSuffix(imp.Path.Value, `/pkg/wallet"`) {
			s.walletPkg = imp.Name.Name
		}
	}
	// Package-level variables are visible in every function.
	for _, decl := range file.Decls {
		if _, ok := decl.(*ast.GenDecl); ok {
			s.track(decl)
		}
	}
	rows, users, ledgerDBs := s.rows, s.users, s.ledgerDBs
	for _, decl := range file.Decls {
		s.fn, s.rows, s.users, s.ledgerDBs = "", maps.Clone(rows), maps.Clone(users), maps.Clone(ledgerDBs)
		if fd, ok := decl.(*ast.FuncDecl); ok {
			s.fn = funcName(fd)
			s.track(fd)
		}
		ast.Inspect(decl, s.visit)
	}
	return s.found
}

func (s *walletScanner) report(n ast.Node, what string) {
	s.found = append(s.found, walletWrite{pos: s.fset.Position(n.Pos()), fn: s.fn, what: what})
}

func (s *walletScanner) visit(n ast.Node) bool {
	switch n := n.(type) {
	case *ast.BasicLit, *ast.BinaryExpr:
		// A string, or a + chain of them checked as one: "... coin_balance" + " = ...".
		e := n.(ast.Expr)
		if s.seen[e] {
			break
		}
		str, ok := constString(e)
		if !ok {
			break
		}
		s.markSeen(e)
		switch {
		case walletSQL(str):
			s.report(e, "SQL writes the coin wallet: "+strings.Join(strings.Fields(str), " "))
		case walletName(str) != "" && !s.readOnly[e]:
			// Update("CoinBalance", ...), map keys, clause.Column{Name: ...},
			// and names spliced into SQL later.
			s.report(e, fmt.Sprintf("string %q names the coin wallet", str))
		}
	case *ast.CallExpr:
		s.checkCall(n)
	case *ast.UnaryExpr:
		// p := &u.CoinBalance can be written through; Scan(&u.CoinBalance) only reads.
		if sel, ok := ast.Unparen(n.X).(*ast.SelectorExpr); ok && n.Op == token.AND && walletFields[sel.Sel.Name] && !s.readOnly[n] {
			s.report(n, "pointer to ."+sel.Sel.Name)
		}
	case *ast.CompositeLit:
		// model.User{CoinBalance: ...} passed to Create/Updates.
		for _, elt := range n.Elts {
			if kv, ok := elt.(*ast.KeyValueExpr); ok {
				if id, ok := kv.Key.(*ast.Ident); ok && walletFields[id.Name] {
					s.report(kv, id.Name+" set in a composite literal")
				}
			}
		}
		if n.Type != nil {
			s.checkRowLiteral(n, n.Type)
		}
	case *ast.AssignStmt:
		// u.CoinBalance = ... followed by Save(&u); row.Amount = ... followed
		// by Create(&row).
		if n.Tok != token.DEFINE {
			for _, lhs := range n.Lhs {
				s.checkFieldWrite(lhs, "")
			}
		}
	case *ast.IncDecStmt:
		s.checkFieldWrite(n.X, n.Tok.String())
	case *ast.Field:
		// A tag is not a string in the code, but a gorm column tag can map a
		// field of another name onto a wallet column.
		if n.Tag != nil {
			s.seen[n.Tag] = true
			s.checkTag(n)
		}
	case *ast.StructType:
		for _, f := range n.Fields.List {
			if len(f.Names) == 0 && ledgerType(f.Type) {
				s.report(f, "struct embeds CoinTransaction")
			}
		}
	case *ast.TypeSpec:
		// type Ledger = model.CoinTransaction would hide rows from the checks above.
		if n.Name.Name != "CoinTransaction" && ledgerType(n.Type) {
			s.report(n, "type "+n.Name.Name+" stands in for CoinTransaction")
		}
	}
	return true
}

// checkCall notes the arguments of readOnlyCalls and reports a GORM write
// aimed at coin_transactions or given a ledger row, and a write of a whole
// User, which carries its balance along.
func (s *walletScanner) checkCall(call *ast.CallExpr) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}
	name := sel.Sel.Name
	if readOnlyCalls[name] {
		for _, arg := range call.Args {
			s.readOnly[ast.Unparen(arg)] = true
		}
	}
	if !gormWrites[name] {
		return
	}
	ledger, user := s.ledgerScoped(sel.X), false
	for _, arg := range call.Args {
		ledger = ledger || s.isRow(arg)
		if !s.isUser(arg) {
			continue
		}
		// Save writes every column, the balance included. Updates skips zero
		// fields, so a User literal is judged by its keys unless Select("*")
		// writes them all, but a loaded User brings its balance along.
		switch name {
		case "Save":
			user = true
		case "Updates", "UpdateColumns":
			user = user || !isLiteral(arg) || selectsAll(sel.X)
		}
	}
	switch {
	case ledger:
		s.report(sel.Sel, name+" writes coin_transactions")
	case user:
		s.report(sel.Sel, name+" of a whole User writes coin_balance and frozen_coins")
	}
}

// checkRowLiteral reports lit if its type, written out or implied by an
// enclosing slice or map literal ([]CoinTransaction{{...}}), is
// CoinTransaction and it sets any field. An empty CoinTransaction{} is fine:
// it is how GORM is told which table to read.
func (s *walletScanner) checkRowLiteral(lit *ast.CompositeLit, typ ast.Expr) {
	var elem ast.Expr
	switch t := derefType(typ).(type) {
	case *ast.ArrayType:
		elem = t.Elt
	case *ast.MapType:
		elem = t.Value
	default:
		if ledgerType(t) && len(lit.Elts) > 0 {
			s.report(lit, "coin_transactions row built outside pkg/wallet")
		}
		return
	}
	for _, e := range lit.Elts {
		if kv, ok := e.(*ast.KeyValueExpr); ok {
			e = kv.Value
		}
		if inner, ok := e.(*ast.CompositeLit); ok && inner.Type == nil {
			s.checkRowLiteral(inner, elem)
		}
	}
}

// checkFieldWrite reports an assignment (incDec "") or ++/-- to a wallet
// field of any struct, or to any field of a ledger row.
func (s *walletScanner) checkFieldWrite(target ast.Expr, incDec string) {
	sel, ok := ast.Unparen(target).(*ast.SelectorExpr)
	if !ok {
		return
	}
	what := "assignment to ." + sel.Sel.Name
	if incDec != "" {
		what = "." + sel.Sel.Name + incDec
	}
	switch {
	case walletFields[sel.Sel.Name]:
		s.report(target, what)
	case s.isRow(sel.X):
		s.report(target, what+" of a coin_transactions row")
	}
}

// checkTag reports a gorm tag that maps a struct field onto a wallet column.
func (s *walletScanner) checkTag(f *ast.Field) {
	tag, err := strconv.Unquote(f.Tag.Value)
	if err != nil {
		return
	}
	for _, setting := range strings.Split(reflect.StructTag(tag).Get("gorm"), ";") {
		key, column, _ := strings.Cut(setting, ":")
		if strings.EqualFold(strings.TrimSpace(key), "column") && walletColumns[strings.ToLower(walletName(column))] {
			s.report(f.Tag, "gorm tag maps a field onto "+strings.TrimSpace(column))
		}
	}
}

// track records which variables in decl hold ledger rows, Users or a query
// on coin_transactions. It repeats until nothing changes, so a variable
// copied from a tracked one is tracked too.
func (s *walletScanner) track(decl ast.Node) {
	for changed := true; changed; {
		changed = false
		mark := func(set map[string]bool, e ast.Expr) {
			if id, ok := e.(*ast.Ident); ok && id.Name != "_" && !set[id.Name] {
				set[id.Name], changed = true, true
			}
		}
		// declare tracks name by its declared type or the value it is given.
		declare := func(name, typ, value ast.Expr) {
			if namedType(typ, "CoinTransaction") || s.isRow(value) {
				mark(s.rows, name)
			}
			if namedType(typ, "User") || s.isUser(value) {
				mark(s.users, name)
			}
			if s.ledgerScoped(value) {
				mark(s.ledgerDBs, name)
			}
		}
		params := func(ft *ast.FuncType) {
			for _, list := range []*ast.FieldList{ft.Params, ft.Results} {
				if list == nil {
					continue
				}
				for _, f := range list.List {
					for _, name := range f.Names {
						declare(name, f.Type, nil)
					}
				}
			}
		}
		ast.Inspect(decl, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.FuncDecl:
				params(n.Type)
			case *ast.FuncLit:
				params(n.Type)
			case *ast.ValueSpec:
				for i, name := range n.Names {
					var v ast.Expr
					if i < len(n.Values) {
						v = n.Values[i]
					}
					declare(name, n.Type, v)
				}
			case *ast.AssignStmt:
				for i, lhs := range n.Lhs {
					switch {
					case len(n.Rhs) == len(n.Lhs):
						declare(lhs, nil, n.Rhs[i])
					case i == 0 && s.isWalletCall(n.Rhs[0]):
						// row, err := wallet.Credit(...)
						mark(s.rows, lhs)
					case i == 0:
						// row, ok := v.(*CoinTransaction)
						declare(lhs, nil, n.Rhs[0])
					}
				}
			case *ast.RangeStmt:
				if n.Value != nil {
					declare(n.Value, nil, n.X)
				}
			}
			return true
		})
	}
}

// isRow and isUser report whether e holds ledger rows or Users.
func (s *walletScanner) isRow(e ast.Expr) bool  { return holds(e, "CoinTransaction", s.rows) }
func (s *walletScanner) isUser(e ast.Expr) bool { return holds(e, "User", s.users) }

// holds reports whether e is a value of the type called typ, or a pointer or
// slice of them: a literal, new, make, append, conversion or type assertion,
// or a variable in vars.
func holds(e ast.Expr, typ string, vars map[string]bool) bool {
	switch e := e.(type) {
	case *ast.Ident:
		return vars[e.Name]
	case *ast.ParenExpr:
		return holds(e.X, typ, vars)
	case *ast.StarExpr:
		return holds(e.X, typ, vars)
	case *ast.UnaryExpr:
		return e.Op == token.AND && holds(e.X, typ, vars)
	case *ast.IndexExpr:
		return holds(e.X, typ, vars)
	case *ast.SliceExpr:
		return holds(e.X, typ, vars)
	case *ast.CompositeLit:
		return namedType(e.Type, typ)
	case *ast.TypeAssertExpr:
		return namedType(e.Type, typ)
	case *ast.CallExpr:
		if id, ok := e.Fun.(*ast.Ident); ok && len(e.Args) > 0 {
			switch id.Name {
			case "new", "make":
				return namedType(e.Args[0], typ)
			case "append":
				return holds(e.Args[0], typ, vars)
			}
		}
		return namedType(e.Fun, typ) // conversion: model.CoinTransaction(v)
	}
	return false
}

// isWalletCall reports whether e calls one of the walletOps.
func (s *walletScanner) isWalletCall(e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == s.walletPkg && walletOps[sel.Sel.Name]
}

// ledgerScoped reports whether the GORM chain e is aimed at coin_transactions:
// it calls Table("coin_transactions ...") or Model(<ledger row>), or starts
// from a variable holding such a chain.
func (s *walletScanner) ledgerScoped(e ast.Expr) bool {
	calls, root := chain(e)
	for _, call := range calls {
		name := call.Fun.(*ast.SelectorExpr).Sel.Name
		if len(call.Args) > 0 && (name == "Table" && isLedgerTable(call.Args[0]) ||
			name == "Model" && s.isRow(call.Args[0])) {
			return true
		}
	}
	return s.ledgerDBs[root]
}

// selectsAll reports whether the GORM chain e calls Select("*"), which makes
// Updates write zero-valued fields too.
func selectsAll(e ast.Expr) bool {
	calls, _ := chain(e)
	for _, call := range calls {
		if call.Fun.(*ast.SelectorExpr).Sel.Name != "Select" {
			continue
		}
		for _, arg := range call.Args {
			if s, _ := constString(arg); strings.TrimSpace(s) == "*" {
				return true
			}
		}
	}
	return false
}

// chain returns the method calls (each with a *ast.SelectorExpr Fun) of a
// GORM chain such as tx.Table("t").Where(...), outermost first, and the
// variable it starts from.
func chain(e ast.Expr) (calls []*ast.CallExpr, root string) {
	for {
		switch x := ast.Unparen(e).(type) {
		case *ast.Ident:
			return calls, x.Name
		case *ast.CallExpr:
			sel, ok := x.Fun.(*ast.SelectorExpr)
			if !ok {
				return calls, ""
			}
			calls, e = append(calls, x), sel.X
		default:
			return calls, ""
		}
	}
}

// markSeen marks the string literals and + nodes of a concatenation so they
// are not checked again one by one.
func (s *walletScanner) markSeen(e ast.Expr) {
	switch e := e.(type) {
	case *ast.ParenExpr:
		s.markSeen(e.X)
	case *ast.BasicLit:
		s.seen[e] = true
	case *ast.BinaryExpr:
		if e.Op == token.ADD {
			s.seen[e] = true
			s.markSeen(e.X)
			s.markSeen(e.Y)
		}
	}
}

// constString returns the value of a string literal or of a + chain with at
// least one string literal in it; any other operand reads as a space.
func constString(e ast.Expr) (string, bool) {
	switch e := e.(type) {
	case *ast.ParenExpr:
		return constString(e.X)
	case *ast.BasicLit:
		if e.Kind == token.STRING {
			s, err := strconv.Unquote(e.Value)
			return s, err == nil
		}
	case *ast.BinaryExpr:
		if e.Op == token.ADD {
			x, xok := constString(e.X)
			y, yok := constString(e.Y)
			return x + y, xok || yok
		}
	}
	return " ", false
}

// walletSQL reports whether SQL text s writes a wallet column or the ledger.
// It is checked as written and with comments blanked out.
func walletSQL(s string) bool {
	for _, sql := range []string{s, sqlComment.ReplaceAllString(s, " ")} {
		if walletSQLAssign.MatchString(sql) ||
			walletSQLName.MatchString(sql) && sqlWriteVerb.MatchString(sqlNotWrite.ReplaceAllString(sql, " ")) {
			return true
		}
	}
	return false
}

// walletName returns s if it names, on its own, a wallet column, a wallet
// field or the ledger table, allowing quotes and a table or schema prefix
// ("`users`.`coin_balance`", "CoinBalance", "coin_transactions"). It returns
// "" otherwise.
func walletName(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s[strings.LastIndexByte(s, '.')+1:], "`\"[] ")
	if walletFields[s] || walletColumns[strings.ToLower(s)] || strings.EqualFold(s, "coin_transactions") {
		return s
	}
	return ""
}

// isLedgerTable reports whether a Table() argument is coin_transactions,
// with or without an alias.
func isLedgerTable(arg ast.Expr) bool {
	s, _ := constString(arg)
	fields := strings.Fields(s)
	return len(fields) > 0 && strings.EqualFold(walletName(fields[0]), "coin_transactions")
}

// ledgerType reports whether t is CoinTransaction, or a pointer, slice, array
// or map of it.
func ledgerType(t ast.Expr) bool { return namedType(t, "CoinTransaction") }

// namedType reports whether type expression t is the type called name, from
// any package, or a pointer, slice, array or map of it.
func namedType(t ast.Expr, name string) bool {
	switch t := derefType(t).(type) {
	case *ast.Ident:
		return t.Name == name
	case *ast.SelectorExpr:
		return t.Sel.Name == name
	case *ast.ArrayType:
		return namedType(t.Elt, name)
	case *ast.MapType:
		return namedType(t.Value, name)
	}
	return false
}

// isLiteral reports whether e is a composite literal or its address.
func isLiteral(e ast.Expr) bool {
	if u, ok := ast.Unparen(e).(*ast.UnaryExpr); ok && u.Op == token.AND {
		e = u.X
	}
	_, ok := ast.Unparen(e).(*ast.CompositeLit)
	return ok
}

// derefType strips parentheses and pointers from a type expression.
func derefType(t ast.Expr) ast.Expr {
	for {
		switch x := t.(type) {
		case *ast.ParenExpr:
			t = x.X
		case *ast.StarExpr:
			t = x.X
		default:
			return t
		}
	}
}

// funcName names fd, as Type.Method for methods.
func funcName(fd *ast.FuncDecl) string {
	if fd.Recv != nil && len(fd.Recv.List) > 0 {
		if id, ok := derefType(fd.Recv.List[0].Type).(*ast.Ident); ok {
			return id.Name + "." + fd.Name.Name
		}
	}
	return fd.Name.Name
}

// TestOnlyWalletWritesCoinBalances fails if any non-test Go file outside
// pkg/wallet writes users.coin_balance / users.frozen_coins or the
// coin_transactions ledger directly, except as walletWriteAllowlist allows.
// Use pkg/wallet instead. Test files are not scanned: their fixtures seed
// balances directly.
func TestOnlyWalletWritesCoinBalances(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(root, "go.mod"))
	require.NoError(t, err, "module root not found at %s", root)

	skipDirs := map[string]bool{
		filepath.Join(root, "pkg", "wallet"): true,
		filepath.Join(root, "api", "gen"):    true,
	}
	// matches[i] collects the findings that allowlist entry i covers.
	matches := make([][]walletWrite, len(walletWriteAllowlist))
	allowlisted := func(w walletWrite) bool {
		for i, a := range walletWriteAllowlist {
			if a.file == filepath.ToSlash(w.pos.Filename) && a.fn == w.fn && a.what == w.what {
				matches[i] = append(matches[i], w)
				return true
			}
		}
		return false
	}
	var violations []string
	files := 0
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[path] || (path != root && (strings.HasPrefix(d.Name(), ".") || d.Name() == "vendor" || d.Name() == "testdata")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, rel, src, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		files++
		for _, w := range scanWalletWrites(fset, file) {
			if !allowlisted(w) {
				violations = append(violations, w.String())
			}
		}
		return nil
	})
	require.NoError(t, err)
	require.Greater(t, files, 50, "scan found suspiciously few Go files under %s", root)
	for i, a := range walletWriteAllowlist {
		require.NotEmpty(t, a.why, "allowlist entry for %s in %s needs a reason", a.file, a.fn)
		if n := len(matches[i]); n == 0 {
			violations = append(violations, fmt.Sprintf("%s: in %s: allowlisted %q (%s) matched nothing; remove the entry", a.file, a.fn, a.what, a.why))
		} else if n > 1 {
			// Report every match: the scanner cannot tell which one is new.
			for _, w := range matches[i] {
				violations = append(violations, fmt.Sprintf("%s (allowlisted once, found %d times)", w, n))
			}
		}
	}
	if len(violations) > 0 {
		t.Fatalf("coin wallet writes must go through pkg/wallet (Debit, AdminDebit, Credit, Freeze, Unfreeze); "+
			"a new account's opening balance, or a read the scanner mistakes for a write, goes in walletWriteAllowlist:\n%s",
			strings.Join(violations, "\n"))
	}
}

// scanSnippet scans body as the body of a function in a file outside pkg/wallet.
func scanSnippet(t *testing.T, body string) []walletWrite {
	t.Helper()
	src := "package x\n\nfunc f(tx *gorm.DB, u *model.User) {\n" + body + "\n}\n"
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "x.go", src, parser.SkipObjectResolution)
	require.NoError(t, err, src)
	return scanWalletWrites(fset, file)
}

func TestScanWalletWritesCatchesDirectWrites(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		// SQL.
		{"UPDATE coin_balance", `tx.Exec("UPDATE users SET coin_balance = coin_balance + ? WHERE id = ?", 1, "u")`},
		{"UPDATE frozen_coins", `tx.Exec("UPDATE users SET frozen_coins = 0 WHERE id = ?", "u")`},
		{"UPDATE of a quoted column", "tx.Exec(\"UPDATE users SET `coin_balance` = `coin_balance` + ? WHERE id = ?\", 100, \"u\")"},
		{"UPDATE split across +", `tx.Exec("UPDATE users SET coin_balance" + " = coin_balance + ? WHERE id = ?", 100, "u")`},
		{"REPLACE INTO users", `tx.Exec("REPLACE INTO users (id, coin_balance) VALUES (?, ?)", "u", 1)`},
		{"INSERT INTO coin_transactions", `tx.Exec("INSERT INTO coin_transactions (id) VALUES (?)", "x")`},
		{"INSERT INTO a qualified ledger", "tx.Exec(\"INSERT INTO `golive`.`coin_transactions` (id) VALUES (?)\", \"x\")"},
		{"DELETE FROM coin_transactions", `tx.Exec("DELETE FROM coin_transactions WHERE id = ?", "x")`},
		{"column spliced in by Sprintf", `tx.Exec(fmt.Sprintf("UPDATE users SET %s = 0", "frozen_coins"))`},
		{"column in a constant", `
	const col = "coin_balance"
	tx.Exec("UPDATE users SET " + col + " = 0")`},
		// GORM column and field names.
		{"UpdateColumn by column", `tx.Model(u).UpdateColumn("coin_balance", gorm.Expr("coin_balance - 1"))`},
		{"Update by field", `tx.Model(u).Update("CoinBalance", gorm.Expr("coin_balance + 100"))`},
		{"Updates map keyed by column", `tx.Model(u).Updates(map[string]any{"frozen_coins": 0})`},
		{"Updates map keyed by field", `tx.Model(u).Updates(map[string]any{"CoinBalance": 1})`},
		{"Updates map filled by index", `
	m := map[string]any{}
	m["coin_balance"] = 1
	tx.Model(u).Updates(m)`},
		{"Select before Updates", `tx.Model(u).Select("coin_balance").Updates(u)`},
		{"clause.Set", `tx.Clauses(clause.Set{{Column: clause.Column{Name: "frozen_coins"}, Value: 0}})`},
		// Wallet fields.
		{"Updates with a User literal", `tx.Model(u).Updates(model.User{CoinBalance: 0, FrozenCoins: 0})`},
		{"assignment then Save", `
	u.CoinBalance += 5
	tx.Save(u)`},
		{"increment then Save", `
	u.CoinBalance++
	tx.Save(u)`},
		{"decrement", `u.FrozenCoins--`},
		{"write through a pointer", `
	p := &u.FrozenCoins
	*p = 0`},
		{"Save of a loaded User", `
	var user model.User
	tx.Take(&user, "id = ?", "u")
	user.DisplayName = "x"
	tx.Save(&user)`},
		{"Save of a User literal", `tx.Save(&model.User{ID: "u", DisplayName: "x"})`},
		{"Updates with a loaded User", `tx.Model(u).Updates(u)`},
		{"Updates of every User column", `tx.Model(u).Select("*").Updates(model.User{DisplayName: "x"})`},
		{"gorm tag mapping another field", `
	type balance struct{ Amount int64 ` + "`gorm:\"column:coin_balance\"`" + ` }
	tx.Table("users").Where("id = ?", "u").Updates(balance{Amount: 1})`},
		// Ledger rows.
		{"ledger row literal", `tx.Create(&model.CoinTransaction{ID: "x"})`},
		{"untyped slice elements", `tx.Create([]model.CoinTransaction{{ID: "x", Amount: 100}})`},
		{"untyped pointer elements", `
	rows := []*model.CoinTransaction{{ID: "x"}}
	tx.Create(&rows)`},
		{"row from var", `
	var row model.CoinTransaction
	row.Amount = 100
	tx.Create(&row)`},
		{"row from new", `
	row := new(model.CoinTransaction)
	row.Amount = 100
	tx.Create(row)`},
		{"wallet's row saved again", `
	row, _ := wallet.Credit(tx, "u", 1, wallet.Entry{})
	row.Amount = 1000
	tx.Save(row)`},
		{"renamed ledger type", `
	type ledger = model.CoinTransaction
	tx.Create(&ledger{ID: "x"})`},
		{"Create on Table(coin_transactions)", `tx.Table("coin_transactions").Create(map[string]any{"id": "y", "amount": 100})`},
		{"Create on a saved ledger query", `
	q := tx.Table("coin_transactions")
	q.Create(map[string]any{"id": "y"})`},
		{"Update on Model(CoinTransaction)", `tx.Model(&model.CoinTransaction{}).Where("id = ?", "x").Update("amount", 0)`},
		{"Delete of ledger rows", `tx.Where("user_id = ?", "u").Delete(&model.CoinTransaction{})`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NotEmpty(t, scanSnippet(t, tc.body), "not flagged:\n%s", tc.body)
		})
	}
}

func TestScanWalletWritesAllowsReads(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"SELECT", `tx.Raw("SELECT coin_balance, COALESCE(frozen_coins, 0) FROM users WHERE coin_balance >= ?", 1)`},
		{"SELECT FOR UPDATE", `tx.Raw("SELECT coin_balance FROM users WHERE id = ? FOR UPDATE", "u").Row().Scan(&u.CoinBalance)`},
		{"ledger subquery", `tx.Table("users AS u").Select("(SELECT SUM(ct.amount) FROM coin_transactions AS ct WHERE ct.user_id = u.id) AS topup").Scan(&rows)`},
		{"ledger count", `tx.Model(&model.CoinTransaction{}).Where("user_id = ?", "u").Count(&n)`},
		{"ledger sum", `tx.Table("coin_transactions").Select("COALESCE(SUM(amount), 0)").Row().Scan(&n)`},
		{"saved ledger query", `
	q := tx.Table("coin_transactions ct").Joins("LEFT JOIN users u ON u.id = ct.user_id")
	q = q.Where("ct.type = ?", "topup")
	q.Count(&n)
	q.Order("ct.created_at DESC").Scan(&items)`},
		{"rows read into variables", `
	var rows []model.CoinTransaction
	tx.Where("user_id = ?", "u").Find(&rows)
	var coinTx model.CoinTransaction
	tx.Where("id = ?", "x").Take(&coinTx)`},
		{"wallet's row kept", `
	var coinTx model.CoinTransaction
	row, err := wallet.Debit(tx, "u", 5, wallet.Entry{Type: "gift_spend", Title: "Gift"})
	if err == nil {
		coinTx = *row
	}`},
		{"ledger migration", `tx.AutoMigrate(&model.CoinTransaction{})`},
		{"wallet fields read", `
	if u.CoinBalance-u.FrozenCoins < price {
		c.JSON(402, gin.H{"coinBalance": u.CoinBalance})
	}`},
		{"columns named, not written", `
	tx.Model(&model.User{}).Order("coin_balance").Pluck("coin_balance", &balances)
	tx.Omit("coin_balance", "frozen_coins").Create(u)`},
		{"other user columns", `
	tx.Model(u).Updates(map[string]any{"display_name": "x", "banned": false})
	tx.Model(u).Updates(model.User{DisplayName: "x"})
	tx.Exec("UPDATE users SET banned = true WHERE id = ?", "u")`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			found := scanSnippet(t, tc.body)
			require.Empty(t, found, "flagged:\n%s\nin:\n%s", fmt.Sprint(found), tc.body)
		})
	}
}
