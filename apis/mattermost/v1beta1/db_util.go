package v1beta1

import "github.com/mattermost/mattermost-operator/pkg/utils"

// Database utils

// SetDefaults sets the missing values in Database to the default ones.
func (db *Database) SetDefaults() {
	if db.IsExternal() {
		return
	}

	if db.OperatorManaged == nil {
		db.OperatorManaged = &OperatorManagedDatabase{}
	}

	db.OperatorManaged.SetDefaults()
}

// IsExternal returns true if the Database is set to external.
func (db *Database) IsExternal() bool {
	return db.External != nil && db.External.Secret != ""
}

func (db *Database) ensureDefault() {
	if db.OperatorManaged == nil {
		db.OperatorManaged = &OperatorManagedDatabase{}
	}
}

// SetDefaults sets the missing values in OperatorManagedDatabase to the default ones.
func (omd *OperatorManagedDatabase) SetDefaults() {
	if omd.Type == "" {
		omd.Type = DefaultMattermostDatabaseType
	}
	if omd.StorageSize == "" {
		omd.StorageSize = DefaultStorageSize
	}
}

func (db *Database) SetDefaultReplicasAndResources() {
	if db.IsExternal() {
		return
	}
	db.ensureDefault()
	db.OperatorManaged.SetDefaultReplicasAndResources()
}

func (omd *OperatorManagedDatabase) SetDefaultReplicasAndResources() {
	if omd.Replicas == nil {
		omd.Replicas = &defaultSize.Database.Replicas
	}
	if omd.Resources.Size() == 0 {
		omd.Resources = defaultSize.Database.Resources
	}
}

func (db *Database) OverrideReplicasAndResourcesFromSize(size MattermostSize) {
	if db.IsExternal() {
		return
	}
	db.ensureDefault()
	db.OperatorManaged.OverrideReplicasAndResourcesFromSize(size)
}

func (omd *OperatorManagedDatabase) OverrideReplicasAndResourcesFromSize(size MattermostSize) {
	omd.Replicas = utils.NewInt32(size.Database.Replicas)
	omd.Resources = size.Database.Resources
}

// MySQLLabels returns the labels for selecting the resources belonging to the
// given mysql cluster.
func MySQLLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/component":  "database",
		"app.kubernetes.io/instance":   "db",
		"app.kubernetes.io/managed-by": "mysql.presslabs.org",
		"app.kubernetes.io/name":       "mysql",
	}
}
