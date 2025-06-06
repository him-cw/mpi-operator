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

package v2beta1

import (
	v2beta1 "github.com/coreweave/group-operator/pkg/apis/kubeflow/v2beta1"
)

// RunPolicyApplyConfiguration represents a declarative configuration of the RunPolicy type for use
// with apply.
type RunPolicyApplyConfiguration struct {
	CleanPodPolicy          *v2beta1.CleanPodPolicy             `json:"cleanPodPolicy,omitempty"`
	TTLSecondsAfterFinished *int32                              `json:"ttlSecondsAfterFinished,omitempty"`
	ActiveDeadlineSeconds   *int64                              `json:"activeDeadlineSeconds,omitempty"`
	BackoffLimit            *int32                              `json:"backoffLimit,omitempty"`
	SchedulingPolicy        *SchedulingPolicyApplyConfiguration `json:"schedulingPolicy,omitempty"`
	Suspend                 *bool                               `json:"suspend,omitempty"`
	ManagedBy               *string                             `json:"managedBy,omitempty"`
}

// RunPolicyApplyConfiguration constructs a declarative configuration of the RunPolicy type for use with
// apply.
func RunPolicy() *RunPolicyApplyConfiguration {
	return &RunPolicyApplyConfiguration{}
}

// WithCleanPodPolicy sets the CleanPodPolicy field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the CleanPodPolicy field is set to the value of the last call.
func (b *RunPolicyApplyConfiguration) WithCleanPodPolicy(value v2beta1.CleanPodPolicy) *RunPolicyApplyConfiguration {
	b.CleanPodPolicy = &value
	return b
}

// WithTTLSecondsAfterFinished sets the TTLSecondsAfterFinished field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the TTLSecondsAfterFinished field is set to the value of the last call.
func (b *RunPolicyApplyConfiguration) WithTTLSecondsAfterFinished(value int32) *RunPolicyApplyConfiguration {
	b.TTLSecondsAfterFinished = &value
	return b
}

// WithActiveDeadlineSeconds sets the ActiveDeadlineSeconds field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the ActiveDeadlineSeconds field is set to the value of the last call.
func (b *RunPolicyApplyConfiguration) WithActiveDeadlineSeconds(value int64) *RunPolicyApplyConfiguration {
	b.ActiveDeadlineSeconds = &value
	return b
}

// WithBackoffLimit sets the BackoffLimit field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the BackoffLimit field is set to the value of the last call.
func (b *RunPolicyApplyConfiguration) WithBackoffLimit(value int32) *RunPolicyApplyConfiguration {
	b.BackoffLimit = &value
	return b
}

// WithSchedulingPolicy sets the SchedulingPolicy field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the SchedulingPolicy field is set to the value of the last call.
func (b *RunPolicyApplyConfiguration) WithSchedulingPolicy(value *SchedulingPolicyApplyConfiguration) *RunPolicyApplyConfiguration {
	b.SchedulingPolicy = value
	return b
}

// WithSuspend sets the Suspend field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Suspend field is set to the value of the last call.
func (b *RunPolicyApplyConfiguration) WithSuspend(value bool) *RunPolicyApplyConfiguration {
	b.Suspend = &value
	return b
}

// WithManagedBy sets the ManagedBy field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the ManagedBy field is set to the value of the last call.
func (b *RunPolicyApplyConfiguration) WithManagedBy(value string) *RunPolicyApplyConfiguration {
	b.ManagedBy = &value
	return b
}
