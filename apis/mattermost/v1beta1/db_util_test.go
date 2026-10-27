package v1beta1

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDatabase_IsUnmanaged(t *testing.T) {
	t.Run("nil receiver", func(t *testing.T) {
		var db *Database
		assert.False(t, db.IsUnmanaged())
	})
	t.Run("unmanaged false", func(t *testing.T) {
		db := &Database{}
		assert.False(t, db.IsUnmanaged())
	})
	t.Run("unmanaged true", func(t *testing.T) {
		db := &Database{Unmanaged: true}
		assert.True(t, db.IsUnmanaged())
	})
}

func TestDatabase_SetDefaults_Unmanaged(t *testing.T) {
	db := &Database{Unmanaged: true}
	db.SetDefaults()

	assert.Nil(t, db.OperatorManaged, "SetDefaults must not populate OperatorManaged when Unmanaged")
}

func TestDatabase_SetDefaultReplicasAndResources_Unmanaged(t *testing.T) {
	db := &Database{Unmanaged: true}
	db.SetDefaultReplicasAndResources()

	assert.Nil(t, db.OperatorManaged, "SetDefaultReplicasAndResources must not populate OperatorManaged when Unmanaged")
}
