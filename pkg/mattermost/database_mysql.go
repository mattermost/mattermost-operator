package mattermost

import (
	"fmt"

	mattermostv1beta1 "github.com/mattermost/mattermost-operator/apis/mattermost/v1beta1"
	"github.com/mattermost/mattermost-operator/pkg/components/utils"
	corev1 "k8s.io/api/core/v1"
)

type MySQLDBConfig struct {
	secretName   string
	rootPassword string
	userName     string
	userPassword string
	databaseName string
}

func NewMySQLDB(secret corev1.Secret) (*MySQLDBConfig, error) {
	rootPassword := string(secret.Data["ROOT_PASSWORD"])
	if rootPassword == "" {
		return nil, fmt.Errorf("database root password shouldn't be empty")
	}
	userName := string(secret.Data["USER"])
	if userName == "" {
		return nil, fmt.Errorf("database username shouldn't be empty")
	}
	userPassword := string(secret.Data["PASSWORD"])
	if userPassword == "" {
		return nil, fmt.Errorf("database password shouldn't be empty")
	}
	databaseName := string(secret.Data["DATABASE"])
	if databaseName == "" {
		return nil, fmt.Errorf("database name shouldn't be empty")
	}

	return &MySQLDBConfig{
		secretName:   secret.Name,
		rootPassword: rootPassword,
		userName:     userName,
		userPassword: userPassword,
		databaseName: databaseName,
	}, nil

}

func (m *MySQLDBConfig) EnvVars(mattermost *mattermostv1beta1.Mattermost) []corev1.EnvVar {
	mysqlName := utils.HashWithPrefix("db", mattermost.Name)

	dbEnvVars := []corev1.EnvVar{
		{
			Name:      "MYSQL_USERNAME",
			ValueFrom: EnvSourceFromSecret(m.secretName, "USER"),
		},
		{
			Name:      "MYSQL_PASSWORD",
			ValueFrom: EnvSourceFromSecret(m.secretName, "PASSWORD"),
		},
		{
			Name: "MM_SQLSETTINGS_DATASOURCEREPLICAS",
			Value: fmt.Sprintf(
				"$(MYSQL_USERNAME):$(MYSQL_PASSWORD)@tcp(%s-mysql.%s:3306)/%s?readTimeout=30s&writeTimeout=30s",
				mysqlName, mattermost.Namespace, m.databaseName,
			),
		},
		{
			Name: "MM_CONFIG",
			Value: fmt.Sprintf(
				"mysql://$(MYSQL_USERNAME):$(MYSQL_PASSWORD)@tcp(%s-mysql-master.%s:3306)/%s?charset=utf8mb4,utf8&readTimeout=30s&writeTimeout=30s",
				mysqlName, mattermost.Namespace, m.databaseName,
			),
		},
	}

	return dbEnvVars
}

func (m *MySQLDBConfig) InitContainers(mattermost *mattermostv1beta1.Mattermost) []corev1.Container {
	mysqlName := utils.HashWithPrefix("db", mattermost.Name)

	return []corev1.Container{
		{
			Name:            "init-check-operator-mysql",
			Image:           "appropriate/curl:latest",
			ImagePullPolicy: corev1.PullIfNotPresent,
			Command: []string{
				"sh", "-c",
				fmt.Sprintf("until curl --max-time 5 http://%s-mysql-master.%s:3306; do echo waiting for mysql; sleep 5; done;",
					mysqlName, mattermost.Namespace,
				),
			},
		},
	}
}
