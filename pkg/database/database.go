package database

import (
	"errors"

	corev1 "k8s.io/api/core/v1"
)

// Info contains information on a database connection.
type Info struct {
	SecretName       string
	DatabaseName     string
	External         bool
	ReaderEndpoints  bool
	DatabaseCheckURL bool

	// These values should never be directly accessed or used. They are set only
	// to ensure the database info is valid.
	rootPassword string
	userName     string
	userPassword string
}

// IsValid returns if the database Info is valid or not.
func (db *Info) IsValid() error {
	if len(db.SecretName) == 0 {
		return errors.New("database secret name shouldn't be empty")
	}

	if db.External {
		return nil
	}

	if len(db.rootPassword) == 0 {
		return errors.New("database root password shouldn't be empty")
	}
	if len(db.userName) == 0 {
		return errors.New("database username shouldn't be empty")
	}
	if len(db.userPassword) == 0 {
		return errors.New("database password shouldn't be empty")
	}
	if len(db.DatabaseName) == 0 {
		return errors.New("database name shouldn't be empty")
	}

	return nil
}

// IsExternal defines if the database is external or not.
func (db *Info) IsExternal() bool {
	return db.External
}

// HasReaderEndpoints returns if the database has reader endpoints defined.
func (db *Info) HasReaderEndpoints() bool {
	return db.ReaderEndpoints
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
		databaseInfo := &Info{
			SecretName: secret.Name,
			External:   true,
		}

		if _, ok := secret.Data["MM_SQLSETTINGS_DATASOURCEREPLICAS"]; ok {
			databaseInfo.ReaderEndpoints = true
		}

		if _, ok := secret.Data["DB_CONNECTION_CHECK_URL"]; ok {
			// The optional endpoint check was provided.
			databaseInfo.DatabaseCheckURL = true
		}

		return databaseInfo
	}

	return &Info{
		SecretName:       secret.Name,
		External:         false,
		ReaderEndpoints:  true,
		DatabaseCheckURL: true,
		rootPassword:     string(secret.Data["ROOT_PASSWORD"]),
		userName:         string(secret.Data["USER"]),
		userPassword:     string(secret.Data["PASSWORD"]),
		DatabaseName:     string(secret.Data["DATABASE"]),
	}
}
