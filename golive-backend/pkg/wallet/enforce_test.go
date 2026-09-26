package wallet_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// SQL that changes a wallet column or the ledger. Reads (SELECT, WHERE
// coin_balance >= ?) do not match.
var walletWriteSQL = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(coin_balance|frozen_coins)\s*=[^=]`),
	regexp.MustCompile("(?i)\\b(insert|replace)\\s+(ignore\\s+)?into\\s+`?coin_transactions\\b"),
	regexp.MustCompile("(?i)\\bupdate\\s+`?coin_transactions\\b"),
	regexp.MustCompile("(?i)\\bdelete\\s+from\\s+`?coin_transactions\\b"),
}

var walletColumns = map[string]bool{"coin_balance": true, "frozen_coins": true}
var walletFields = map[string]bool{"CoinBalance": true, "FrozenCoins": true}

// scanWalletWrites reports every place in src that writes the coin wallet
// without going through pkg/wallet.
func scanWalletWrites(fset *token.FileSet, file *ast.File) []string {
	var found []string
	report := func(n ast.Node, what string) {
		found = append(found, fmt.Sprintf("%s: %s", fset.Position(n.Pos()), what))
	}
	stringLit := func(e ast.Expr) (string, bool) {
		lit, ok := e.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(lit.Value)
		return s, err == nil
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.BasicLit:
			if s, ok := stringLit(n); ok {
				for _, re := range walletWriteSQL {
					if re.MatchString(s) {
						report(n, "SQL writes the coin wallet: "+strings.Join(strings.Fields(s), " "))
						break
					}
				}
			}
		case *ast.CallExpr:
			// db.Update("coin_balance", ...) / db.UpdateColumn(...)
			sel, ok := n.Fun.(*ast.SelectorExpr)
			if ok && (sel.Sel.Name == "Update" || sel.Sel.Name == "UpdateColumn") && len(n.Args) > 0 {
				if s, ok := stringLit(n.Args[0]); ok && walletColumns[s] {
					report(n, sel.Sel.Name+"("+strconv.Quote(s)+")")
				}
			}
		case *ast.KeyValueExpr:
			// map[string]any{"coin_balance": ...} passed to Updates.
			if s, ok := stringLit(n.Key); ok && walletColumns[s] {
				report(n, "map key "+strconv.Quote(s))
			}
		case *ast.AssignStmt:
			// u.CoinBalance = ... followed by Save(&u).
			for _, lhs := range n.Lhs {
				if sel, ok := lhs.(*ast.SelectorExpr); ok && walletFields[sel.Sel.Name] {
					report(n, "assignment to ."+sel.Sel.Name)
				}
			}
		case *ast.CompositeLit:
			// A non-empty ledger row literal is only built to be inserted.
			name := ""
			switch t := n.Type.(type) {
			case *ast.Ident:
				name = t.Name
			case *ast.SelectorExpr:
				name = t.Sel.Name
			}
			if name == "CoinTransaction" && len(n.Elts) > 0 {
				report(n, "coin_transactions row built outside pkg/wallet")
			}
		}
		return true
	})
	return found
}

// TestOnlyWalletWritesCoinBalances fails if any non-test Go file outside
// pkg/wallet writes users.coin_balance / users.frozen_coins or the
// coin_transactions ledger directly. Use pkg/wallet instead.
func TestOnlyWalletWritesCoinBalances(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(root, "go.mod"))
	require.NoError(t, err, "module root not found at %s", root)

	skipDirs := map[string]bool{
		filepath.Join(root, "pkg", "wallet"): true,
		filepath.Join(root, "api", "gen"):    true,
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
		violations = append(violations, scanWalletWrites(fset, file)...)
		return nil
	})
	require.NoError(t, err)
	require.Greater(t, files, 50, "scan found suspiciously few Go files under %s", root)
	if len(violations) > 0 {
		t.Fatalf("coin wallet writes must go through pkg/wallet:\n%s", strings.Join(violations, "\n"))
	}
}

func TestScanWalletWritesCatchesDirectWrites(t *testing.T) {
	const src = `package x

func bad(tx *gorm.DB, u *model.User) {
	tx.Exec("UPDATE users SET coin_balance = coin_balance + ? WHERE id = ?", 1, "u")
	tx.Exec("UPDATE users SET frozen_coins = 0 WHERE id = ?", "u")
	tx.Exec("INSERT INTO coin_transactions (id) VALUES (?)", "x")
	tx.Exec("DELETE FROM coin_transactions WHERE id = ?", "x")
	tx.Model(u).UpdateColumn("coin_balance", gorm.Expr("coin_balance - 1"))
	tx.Model(u).Updates(map[string]any{"frozen_coins": 0})
	u.CoinBalance += 5
	tx.Create(&model.CoinTransaction{ID: "x"})
}

func fine(tx *gorm.DB) {
	tx.Raw("SELECT coin_balance, COALESCE(frozen_coins, 0) FROM users WHERE coin_balance >= ?", 1)
	tx.Model(&model.CoinTransaction{}).Where("user_id = ?", "u").Count(nil)
	tx.Table("coin_transactions").Select("COALESCE(SUM(amount), 0)")
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "x.go", src, 0)
	require.NoError(t, err)
	found := scanWalletWrites(fset, file)
	require.Len(t, found, 8, strings.Join(found, "\n"))
	for i, line := range []int{4, 5, 6, 7, 8, 9, 10, 11} {
		require.Contains(t, found[i], fmt.Sprintf("x.go:%d:", line))
	}
}
