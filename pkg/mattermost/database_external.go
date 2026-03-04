package mattermost

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	mmv1beta "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	"github.com/mattermost/mattermost-operator/pkg/database"
	corev1 "k8s.io/api/core/v1"
)

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
		if err := validateDBCheckURL(string(checkURL)); err != nil {
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

	var initContainers []corev1.Container
	if e.hasDBCheckURL {
		container := getDBCheckInitContainer(e.secretName, e.dbType)
		if container != nil {
			initContainers = append(initContainers, *container)
		}
	}

	return initContainers
}

// getDBCheckInitContainer prepares init container that checks database readiness based on db type.
// Returns nil if database type is unknown.
func getDBCheckInitContainer(secretName, dbType string) *corev1.Container {
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

// allowedDBCheckSchemes defines the URL schemes permitted for database connection check URLs.
var allowedDBCheckSchemes = map[string]bool{
	"http":     true,
	"https":    true,
	"mysql":    true,
	"postgres": true,
}

// metadataIPBlocks contains IP ranges commonly used for cloud metadata services.
var metadataIPBlocks []*net.IPNet

func init() {
	for _, cidr := range []string{
		"169.254.169.254/32", // AWS, GCP, Azure metadata
		"100.100.100.200/32", // Alibaba metadata
		"fd00:ec2::254/128",  // AWS IPv6 metadata
	} {
		_, block, _ := net.ParseCIDR(cidr)
		metadataIPBlocks = append(metadataIPBlocks, block)
	}
}

// validateDBCheckURL validates that a DB connection check URL uses an allowed
// scheme and does not target cloud metadata endpoints.
func validateDBCheckURL(rawURL string) error {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return errors.New("URL is empty")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("failed to parse URL: %w", err)
	}

	scheme := strings.ToLower(parsed.Scheme)
	if !allowedDBCheckSchemes[scheme] {
		return fmt.Errorf("scheme %q is not allowed; permitted schemes: http, https, mysql, postgres", scheme)
	}

	hostname := parsed.Hostname()
	if hostname == "" {
		return errors.New("URL must contain a hostname")
	}

	// Check if the hostname resolves to a metadata IP.
	if ip := net.ParseIP(hostname); ip != nil {
		for _, block := range metadataIPBlocks {
			if block.Contains(ip) {
				return fmt.Errorf("URL targets a blocked metadata IP range: %s", hostname)
			}
		}
	}

	return nil
}
