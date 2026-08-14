// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package v1beta1

import (
	"github.com/pkg/errors"
)

// Database utils

// SetDefaults validates the database configuration.
//
// There is deliberately no default. Until Operator v2 an unconfigured database
// silently became a MySQL cluster provisioned through the presslabs MySQL
// operator; that integration has been removed, so a database now has to be
// supplied explicitly.
func (db *Database) SetDefaults() error {
	if db.IsExternal() {
		return nil
	}

	return errors.New("a database is required: configure database.external with a secret holding the connection string")
}

// IsExternal returns true if the Database is set to external.
func (db *Database) IsExternal() bool {
	return db.External != nil && db.External.Secret != ""
}
