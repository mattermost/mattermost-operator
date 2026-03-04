package mattermost

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

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
// scheme for the given database type and does not target cloud metadata endpoints.
// For hostnames, it resolves DNS and blocks any IP in metadata ranges.
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

	hostname := parsed.Hostname()
	if hostname == "" {
		return errors.New("URL must contain a hostname")
	}

	ips, err := resolveHostnameIPs(hostname)
	if err != nil {
		return fmt.Errorf("failed to resolve hostname %q: %w", hostname, err)
	}
	for _, ip := range ips {
		for _, block := range metadataIPBlocks {
			if block.Contains(ip) {
				return fmt.Errorf("URL targets a blocked metadata IP range: %s", hostname)
			}
		}
	}

	return nil
}

// resolveHostnameIPs returns IPs for a hostname. If hostname is a literal IP,
// returns it; otherwise performs DNS lookup with timeout.
func resolveHostnameIPs(hostname string) ([]net.IP, error) {
	if ip := net.ParseIP(hostname); ip != nil {
		return []net.IP{ip}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, hostname)
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, a := range addrs {
		ips = append(ips, a.IP)
	}
	return ips, nil
}
