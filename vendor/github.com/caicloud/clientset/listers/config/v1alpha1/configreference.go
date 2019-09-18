/*
Copyright 2019 caicloud authors. All rights reserved.
*/

// Code generated by lister-gen. DO NOT EDIT.

package v1alpha1

import (
	v1alpha1 "github.com/caicloud/clientset/pkg/apis/config/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// ConfigReferenceLister helps list ConfigReferences.
type ConfigReferenceLister interface {
	// List lists all ConfigReferences in the indexer.
	List(selector labels.Selector) (ret []*v1alpha1.ConfigReference, err error)
	// ConfigReferences returns an object that can list and get ConfigReferences.
	ConfigReferences(namespace string) ConfigReferenceNamespaceLister
	ConfigReferenceListerExpansion
}

// configReferenceLister implements the ConfigReferenceLister interface.
type configReferenceLister struct {
	indexer cache.Indexer
}

// NewConfigReferenceLister returns a new ConfigReferenceLister.
func NewConfigReferenceLister(indexer cache.Indexer) ConfigReferenceLister {
	return &configReferenceLister{indexer: indexer}
}

// List lists all ConfigReferences in the indexer.
func (s *configReferenceLister) List(selector labels.Selector) (ret []*v1alpha1.ConfigReference, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.ConfigReference))
	})
	return ret, err
}

// ConfigReferences returns an object that can list and get ConfigReferences.
func (s *configReferenceLister) ConfigReferences(namespace string) ConfigReferenceNamespaceLister {
	return configReferenceNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// ConfigReferenceNamespaceLister helps list and get ConfigReferences.
type ConfigReferenceNamespaceLister interface {
	// List lists all ConfigReferences in the indexer for a given namespace.
	List(selector labels.Selector) (ret []*v1alpha1.ConfigReference, err error)
	// Get retrieves the ConfigReference from the indexer for a given namespace and name.
	Get(name string) (*v1alpha1.ConfigReference, error)
	ConfigReferenceNamespaceListerExpansion
}

// configReferenceNamespaceLister implements the ConfigReferenceNamespaceLister
// interface.
type configReferenceNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all ConfigReferences in the indexer for a given namespace.
func (s configReferenceNamespaceLister) List(selector labels.Selector) (ret []*v1alpha1.ConfigReference, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.ConfigReference))
	})
	return ret, err
}

// Get retrieves the ConfigReference from the indexer for a given namespace and name.
func (s configReferenceNamespaceLister) Get(name string) (*v1alpha1.ConfigReference, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha1.Resource("configreference"), name)
	}
	return obj.(*v1alpha1.ConfigReference), nil
}
