// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package controller

import (
	"github.com/mattermost/mattermost-operator/pkg/controller/mattermostrestoredb"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, mattermostrestoredb.Add)
}
