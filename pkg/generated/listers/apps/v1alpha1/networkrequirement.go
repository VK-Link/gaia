/*
Copyright The Gaia Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by lister-gen. DO NOT EDIT.

package v1alpha1

import (
	v1alpha1 "github.com/lmxia/gaia/pkg/apis/apps/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// NetworkRequirementLister helps list NetworkRequirements.
// All objects returned here must be treated as read-only.
type NetworkRequirementLister interface {
	// List lists all NetworkRequirements in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.NetworkRequirement, err error)
	// NetworkRequirements returns an object that can list and get NetworkRequirements.
	NetworkRequirements(namespace string) NetworkRequirementNamespaceLister
	NetworkRequirementListerExpansion
}

// networkRequirementLister implements the NetworkRequirementLister interface.
type networkRequirementLister struct {
	indexer cache.Indexer
}

// NewNetworkRequirementLister returns a new NetworkRequirementLister.
func NewNetworkRequirementLister(indexer cache.Indexer) NetworkRequirementLister {
	return &networkRequirementLister{indexer: indexer}
}

// List lists all NetworkRequirements in the indexer.
func (s *networkRequirementLister) List(selector labels.Selector) (ret []*v1alpha1.NetworkRequirement, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.NetworkRequirement))
	})
	return ret, err
}

// NetworkRequirements returns an object that can list and get NetworkRequirements.
func (s *networkRequirementLister) NetworkRequirements(namespace string) NetworkRequirementNamespaceLister {
	return networkRequirementNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// NetworkRequirementNamespaceLister helps list and get NetworkRequirements.
// All objects returned here must be treated as read-only.
type NetworkRequirementNamespaceLister interface {
	// List lists all NetworkRequirements in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.NetworkRequirement, err error)
	// Get retrieves the NetworkRequirement from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1alpha1.NetworkRequirement, error)
	NetworkRequirementNamespaceListerExpansion
}

// networkRequirementNamespaceLister implements the NetworkRequirementNamespaceLister
// interface.
type networkRequirementNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all NetworkRequirements in the indexer for a given namespace.
func (s networkRequirementNamespaceLister) List(selector labels.Selector) (ret []*v1alpha1.NetworkRequirement, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.NetworkRequirement))
	})
	return ret, err
}

// Get retrieves the NetworkRequirement from the indexer for a given namespace and name.
func (s networkRequirementNamespaceLister) Get(name string) (*v1alpha1.NetworkRequirement, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha1.Resource("networkrequirement"), name)
	}
	return obj.(*v1alpha1.NetworkRequirement), nil
}
