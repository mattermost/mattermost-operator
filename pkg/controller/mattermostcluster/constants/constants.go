package constants

const (
	// OperatorName is a operator name
	OperatorName = "mattermost-operator"
	// DefaultAmountOfPods is the default amount of Mattermost pods
	DefaultAmountOfPods = 2
	// DefaultMattermostImage is the default Mattermost docker image
	DefaultMattermostImage        = "mattermost/mattermost-enterprise-edition:5.8.0"
	DefaultMattermostDatabaseType = "mysql"

	ClusterLabel = "v1alpha1.mattermost.com/cluster"
)
