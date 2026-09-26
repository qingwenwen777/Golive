package repo

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// mysqlTestDSNEnv names a server-level DSN such as
// root:root@tcp(127.0.0.1:3306)/?parseTime=true&loc=UTC. Tests that need real
// InnoDB locking skip when it is unset.
const mysqlTestDSNEnv = "GOLIVE_TEST_MYSQL_DSN"

// openTestMySQL creates a uniquely named database on the server from
// GOLIVE_TEST_MYSQL_DSN, returns a connection to it and drops it on cleanup,
// so parallel packages sharing one server never see each other's tables.
func openTestMySQL(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv(mysqlTestDSNEnv))
	if dsn == "" {
		t.Skipf("%s not set; skipping MySQL-backed test", mysqlTestDSNEnv)
	}
	serverDSN, dbDSN, name := mysqlTestDSNs(t, dsn)

	cfg := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
	admin, err := gorm.Open(mysql.Open(serverDSN), cfg)
	require.NoError(t, err)
	adminSQL, err := admin.DB()
	require.NoError(t, err)
	require.NoError(t, admin.Exec("CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4").Error)
	t.Cleanup(func() {
		_ = admin.Exec("DROP DATABASE IF EXISTS `" + name + "`").Error
		_ = adminSQL.Close()
	})

	db, err := gorm.Open(mysql.Open(dbDSN), cfg)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	// Close before the DROP DATABASE cleanup above runs (cleanups are LIFO).
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

// mysqlTestDSNs splits dsn (go-sql-driver format) at its database path and
// returns a server DSN without a database, a DSN for a fresh random database,
// and that database's name.
func mysqlTestDSNs(t *testing.T, dsn string) (string, string, string) {
	t.Helper()
	slash := strings.LastIndex(dsn, "/")
	require.GreaterOrEqual(t, slash, 0, "%s must look like user:pass@tcp(host:port)/[db][?params]", mysqlTestDSNEnv)
	prefix := dsn[:slash+1]
	params := ""
	if q := strings.IndexByte(dsn[slash+1:], '?'); q >= 0 {
		params = dsn[slash+1+q:]
	}
	if !strings.Contains(params, "parseTime=") {
		if params == "" {
			params = "?parseTime=true"
		} else {
			params += "&parseTime=true"
		}
	}

	buf := make([]byte, 6)
	_, err := rand.Read(buf)
	require.NoError(t, err)
	name := "golive_test_usersvc_" + hex.EncodeToString(buf)
	return prefix + params, prefix + name + params, name
}
