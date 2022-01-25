/*
Copyright 2021 The Clusternet Authors.

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

package utils

import (
	"context"
	"fmt"
	appsapi "github.com/lmxia/gaia/pkg/apis/apps/v1alpha1"
	gaiaClientSet "github.com/lmxia/gaia/pkg/generated/clientset/versioned"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"

	known "github.com/lmxia/gaia/pkg/common"
)

func DeleteResourceWithRetry(ctx context.Context, dynamicClient dynamic.Interface, restMapper meta.RESTMapper, resource *unstructured.Unstructured) error {
	deletePropagationBackground := metav1.DeletePropagationBackground

	var lastError error
	err := wait.ExponentialBackoffWithContext(ctx, retry.DefaultBackoff, func() (bool, error) {
		restMapping, err := restMapper.RESTMapping(resource.GroupVersionKind().GroupKind(), resource.GroupVersionKind().Version)
		if err != nil {
			lastError = fmt.Errorf("please check whether the advertised apiserver of current child cluster is accessible. %v", err)
			return false, nil
		}

		lastError = dynamicClient.Resource(restMapping.Resource).Namespace(resource.GetNamespace()).
			Delete(context.TODO(), resource.GetName(), metav1.DeleteOptions{PropagationPolicy: &deletePropagationBackground})
		if lastError == nil || (lastError != nil && apierrors.IsNotFound(lastError)) {
			return true, nil
		}
		return false, nil
	})
	if err == nil {
		return nil
	}
	return lastError
}

// copied from k8s.io/apimachinery/pkg/apis/meta/v1/unstructured
func getNestedString(obj map[string]interface{}, fields ...string) string {
	val, found, err := unstructured.NestedString(obj, fields...)
	if !found || err != nil {
		return ""
	}
	return val
}

// copied from k8s.io/apimachinery/pkg/apis/meta/v1/unstructured
// and modified
func setNestedField(u *unstructured.Unstructured, value interface{}, fields ...string) {
	if u.Object == nil {
		u.Object = make(map[string]interface{})
	}
	err := unstructured.SetNestedField(u.Object, value, fields...)
	if err != nil {
		klog.Warningf("failed to set nested field: %v", err)
	}
}

// getStatusCause returns the named cause from the provided error if it exists and
// the error is of the type APIStatus. Otherwise it returns false.
func getStatusCause(err error) ([]metav1.StatusCause, bool) {
	apierr, ok := err.(apierrors.APIStatus)
	if !ok || apierr == nil || apierr.Status().Details == nil {
		return nil, false
	}
	return apierr.Status().Details.Causes, true
}

func GetDeployerCredentials(ctx context.Context, childKubeClientSet kubernetes.Interface, sa string) *corev1.Secret {
	var secret *corev1.Secret
	localCtx, cancel := context.WithCancel(ctx)

	klog.V(4).Infof("get ServiceAccount %s/%s", known.GaiaSystemNamespace, sa)
	wait.JitterUntilWithContext(localCtx, func(ctx context.Context) {
		sa, err := childKubeClientSet.CoreV1().ServiceAccounts(known.GaiaSystemNamespace).Get(ctx, sa, metav1.GetOptions{})
		if err != nil {
			klog.ErrorDepth(5, fmt.Errorf("failed to get ServiceAccount %s/%s: %v", known.GaiaSystemNamespace, sa, err))
			return
		}

		if len(sa.Secrets) == 0 {
			klog.ErrorDepth(5, fmt.Errorf("no secrets found in ServiceAccount %s/%s", known.GaiaSystemNamespace, sa))
			return
		}

		secret, err = childKubeClientSet.CoreV1().Secrets(known.GaiaSystemNamespace).Get(ctx, sa.Secrets[0].Name, metav1.GetOptions{})
		if err != nil {
			klog.ErrorDepth(5, fmt.Errorf("failed to get Secret %s/%s: %v", known.GaiaSystemNamespace, sa.Secrets[0].Name, err))
			return
		}

		cancel()
	}, known.DefaultRetryPeriod, 0.4, true)

	klog.V(4).Info("successfully get credentials populated for deployer")
	return secret
}

func OffloadDescription(ctx context.Context, gaiaClient *gaiaClientSet.Clientset, dynamicClient dynamic.Interface,
	discoveryRESTMapper meta.RESTMapper, desc *appsapi.Description) error {
	var err error
	var allErrs []error
	wg := sync.WaitGroup{}
	objectsToBeDeleted := desc.Spec.Raw
	errCh := make(chan error, len(objectsToBeDeleted))
	for _, object := range objectsToBeDeleted {
		resource := &unstructured.Unstructured{}
		err := resource.UnmarshalJSON(object)
		if err != nil {
			allErrs = append(allErrs, err)
			msg := fmt.Sprintf("failed to unmarshal resource: %v", err)
			klog.ErrorDepth(5, msg)
		} else {
			wg.Add(1)
			go func(resource *unstructured.Unstructured) {
				defer wg.Done()
				klog.V(5).Infof("deleting %s %s defined in Description %s", resource.GetKind(),
					klog.KObj(resource), klog.KObj(desc))
				err := DeleteResourceWithRetry(ctx, dynamicClient, discoveryRESTMapper, resource)
				if err != nil {
					errCh <- err
				}
			}(resource)
		}
	}
	wg.Wait()

	// collect errors
	close(errCh)
	for err := range errCh {
		allErrs = append(allErrs, err)
	}

	err = utilerrors.NewAggregate(allErrs)
	if err != nil {
		msg := fmt.Sprintf("failed to deleting Description %s: %v", klog.KObj(desc), err)
		klog.ErrorDepth(5, msg)
	} else {
		klog.V(5).Infof("Description %s is deleted successfully", klog.KObj(desc))
		descCopy := desc.DeepCopy()
		descCopy.Finalizers = RemoveString(descCopy.Finalizers, known.AppFinalizer)
		_, err = gaiaClient.AppsV1alpha1().Descriptions(descCopy.Namespace).Update(context.TODO(), descCopy, metav1.UpdateOptions{})
		if err != nil {
			klog.WarningDepth(4,
				fmt.Sprintf("failed to remove finalizer %s from Description %s: %v", known.AppFinalizer, klog.KObj(descCopy), err))

		}
	}
	return err
}


func ApplyDescription(ctx context.Context, gaiaclient *gaiaClientSet.Clientset, dynamicClient dynamic.Interface,
	discoveryRESTMapper meta.RESTMapper, desc *appsapi.Description) error {
	var allErrs []error
	wg := sync.WaitGroup{}
	objectsToBeDeployed := desc.Spec.Raw
	errCh := make(chan error, len(objectsToBeDeployed))
	for _, object := range objectsToBeDeployed {
		resource := &unstructured.Unstructured{}
		err := resource.UnmarshalJSON(object)
		if err != nil {
			allErrs = append(allErrs, err)
			msg := fmt.Sprintf("failed to unmarshal resource: %v", err)
			klog.ErrorDepth(5, msg)
			continue
		}
		wg.Add(1)
		go func(resource *unstructured.Unstructured) {
			defer wg.Done()
			retryErr := ApplyResourceWithRetry(ctx, dynamicClient, discoveryRESTMapper, resource)
			if retryErr != nil {
				errCh <- retryErr
				return
			}
		}(resource)

	}
	wg.Wait()

	// collect errors
	close(errCh)
	for err := range errCh {
		allErrs = append(allErrs, err)
	}

	var statusPhase appsapi.DescriptionPhase
	var reason string
	if len(allErrs) > 0 {
		statusPhase = appsapi.DescriptionPhaseFailure
		reason = utilerrors.NewAggregate(allErrs).Error()

		msg := fmt.Sprintf("failed to deploying Description %s: %s", klog.KObj(desc), reason)
		klog.ErrorDepth(5, msg)
	} else {
		statusPhase = appsapi.DescriptionPhaseSuccess
		reason = ""

		msg := fmt.Sprintf("Description %s is deployed successfully", klog.KObj(desc))
		klog.V(5).Info(msg)
	}

	// update status
	desc.Status.Phase = statusPhase
	desc.Status.Reason = reason
	_, err := gaiaclient.AppsV1alpha1().Descriptions(desc.Namespace).UpdateStatus(context.TODO(), desc, metav1.UpdateOptions{})

	if len(allErrs) > 0 {
		return utilerrors.NewAggregate(allErrs)
	}
	return err
}

func ApplyResourceWithRetry(ctx context.Context, dynamicClient dynamic.Interface, restMapper meta.RESTMapper, resource *unstructured.Unstructured) error {
	// set UID as empty
	resource.SetUID("")

	var lastError error
	err := wait.ExponentialBackoffWithContext(ctx, retry.DefaultBackoff, func() (bool, error) {
		restMapping, err := restMapper.RESTMapping(resource.GroupVersionKind().GroupKind(), resource.GroupVersionKind().Version)
		if err != nil {
			lastError = fmt.Errorf("please check whether the advertised apiserver of current child cluster is accessible. %v", err)
			return false, nil
		}

		_, lastError = dynamicClient.Resource(restMapping.Resource).Namespace(resource.GetNamespace()).
			Create(context.TODO(), resource, metav1.CreateOptions{})
		if lastError == nil {
			return true, nil
		}
		if !apierrors.IsAlreadyExists(lastError) {
			return false, nil
		}

		curObj, err := dynamicClient.Resource(restMapping.Resource).Namespace(resource.GetNamespace()).
			Get(context.TODO(), resource.GetName(), metav1.GetOptions{})
		if err != nil {
			lastError = err
			return false, nil
		} else {
			lastError = nil
		}

		// try to update resource
		_, lastError = dynamicClient.Resource(restMapping.Resource).Namespace(resource.GetNamespace()).
			Update(context.TODO(), resource, metav1.UpdateOptions{})
		if lastError == nil {
			return true, nil
		}
		statusCauses, ok := getStatusCause(lastError)
		if !ok {
			lastError = fmt.Errorf("failed to get StatusCause for %s %s", resource.GetKind(), klog.KObj(resource))
			return false, nil
		}
		resourceCopy := resource.DeepCopy()
		for _, cause := range statusCauses {
			if cause.Type != metav1.CauseTypeFieldValueInvalid {
				continue
			}
			// apply immutable value
			fields := strings.Split(cause.Field, ".")
			setNestedField(resourceCopy, getNestedString(curObj.Object, fields...), fields...)
		}
		// update with immutable values applied
		_, lastError = dynamicClient.Resource(restMapping.Resource).Namespace(resourceCopy.GetNamespace()).
			Update(context.TODO(), resourceCopy, metav1.UpdateOptions{})
		if lastError == nil {
			return true, nil
		}
		return false, nil
	})

	if err == nil {
		return nil
	}
	return lastError
}
