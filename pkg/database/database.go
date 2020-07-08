package database

import (
	"errors"

	corev1 "k8s.io/api/core/v1"
)

// Info contains information on a database connection.
type Info struct {
	External         bool
	DatabaseCheckURL bool
	rootPassword     string
	UserName         string
	UserPassword     string
	DatabaseName     string
}

// IsValid returns if the database Info is valid or not.
func (db *Info) IsValid() error {
	if db.External {
		return nil
	}

	if len(db.rootPassword) == 0 {
		return errors.New("database root password shouldn't be empty")
	}
	if len(db.UserName) == 0 {
		return errors.New("database username shouldn't be empty")
	}
	if len(db.UserPassword) == 0 {
		return errors.New("database password shouldn't be empty")
	}
	if len(db.DatabaseName) == 0 {
		return errors.New("database name shouldn't be empty")
	}

	return nil
}

// IsExternal defines if the database is external or not
func (db *Info) IsExternal() bool {
	return db.External
}

// HasDatabaseCheckURL returns if the database has an endpoint check defined.
func (db *Info) HasDatabaseCheckURL() bool {
	return db.DatabaseCheckURL
}

// GenerateDatabaseInfoFromSecret takes a secret and returns database based on
// the characteristics of the secret.
func GenerateDatabaseInfoFromSecret(secret *corev1.Secret) *Info {
	if _, ok := secret.Data["DB_CONNECTION_STRING"]; ok {
		// This is a secret for an external database.
		databaseInfo := &Info{External: true}

		if _, ok := secret.Data["DB_CONNECTION_CHECK_URL"]; ok {
			// The optional endpoint check was provided.
			databaseInfo.DatabaseCheckURL = true
		}

		return databaseInfo
	}

	return &Info{
		External:         false,
		DatabaseCheckURL: true,
		rootPassword:     string(secret.Data["ROOT_PASSWORD"]),
		UserName:         string(secret.Data["USER"]),
		UserPassword:     string(secret.Data["PASSWORD"]),
		DatabaseName:     string(secret.Data["DATABASE"]),
	}
}
