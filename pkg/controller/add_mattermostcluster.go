package controller

import (
	"github.com/mattermost/mattermost-operator/pkg/controller/mattermostcluster"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, mattermostcluster.Add)
}
