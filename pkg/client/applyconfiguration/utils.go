// Copyright 2025 The Kubeflow Authors.
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

// Code generated by applyconfiguration-gen. DO NOT EDIT.

package applyconfiguration

import (
	v2beta1 "github.com/coreweave/group-operator/pkg/apis/kubeflow/v2beta1"
	internal "github.com/coreweave/group-operator/pkg/client/applyconfiguration/internal"
	kubeflowv2beta1 "github.com/coreweave/group-operator/pkg/client/applyconfiguration/kubeflow/v2beta1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	testing "k8s.io/client-go/testing"
)

// ForKind returns an apply configuration type for the given GroupVersionKind, or nil if no
// apply configuration type exists for the given GroupVersionKind.
func ForKind(kind schema.GroupVersionKind) interface{} {
	switch kind {
	// Group=kubeflow.org, Version=v2beta1
	case v2beta1.SchemeGroupVersion.WithKind("JobCondition"):
		return &kubeflowv2beta1.JobConditionApplyConfiguration{}
	case v2beta1.SchemeGroupVersion.WithKind("JobStatus"):
		return &kubeflowv2beta1.JobStatusApplyConfiguration{}
	case v2beta1.SchemeGroupVersion.WithKind("GroupJob"):
		return &kubeflowv2beta1.GroupJobApplyConfiguration{}
	case v2beta1.SchemeGroupVersion.WithKind("GroupJobSpec"):
		return &kubeflowv2beta1.GroupJobSpecApplyConfiguration{}
	case v2beta1.SchemeGroupVersion.WithKind("ReplicaSpec"):
		return &kubeflowv2beta1.ReplicaSpecApplyConfiguration{}
	case v2beta1.SchemeGroupVersion.WithKind("ReplicaStatus"):
		return &kubeflowv2beta1.ReplicaStatusApplyConfiguration{}
	case v2beta1.SchemeGroupVersion.WithKind("RunPolicy"):
		return &kubeflowv2beta1.RunPolicyApplyConfiguration{}
	case v2beta1.SchemeGroupVersion.WithKind("SchedulingPolicy"):
		return &kubeflowv2beta1.SchedulingPolicyApplyConfiguration{}

	}
	return nil
}

func NewTypeConverter(scheme *runtime.Scheme) *testing.TypeConverter {
	return &testing.TypeConverter{Scheme: scheme, TypeResolver: internal.Parser()}
}
