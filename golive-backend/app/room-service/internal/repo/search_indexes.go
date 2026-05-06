package repo

import (
	"strings"

	"gorm.io/gorm"
)

func ensureMySQLFullTextIndexes(db *gorm.DB, specs ...fullTextIndexSpec) error {
	if db == nil || db.Dialector.Name() != "mysql" {
		return nil
	}
	for _, spec := range specs {
		if err := ensureMySQLFullTextIndex(db, spec); err != nil {
			if isMissingTable(err) {
				continue
			}
			return err
		}
	}
	return nil
}

func ensureMySQLIndex(db *gorm.DB, table, name, ddl string) error {
	if db == nil || db.Dialector.Name() != "mysql" {
		return nil
	}
	var count int64
	err := db.Raw(`
SELECT COUNT(*)
FROM information_schema.statistics
WHERE table_schema = DATABASE()
  AND table_name = ?
  AND index_name = ?
`, table, name).Scan(&count).Error
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return db.Exec(ddl).Error
}

type fullTextIndexSpec struct {
	Table   string
	Name    string
	Columns []string
}

func ensureMySQLFullTextIndex(db *gorm.DB, spec fullTextIndexSpec) error {
	return ensureMySQLIndex(
		db,
		spec.Table,
		spec.Name,
		"CREATE FULLTEXT INDEX "+spec.Name+" ON "+spec.Table+" ("+strings.Join(spec.Columns, ", ")+")",
	)
}

func sharedSearchFullTextIndexes() []fullTextIndexSpec {
	return []fullTextIndexSpec{
		{
			Table:   "rooms",
			Name:    "ft_rooms_search",
			Columns: []string{"title", "title_ja", "description", "category", "category_ja", "channel", "channel_id"},
		},
		{
			Table:   "users",
			Name:    "ft_users_search",
			Columns: []string{"username", "display_name", "id"},
		},
	}
}
