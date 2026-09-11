package mysqlstore

import (
	"time"

	scssql "github.com/alexedwards/scs/mysqlstore"
)

func (store *Store) NewManagementSessionStore(cleanupInterval time.Duration) *scssql.MySQLStore {
	return scssql.NewWithConfig(store.db, scssql.Config{
		CleanUpInterval: cleanupInterval,
		TableName:       "management_sessions",
	})
}
