package mattermost

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	"github.com/mattermost/mattermost-operator/pkg/database"
	corev1 "k8s.io/api/core/v1"
)

// defaultBuiltinReadinessTimeout is the default for
// `mattermost db ping --timeout` when the user does not supply one.
const defaultBuiltinReadinessTimeout = 5 * time.Minute

type ExternalDBConfig struct {
	secretName               string
	dbType                   string
	hasReaderEndpoints       bool
	hasDBCheckURL            bool
	hasSeparateDatasourceKey bool
}

func NewExternalDBConfig(mattermost *mmv1beta.Mattermost, secret corev1.Secret) (*ExternalDBConfig, error) {
	if mattermost.Spec.Database.External == nil {
		return nil, errors.New("external database config not provided")
	}
	if mattermost.Spec.Database.External.Secret == "" {
		return nil, errors.New("external database Secret not provided")
	}

	connectionStr, ok := secret.Data["DB_CONNECTION_STRING"]
	if !ok {
		return nil, errors.New("external database Secret does not containt DB_CONNECTION_STRING key")
	}
	if len(connectionStr) == 0 {
		return nil, errors.New("external database connection string is empty")
	}

	externalDB := &ExternalDBConfig{
		secretName: mattermost.Spec.Database.External.Secret,
		dbType:     database.GetTypeFromConnectionString(string(connectionStr)),
	}

	if _, ok := secret.Data["MM_SQLSETTINGS_DATASOURCEREPLICAS"]; ok {
		externalDB.hasReaderEndpoints = true
	}
	if checkURL, ok := secret.Data["DB_CONNECTION_CHECK_URL"]; ok {
		if err := validateDBCheckURL(string(checkURL), externalDB.dbType); err != nil {
			return nil, fmt.Errorf("invalid DB_CONNECTION_CHECK_URL: %w", err)
		}
		externalDB.hasDBCheckURL = true
	}
	if _, ok := secret.Data["MM_SQLSETTINGS_DATASOURCE"]; ok {
		externalDB.hasSeparateDatasourceKey = true
	}

	return externalDB, nil
}

func (e *ExternalDBConfig) EnvVars(_ *mmv1beta.Mattermost) []corev1.EnvVar {
	dbEnvVars := []corev1.EnvVar{
		{
			Name:      "MM_CONFIG",
			ValueFrom: EnvSourceFromSecret(e.secretName, "DB_CONNECTION_STRING"),
		},
	}

	// If the secret has a separate MM_SQLSETTINGS_DATASOURCE key (without protocol prefix),
	// use it. Otherwise fall back to DB_CONNECTION_STRING for backward compatibility.
	datasourceKey := "DB_CONNECTION_STRING"
	if e.hasSeparateDatasourceKey {
		datasourceKey = "MM_SQLSETTINGS_DATASOURCE"
	}

	dbEnvVars = append(dbEnvVars, corev1.EnvVar{
		Name:      "MM_SQLSETTINGS_DATASOURCE",
		ValueFrom: EnvSourceFromSecret(e.secretName, datasourceKey),
	})

	if e.hasReaderEndpoints {
		dbEnvVars = append(dbEnvVars, corev1.EnvVar{
			Name:      "MM_SQLSETTINGS_DATASOURCEREPLICAS",
			ValueFrom: EnvSourceFromSecret(e.secretName, "MM_SQLSETTINGS_DATASOURCEREPLICAS"),
		})
	}

	return dbEnvVars
}

func (e *ExternalDBConfig) InitContainers(mattermost *mmv1beta.Mattermost) []corev1.Container {
	if mattermost.Spec.Database.DisableReadinessCheck {
		return nil
	}

	mode := mmv1beta.DatabaseReadinessCheckModeExternal
	if rc := mattermost.Spec.Database.ReadinessCheck; rc != nil && rc.Mode != "" {
		mode = rc.Mode
	}

	// External mode requires the optional DB_CONNECTION_CHECK_URL secret key.
	// Builtin mode uses MM_CONFIG/MM_SQLSETTINGS_DATASOURCE and does not
	// require that key.
	if mode == mmv1beta.DatabaseReadinessCheckModeExternal && !e.hasDBCheckURL {
		return nil
	}

	container := getDBCheckInitContainer(mattermost, e.secretName, e.dbType, e.hasSeparateDatasourceKey)
	if container == nil {
		return nil
	}
	return []corev1.Container{*container}
}

// getDBCheckInitContainer prepares the init container that checks database
// readiness. It dispatches between the legacy external-image mode and the
// builtin mode that reuses the Mattermost image to run `mattermost db ping`.
// Returns nil if no init container can be produced (for example, an unknown
// database type when running in external mode).
func getDBCheckInitContainer(
	mattermost *mmv1beta.Mattermost,
	secretName, dbType string,
	hasSeparateDatasourceKey bool,
) *corev1.Container {
	mode := mmv1beta.DatabaseReadinessCheckModeExternal
	timeout := defaultBuiltinReadinessTimeout
	if rc := mattermost.Spec.Database.ReadinessCheck; rc != nil {
		if rc.Mode != "" {
			mode = rc.Mode
		}
		if rc.Timeout != nil {
			timeout = rc.Timeout.Duration
		}
	}

	if mode == mmv1beta.DatabaseReadinessCheckModeBuiltin {
		return builtinDBCheckInitContainer(mattermost, secretName, hasSeparateDatasourceKey, timeout)
	}
	return externalDBCheckInitContainer(secretName, dbType)
}

// externalDBCheckInitContainer returns the legacy postgres:13 / curl-based
// init container. Behavior must remain byte-for-byte identical to the
// pre-readinessCheck implementation.
func externalDBCheckInitContainer(secretName, dbType string) *corev1.Container {
	envVars := []corev1.EnvVar{
		{
			Name:      "DB_CONNECTION_CHECK_URL",
			ValueFrom: EnvSourceFromSecret(secretName, "DB_CONNECTION_CHECK_URL"),
		},
	}

	switch dbType {
	case database.MySQLDatabase:
		return &corev1.Container{
			Name:            "init-check-database",
			Image:           "appropriate/curl:latest",
			ImagePullPolicy: corev1.PullIfNotPresent,
			Env:             envVars,
			Command: []string{
				"sh", "-c",
				`until curl --max-time 5 "$DB_CONNECTION_CHECK_URL"; do echo waiting for database; sleep 5; done;`,
			},
		}
	case database.PostgreSQLDatabase:
		return &corev1.Container{
			Name:            "init-check-database",
			Image:           "postgres:13",
			ImagePullPolicy: corev1.PullIfNotPresent,
			Env:             envVars,
			Command: []string{
				"sh", "-c",
				`until pg_isready --dbname="$DB_CONNECTION_CHECK_URL"; do echo waiting for database; sleep 5; done;`,
			},
		}
	}

	return nil
}

// builtinDBCheckInitContainer returns an init container built from the same
// Mattermost image as the main container. It runs `mattermost db ping` with
// the configured timeout, removing the runtime dependency on postgres:13 /
// appropriate/curl.
func builtinDBCheckInitContainer(
	mattermost *mmv1beta.Mattermost,
	secretName string,
	hasSeparateDatasourceKey bool,
	timeout time.Duration,
) *corev1.Container {
	datasourceKey := "DB_CONNECTION_STRING"
	if hasSeparateDatasourceKey {
		datasourceKey = "MM_SQLSETTINGS_DATASOURCE"
	}

	envVars := []corev1.EnvVar{
		{
			Name:      "MM_CONFIG",
			ValueFrom: EnvSourceFromSecret(secretName, "DB_CONNECTION_STRING"),
		},
		{
			Name:      "MM_SQLSETTINGS_DATASOURCE",
			ValueFrom: EnvSourceFromSecret(secretName, datasourceKey),
		},
	}

	return &corev1.Container{
		Name:            "init-check-database",
		Image:           mattermost.GetImageName(),
		ImagePullPolicy: mattermost.Spec.ImagePullPolicy,
		Env:             envVars,
		Command:         []string{"/mattermost/bin/mattermost"},
		Args: []string{
			"db", "ping",
			fmt.Sprintf("--timeout=%s", timeout),
		},
	}
}

// allowedDBCheckSchemesByType defines URL schemes permitted per database type.
// MySQL uses curl for readiness checks (http/https only); PostgreSQL uses pg_isready (postgres URI).
var allowedDBCheckSchemesByType = map[string]map[string]bool{
	database.MySQLDatabase:      {"http": true, "https": true},
	database.PostgreSQLDatabase: {"http": true, "https": true, "postgres": true},
	"unknown":                   {"http": true, "https": true, "mysql": true, "postgres": true},
}

func isAllowedDBCheckScheme(dbType, scheme string) bool {
	schemes, ok := allowedDBCheckSchemesByType[dbType]
	if !ok {
		schemes = allowedDBCheckSchemesByType["unknown"]
	}
	return schemes[scheme]
}

// validateDBCheckURL validates that a DB connection check URL uses an allowed
// scheme for the given database type.
func validateDBCheckURL(rawURL, dbType string) error {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return errors.New("URL is empty")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("failed to parse URL: %w", err)
	}

	scheme := strings.ToLower(parsed.Scheme)
	if !isAllowedDBCheckScheme(dbType, scheme) {
		return fmt.Errorf("scheme %q is not allowed for database type %q", scheme, dbType)
	}

	if parsed.Hostname() == "" {
		return errors.New("URL must contain a hostname")
	}

	return nil
}
