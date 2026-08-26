// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package v1beta1

import (
	"github.com/pkg/errors"
)

// Database utils

// IsValid validates the database configuration.
// A database must be explicitly configured; there is no default.
func (db *Database) IsValid() error {
	if db.IsExternal() {
		return nil
	}

	return errors.New("a database is required: configure database.external with a secret holding the connection string")
}

// IsExternal returns true if the Database is set to external.
func (db *Database) IsExternal() bool {
	return db.External != nil && db.External.Secret != ""
}
