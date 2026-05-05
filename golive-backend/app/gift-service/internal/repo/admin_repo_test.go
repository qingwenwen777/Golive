package repo

import (
	"strings"
	"testing"
)

func TestAdminOrderPartsUseMySQLSafeBetOptionAlias(t *testing.T) {
	sql := strings.Join(adminOrderParts("all"), "\n")

	if strings.Contains(sql, " AS option") {
		t.Fatal("admin order SQL must not alias a column as MySQL reserved word option")
	}
	if !strings.Contains(sql, "bw.`option` AS bet_option") {
		t.Fatal("admin order SQL should expose bet_wagers option as bet_option for scanning")
	}
}
