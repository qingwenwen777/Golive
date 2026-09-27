package repo

import "gorm.io/gorm"

// MaxPage is the deepest page a paginated list serves. A deeper page is empty
// rather than a deep OFFSET scan; the handlers cap ?page= just past it.
const MaxPage = 1000

// pageWindow skips to page's rows (OFFSET). A page past MaxPage selects
// nothing, which the database answers without scanning; the caller's Limit
// still applies.
func pageWindow(page, size int) func(*gorm.DB) *gorm.DB {
	return func(tx *gorm.DB) *gorm.DB {
		if page > MaxPage {
			return tx.Where("1 = 0")
		}
		return tx.Offset((page - 1) * size)
	}
}
