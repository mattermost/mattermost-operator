package apis

import (
	v1ElasticSearch "github.com/upmc-enterprises/elasticsearch-operator/pkg/apis/elasticsearchoperator/v1"
)

func init() {
	// Register the types with the Scheme so the components can map objects to GroupVersionKinds and back
	AddToSchemes = append(AddToSchemes, v1ElasticSearch.SchemeBuilder.AddToScheme)
}
