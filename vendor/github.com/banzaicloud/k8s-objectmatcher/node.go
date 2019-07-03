// Copyright © 2019 Banzai Cloud
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package objectmatch

import (
	"encoding/json"

	"github.com/goph/emperror"
	v1 "k8s.io/api/core/v1"
	kubernetescorev1 "k8s.io/kubernetes/pkg/apis/core/v1"
)

type nodeMatcher struct {
	objectMatcher ObjectMatcher
}

func NewNodeMatcher(objectMatcher ObjectMatcher) *nodeMatcher {
	return &nodeMatcher{
		objectMatcher: objectMatcher,
	}
}

func (m nodeMatcher) Match(oldOrig, newOrig *v1.Node) (bool, error) {

	old := oldOrig.DeepCopy()
	new := newOrig.DeepCopy()

	kubernetescorev1.SetObjectDefaults_Node(new)

	type Node struct {
		ObjectMeta
		Spec v1.NodeSpec
	}

	oldData, err := json.Marshal(Node{
		ObjectMeta: m.objectMatcher.GetObjectMeta(old.ObjectMeta),
		Spec:       old.Spec,
	})
	if err != nil {
		return false, emperror.WrapWith(err, "could not marshal old object", "name", old.Name)
	}

	newObject := Node{
		ObjectMeta: m.objectMatcher.GetObjectMeta(new.ObjectMeta),
		Spec:       new.Spec,
	}

	newData, err := json.Marshal(newObject)
	if err != nil {
		return false, emperror.WrapWith(err, "could not marshal new object", "name", new.Name)
	}

	matched, err := m.objectMatcher.MatchJSON(oldData, newData, newObject)
	if err != nil {
		return false, emperror.WrapWith(err, "could not match objects", "name", new.Name)
	}

	return matched, nil
}
