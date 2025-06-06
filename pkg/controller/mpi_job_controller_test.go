// Copyright 2018 The Kubeflow Authors.
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

package controller

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/uuid"
	kubeinformers "k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/clock"
	clocktesting "k8s.io/utils/clock/testing"
	"k8s.io/utils/ptr"
	schedv1alpha1 "sigs.k8s.io/scheduler-plugins/apis/scheduling/v1alpha1"
	schedclientset "sigs.k8s.io/scheduler-plugins/pkg/generated/clientset/versioned"
	volcanov1beta1 "volcano.sh/apis/pkg/apis/scheduling/v1beta1"
	volcanofake "volcano.sh/apis/pkg/client/clientset/versioned/fake"

	kubeflow "github.com/coreweave/group-operator/pkg/apis/kubeflow/v2beta1"
	"github.com/coreweave/group-operator/pkg/client/clientset/versioned/fake"
	"github.com/coreweave/group-operator/pkg/client/clientset/versioned/scheme"
	informers "github.com/coreweave/group-operator/pkg/client/informers/externalversions"
)

var (
	alwaysReady        = func() bool { return true }
	noResyncPeriodFunc = func() time.Duration { return 0 }

	ignoreConditionTimes = cmpopts.IgnoreFields(kubeflow.JobCondition{}, "LastUpdateTime", "LastTransitionTime")
	ignoreSecretEntries  = cmpopts.IgnoreMapEntries(func(k string, v []uint8) bool { return true })
	ignoreReferences     = cmpopts.IgnoreFields(metav1.ObjectMeta{}, "OwnerReferences")
)

type fixture struct {
	t *testing.T

	client        *fake.Clientset
	kubeClient    *k8sfake.Clientset
	volcanoClient *volcanofake.Clientset
	schedClient   *schedclientset.Clientset

	// Objects to put in the store.
	configMapLister       []*corev1.ConfigMap
	serviceLister         []*corev1.Service
	secretLister          []*corev1.Secret
	volcanoPodGroupLister []*volcanov1beta1.PodGroup
	schedPodGroupLister   []*schedv1alpha1.PodGroup
	jobLister             []*batchv1.Job
	podLister             []*corev1.Pod
	priorityClassLister   []*schedulingv1.PriorityClass
	mpiJobLister          []*kubeflow.GroupJob

	// Actions expected to happen on the client.
	kubeActions []core.Action
	actions     []core.Action

	// Objects from here are pre-loaded into NewSimpleFake.
	kubeObjects []runtime.Object
	objects     []runtime.Object

	gangSchedulingName string
}

func newFixture(t *testing.T, gangSchedulingName string) *fixture {
	f := &fixture{}
	f.t = t
	f.objects = []runtime.Object{}
	f.kubeObjects = []runtime.Object{}
	f.gangSchedulingName = gangSchedulingName
	return f
}

func newGroupJobCommon(name string, startTime, completionTime *metav1.Time) *kubeflow.GroupJob {
	cleanPodPolicyAll := kubeflow.CleanPodPolicyAll
	mpiJob := &kubeflow.GroupJob{
		TypeMeta: metav1.TypeMeta{APIVersion: kubeflow.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metav1.NamespaceDefault,
		},
		Spec: kubeflow.GroupJobSpec{
			RunPolicy: kubeflow.RunPolicy{
				CleanPodPolicy: &cleanPodPolicyAll,
			},
			MPIReplicaSpecs: map[kubeflow.MPIReplicaType]*kubeflow.ReplicaSpec{
				kubeflow.MPIReplicaTypeLauncher: {
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "foo",
									Image: "bar",
								},
							},
						},
					},
				},
			},
		},
		Status: kubeflow.JobStatus{},
	}

	if startTime != nil {
		mpiJob.Status.StartTime = startTime
	}
	if completionTime != nil {
		mpiJob.Status.CompletionTime = completionTime
	}

	return mpiJob
}

func newGroupJob(name string, replicas *int32, startTime, completionTime *metav1.Time) *kubeflow.GroupJob {
	mpiJob := newGroupJobCommon(name, startTime, completionTime)
	if ptr.Deref(replicas, 0) > 0 {
		mpiJob.Spec.MPIReplicaSpecs[kubeflow.MPIReplicaTypeWorker] =
			&kubeflow.ReplicaSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "foo",
								Image: "bar",
							},
						},
					},
				},
				Replicas: replicas,
			}
	}
	return mpiJob
}

func (f *fixture) newController(clock clock.WithTicker) (*GroupJobController, informers.SharedInformerFactory, kubeinformers.SharedInformerFactory) {
	f.client = fake.NewSimpleClientset(f.objects...)
	f.kubeClient = k8sfake.NewSimpleClientset(f.kubeObjects...)
	i := informers.NewSharedInformerFactory(f.client, noResyncPeriodFunc())
	k8sI := kubeinformers.NewSharedInformerFactory(f.kubeClient, noResyncPeriodFunc())
	workqueueRateLimiter := workqueue.DefaultTypedControllerRateLimiter[any]()

	c, err := NewGroupJobControllerWithClock(
		f.kubeClient,
		f.client,
		f.volcanoClient,
		f.schedClient,
		k8sI.Core().V1().ConfigMaps(),
		k8sI.Core().V1().Secrets(),
		k8sI.Core().V1().Services(),
		k8sI.Batch().V1().Jobs(),
		k8sI.Core().V1().Pods(),
		k8sI.Scheduling().V1().PriorityClasses(),
		i.Kubeflow().V2beta1().GroupJobs(),
		clock,
		metav1.NamespaceAll,
		f.gangSchedulingName,
		workqueueRateLimiter,
	)
	if err != nil {
		fmt.Println("Failed to setup the controller")
	}

	c.configMapSynced = alwaysReady
	c.serviceSynced = alwaysReady
	c.secretSynced = alwaysReady
	c.podSynced = alwaysReady
	c.podGroupSynced = alwaysReady
	c.mpiJobSynced = alwaysReady
	c.recorder = &record.FakeRecorder{}

	for _, configMap := range f.configMapLister {
		err = k8sI.Core().V1().ConfigMaps().Informer().GetIndexer().Add(configMap)
		if err != nil {
			fmt.Println("Failed to create config map")
		}
	}

	for _, service := range f.serviceLister {
		err = k8sI.Core().V1().Services().Informer().GetIndexer().Add(service)
		if err != nil {
			fmt.Println("Failed to create service account")
		}
	}

	for _, secret := range f.secretLister {
		err = k8sI.Core().V1().Secrets().Informer().GetIndexer().Add(secret)
		if err != nil {
			fmt.Println("Failed to create role")
		}
	}

	for _, job := range f.jobLister {
		err = k8sI.Batch().V1().Jobs().Informer().GetIndexer().Add(job)
		if err != nil {
			fmt.Println("Failed to create job")
		}
	}

	for _, pod := range f.podLister {
		err = k8sI.Core().V1().Pods().Informer().GetIndexer().Add(pod)
		if err != nil {
			fmt.Println("Failed to create pod")
		}
	}

	if c.PodGroupCtrl != nil {
		for _, podGroup := range f.volcanoPodGroupLister {
			err = c.PodGroupCtrl.PodGroupSharedIndexInformer().GetIndexer().Add(podGroup)
			if err != nil {
				fmt.Println("Failed to create volcano pod group")
			}
		}
		for _, podGroup := range f.schedPodGroupLister {
			err = c.PodGroupCtrl.PodGroupSharedIndexInformer().GetIndexer().Add(podGroup)
			if err != nil {
				fmt.Println("Failed to create scheduler-plugins pod group")
			}
		}
		for _, priorityClass := range f.priorityClassLister {
			err = k8sI.Scheduling().V1().PriorityClasses().Informer().GetIndexer().Add(priorityClass)
			if err != nil {
				fmt.Println("Failed to create priorityClass")
			}
		}
	}

	for _, mpiJob := range f.mpiJobLister {
		err = i.Kubeflow().V2beta1().GroupJobs().Informer().GetIndexer().Add(mpiJob)
		if err != nil {
			fmt.Println("Failed to create mpijob")
		}
	}

	return c, i, k8sI
}

func (f *fixture) run(mpiJobName string) {
	f.runWithClock(mpiJobName, clock.RealClock{})
}

func (f *fixture) runWithClock(mpiJobName string, clock clock.WithTicker) {
	f.runController(mpiJobName, true, false, clock)
}

func (f *fixture) runExpectError(mpiJobName string) {
	f.runController(mpiJobName, true, true, clock.RealClock{})
}

func (f *fixture) runController(mpiJobName string, startInformers, expectError bool, clock clock.WithTicker) {
	c, i, k8sI := f.newController(clock)
	if startInformers {
		stopCh := make(chan struct{})
		defer close(stopCh)
		i.Start(stopCh)
		k8sI.Start(stopCh)
		if c.PodGroupCtrl != nil {
			c.PodGroupCtrl.StartInformerFactory(stopCh)
		}
	}

	err := c.syncHandler(mpiJobName)
	if !expectError && err != nil {
		f.t.Errorf("error syncing mpi job: %v", err)
	} else if expectError && err == nil {
		f.t.Error("expected error syncing mpi job, got nil")
	}

	actions := filterInformerActions(f.client.Actions())
	for i, action := range actions {
		if len(f.actions) < i+1 {
			f.t.Errorf("%d unexpected actions: %+v", len(actions)-len(f.actions), actions[i:])
			break
		}

		expectedAction := f.actions[i]
		checkAction(expectedAction, action, f.t)
	}

	if len(f.actions) > len(actions) {
		f.t.Errorf("%d additional expected actions:%+v", len(f.actions)-len(actions), f.actions[len(actions):])
	}

	k8sActions := filterInformerActions(f.kubeClient.Actions())
	for i, action := range k8sActions {
		if len(f.kubeActions) < i+1 {
			f.t.Errorf("%d unexpected actions: %+v", len(k8sActions)-len(f.kubeActions), k8sActions[i:])
			break
		}

		expectedAction := f.kubeActions[i]
		checkAction(expectedAction, action, f.t)
	}

	if len(f.kubeActions) > len(k8sActions) {
		f.t.Errorf("%d additional expected actions:%+v", len(f.kubeActions)-len(k8sActions), f.kubeActions[len(k8sActions):])
	}
}

// checkAction verifies that expected and actual actions are equal and both have
// same attached resources
func checkAction(expected, actual core.Action, t *testing.T) {
	if !(expected.Matches(actual.GetVerb(), actual.GetResource().Resource) && actual.GetSubresource() == expected.GetSubresource()) {
		t.Errorf("Expected\n\t%#v\ngot\n\t%#v", expected, actual)
		return
	}

	if reflect.TypeOf(actual) != reflect.TypeOf(expected) {
		t.Errorf("Action has wrong type. Expected: %t. Got: %t", expected, actual)
		return
	}

	//nolint
	switch a := actual.(type) {
	case core.UpdateAction:
		e, _ := expected.(core.UpdateAction)
		expObject := e.GetObject()
		object := a.GetObject()

		if diff := cmp.Diff(expObject, object, ignoreSecretEntries, ignoreConditionTimes); diff != "" {
			t.Errorf("Action %s %s has wrong object (-want +got):\n %s", a.GetVerb(), a.GetResource().Resource, diff)
		}
	case core.CreateAction:
		e, _ := expected.(core.CreateAction)
		expObject := e.GetObject()
		object := a.GetObject()

		if diff := cmp.Diff(expObject, object, ignoreSecretEntries); diff != "" {
			t.Errorf("Action %s %s has wrong object (-want +got):\n %s", a.GetVerb(), a.GetResource().Resource, diff)
		}
	case core.PatchAction:
		e, _ := expected.(core.PatchAction)
		expPatch := e.GetPatch()
		patch := a.GetPatch()

		if diff := cmp.Diff(expPatch, patch); diff != "" {
			t.Errorf("Action %s %s has wrong patch (-want +got):\n %s", a.GetVerb(), a.GetResource().Resource, diff)
		}
	}
}

// filterInformerActions filters list and watch actions for testing resources.
// Since list and watch don't change resource state we can filter it to lower
// nose level in our tests.
func filterInformerActions(actions []core.Action) []core.Action {
	var ret []core.Action
	for _, action := range actions {
		if len(action.GetNamespace()) == 0 &&
			(action.Matches("list", "configmaps") ||
				action.Matches("watch", "configmaps") ||
				action.Matches("list", "services") ||
				action.Matches("watch", "services") ||
				action.Matches("list", "secrets") ||
				action.Matches("watch", "secrets") ||
				action.Matches("list", "jobs") ||
				action.Matches("watch", "jobs") ||
				action.Matches("list", "pods") ||
				action.Matches("watch", "pods") ||
				action.Matches("list", "podgroups") ||
				action.Matches("watch", "podgroups") ||
				action.Matches("list", "priorityclasses") ||
				action.Matches("watch", "priorityclasses") ||
				action.Matches("list", "groupjobs") ||
				action.Matches("watch", "groupjobs")) {
			continue
		}
		ret = append(ret, action)
	}

	return ret
}

func (f *fixture) expectCreateJobAction(d *batchv1.Job) {
	f.kubeActions = append(f.kubeActions, core.NewCreateAction(schema.GroupVersionResource{Resource: "jobs", Group: "batch"}, d.Namespace, d))
}

func (f *fixture) expectUpdateJobAction(job *batchv1.Job) {
	action := core.NewUpdateAction(schema.GroupVersionResource{Resource: "jobs", Group: "batch", Version: "v1"}, job.Namespace, job)
	f.kubeActions = append(f.kubeActions, action)
}

func (f *fixture) expectCreatePodAction(d *corev1.Pod) {
	f.kubeActions = append(f.kubeActions, core.NewCreateAction(schema.GroupVersionResource{Resource: "pods"}, d.Namespace, d))
}

func (f *fixture) expectCreateServiceAction(d *corev1.Service) {
	f.kubeActions = append(f.kubeActions, core.NewCreateAction(schema.GroupVersionResource{Resource: "services"}, d.Namespace, d))
}

func (f *fixture) expectCreateConfigMapAction(d *corev1.ConfigMap) {
	f.kubeActions = append(f.kubeActions, core.NewCreateAction(schema.GroupVersionResource{Resource: "configmaps"}, d.Namespace, d))
}

func (f *fixture) expectCreateSecretAction(d *corev1.Secret) {
	f.kubeActions = append(f.kubeActions, core.NewCreateAction(schema.GroupVersionResource{Resource: "secrets"}, d.Namespace, d))
}

func (f *fixture) expectNoKubeActions() bool {
	k8sActions := filterInformerActions(f.kubeClient.Actions())
	return len(k8sActions) == 0
}

func (f *fixture) expectUpdateGroupJobStatusAction(mpiJob *kubeflow.GroupJob) {
	action := core.NewUpdateAction(schema.GroupVersionResource{Resource: "groupjobs"}, mpiJob.Namespace, mpiJob)
	action.Subresource = "status"
	f.actions = append(f.actions, action)
}

func (f *fixture) setUpGroupJob(mpiJob *kubeflow.GroupJob) {
	f.mpiJobLister = append(f.mpiJobLister, mpiJob)
	f.objects = append(f.objects, mpiJob)
}

func (f *fixture) setUpLauncher(launcher *batchv1.Job) {
	f.jobLister = append(f.jobLister, launcher)
	f.kubeObjects = append(f.kubeObjects, launcher)
}

func (f *fixture) setUpPod(worker *corev1.Pod) {
	f.podLister = append(f.podLister, worker)
	f.kubeObjects = append(f.kubeObjects, worker)
}

func (f *fixture) setUpConfigMap(configMap *corev1.ConfigMap) {
	f.configMapLister = append(f.configMapLister, configMap)
	f.kubeObjects = append(f.kubeObjects, configMap)
}

func (f *fixture) setUpService(service *corev1.Service) {
	f.serviceLister = append(f.serviceLister, service)
	f.kubeObjects = append(f.kubeObjects, service)
}

func (f *fixture) setUpSecret(secret *corev1.Secret) {
	f.secretLister = append(f.secretLister, secret)
	f.kubeObjects = append(f.kubeObjects, secret)
}

func (f *fixture) setUpPriorityClass(priorityClass *schedulingv1.PriorityClass) {
	f.priorityClassLister = append(f.priorityClassLister, priorityClass)
	f.kubeObjects = append(f.kubeObjects, priorityClass)
}

func setUpGroupJobTimestamp(mpiJob *kubeflow.GroupJob, startTime, completionTime *metav1.Time) {
	if startTime != nil {
		mpiJob.Status.StartTime = startTime
	}

	if completionTime != nil {
		mpiJob.Status.CompletionTime = completionTime
	}
}

func getKey(mpiJob *kubeflow.GroupJob, t *testing.T) string {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(mpiJob)
	if err != nil {
		t.Errorf("Unexpected error getting key for mpi job %v: %v", mpiJob.Name, err)
		return ""
	}
	return key
}

func TestDoNothingWithInvalidKey(t *testing.T) {
	f := newFixture(t, "")
	f.run("foo/bar/baz")
}

func TestDoNothingWithNonexistentGroupJob(t *testing.T) {
	f := newFixture(t, "")
	startTime := metav1.Now()
	completionTime := metav1.Now()
	mpiJob := newGroupJob("test", ptr.To[int32](64), &startTime, &completionTime)
	f.run(getKey(mpiJob, t))
}

func TestDoNothingWithInvalidGroupJob(t *testing.T) {
	f := newFixture(t, "")
	// An empty GroupJob doesn't pass validation.
	mpiJob := &kubeflow.GroupJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "bar",
		},
	}
	f.setUpGroupJob(mpiJob)
	f.run(getKey(mpiJob, t))
}

func TestDoNothingWithGroupJobManagedExternally(t *testing.T) {
	f := newFixture(t, "")
	var replicas int32 = 1
	startTime := metav1.Now()
	completionTime := metav1.Now()
	mpiJob := newGroupJob("test", &replicas, &startTime, &completionTime)
	mpiJob.Spec.MPIImplementation = kubeflow.MPIImplementationOpenMPI
	mpiJob.Spec.RunPolicy.ManagedBy = ptr.To(kubeflow.MultiKueueController)
	f.setUpGroupJob(mpiJob)
	f.run(getKey(mpiJob, t))
	if !f.expectNoKubeActions() {
		t.Fatalf("Expected no kubeActions (secrets, pods, services etc.)")
	}
}

func TestAllResourcesCreated(t *testing.T) {
	impls := []kubeflow.MPIImplementation{kubeflow.MPIImplementationOpenMPI, kubeflow.MPIImplementationIntel, kubeflow.MPIImplementationMPICH}
	for _, implementation := range impls {
		t.Run(string(implementation), func(t *testing.T) {
			f := newFixture(t, "")
			now := metav1.Now()
			mpiJob := newGroupJob("foo", ptr.To[int32](5), &now, nil)
			mpiJob.Spec.MPIImplementation = implementation
			f.setUpGroupJob(mpiJob)

			fmjc := f.newFakeGroupJobController()
			mpiJobCopy := mpiJob.DeepCopy()
			scheme.Scheme.Default(mpiJobCopy)
			f.expectCreateServiceAction(newJobService(mpiJobCopy))
			cfgMap := newConfigMap(mpiJobCopy, 5)
			updateDiscoverHostsInConfigMap(cfgMap, mpiJob, nil)
			f.expectCreateConfigMapAction(cfgMap)
			secret, err := newSSHAuthSecret(mpiJobCopy)
			if err != nil {
				t.Fatalf("Failed creating secret")
			}
			f.expectCreateSecretAction(secret)
			for i := 0; i < 5; i++ {
				f.expectCreatePodAction(fmjc.newWorker(mpiJobCopy, i))
			}
			f.expectCreateJobAction(fmjc.newLauncherJob(mpiJobCopy))

			mpiJobCopy.Status.Conditions = []kubeflow.JobCondition{newCondition(kubeflow.JobCreated, corev1.ConditionTrue, mpiJobCreatedReason, "GroupJob default/foo is created.")}
			mpiJobCopy.Status.ReplicaStatuses = map[kubeflow.MPIReplicaType]*kubeflow.ReplicaStatus{
				kubeflow.MPIReplicaTypeLauncher: {},
				kubeflow.MPIReplicaTypeWorker:   {},
			}
			f.expectUpdateGroupJobStatusAction(mpiJobCopy)

			f.run(getKey(mpiJob, t))
		})
	}
}

func TestLauncherNotControlledByUs(t *testing.T) {
	f := newFixture(t, "")
	startTime := metav1.Now()
	completionTime := metav1.Now()

	mpiJob := newGroupJob("test", ptr.To[int32](64), &startTime, &completionTime)
	f.setUpGroupJob(mpiJob)

	fmjc := f.newFakeGroupJobController()
	mpiJobCopy := mpiJob.DeepCopy()
	scheme.Scheme.Default(mpiJobCopy)
	launcher := fmjc.newLauncherJob(mpiJobCopy)
	launcher.OwnerReferences = nil
	f.setUpLauncher(launcher)

	f.runExpectError(getKey(mpiJob, t))
}

func TestLauncherSucceeded(t *testing.T) {
	f := newFixture(t, "")

	startTime := metav1.Now()
	completionTime := metav1.Now()

	mpiJob := newGroupJob("test", ptr.To[int32](64), &startTime, &completionTime)
	f.setUpGroupJob(mpiJob)

	fmjc := f.newFakeGroupJobController()
	mpiJobCopy := mpiJob.DeepCopy()
	scheme.Scheme.Default(mpiJobCopy)
	launcher := fmjc.newLauncherJob(mpiJobCopy)
	launcher.Status.Conditions = append(launcher.Status.Conditions, batchv1.JobCondition{
		Type:   batchv1.JobComplete,
		Status: corev1.ConditionTrue,
	})
	f.setUpLauncher(launcher)

	mpiJobCopy.Status.ReplicaStatuses = map[kubeflow.MPIReplicaType]*kubeflow.ReplicaStatus{
		kubeflow.MPIReplicaTypeLauncher: {
			Active:    0,
			Succeeded: 1,
			Failed:    0,
		},
		kubeflow.MPIReplicaTypeWorker: {},
	}

	setUpGroupJobTimestamp(mpiJobCopy, &startTime, &completionTime)

	msg := fmt.Sprintf("GroupJob %s/%s is created.", mpiJob.Namespace, mpiJob.Name)
	updateGroupJobConditions(mpiJobCopy, kubeflow.JobCreated, corev1.ConditionTrue, mpiJobCreatedReason, msg)
	msg = fmt.Sprintf("GroupJob %s/%s successfully completed.", mpiJob.Namespace, mpiJob.Name)
	updateGroupJobConditions(mpiJobCopy, kubeflow.JobSucceeded, corev1.ConditionTrue, mpiJobSucceededReason, msg)
	f.expectUpdateGroupJobStatusAction(mpiJobCopy)

	f.run(getKey(mpiJob, t))
}

func TestLauncherFailed(t *testing.T) {
	f := newFixture(t, "")
	startTime := metav1.Now()
	completionTime := metav1.Now()

	mpiJob := newGroupJob("test", ptr.To[int32](64), &startTime, &completionTime)
	f.setUpGroupJob(mpiJob)

	fmjc := f.newFakeGroupJobController()
	mpiJobCopy := mpiJob.DeepCopy()
	scheme.Scheme.Default(mpiJobCopy)
	launcher := fmjc.newLauncherJob(mpiJobCopy)
	launcher.Status.Conditions = append(launcher.Status.Conditions, batchv1.JobCondition{
		Type:    batchv1.JobFailed,
		Status:  corev1.ConditionTrue,
		Reason:  batchv1.JobReasonBackoffLimitExceeded,
		Message: "Job has reached the specified backoff limit",
	})
	launcher.Status.Failed = 2
	f.setUpLauncher(launcher)

	now := time.Now()
	launcherPod1 := mockJobPod(launcher)
	launcherPod1.Status.Phase = corev1.PodFailed
	launcherPod1.Status.Reason = "FailedReason1"
	launcherPod1.Status.Message = "first message"
	launcherPod1.CreationTimestamp = metav1.NewTime(now)
	f.setUpPod(launcherPod1)
	launcherPod2 := mockJobPod(launcher)
	launcherPod2.Status.Phase = corev1.PodFailed
	launcherPod2.Status.Reason = "FailedReason2"
	launcherPod2.Status.Message = "second message"
	launcherPod2.CreationTimestamp = metav1.NewTime(now.Add(time.Second))
	f.setUpPod(launcherPod2)

	mpiJobCopy.Status.ReplicaStatuses = map[kubeflow.MPIReplicaType]*kubeflow.ReplicaStatus{
		kubeflow.MPIReplicaTypeLauncher: {
			Active:    0,
			Succeeded: 0,
			Failed:    2,
		},
		kubeflow.MPIReplicaTypeWorker: {},
	}
	setUpGroupJobTimestamp(mpiJobCopy, &startTime, &completionTime)

	msg := fmt.Sprintf("GroupJob %s/%s is created.", mpiJob.Namespace, mpiJob.Name)
	updateGroupJobConditions(mpiJobCopy, kubeflow.JobCreated, corev1.ConditionTrue, mpiJobCreatedReason, msg)
	msg = "Job has reached the specified backoff limit: second message"
	updateGroupJobConditions(mpiJobCopy, kubeflow.JobFailed, corev1.ConditionTrue, batchv1.JobReasonBackoffLimitExceeded+"/FailedReason2", msg)

	f.expectUpdateGroupJobStatusAction(mpiJobCopy)

	f.run(getKey(mpiJob, t))
}

func TestConfigMapNotControlledByUs(t *testing.T) {
	f := newFixture(t, "")
	startTime := metav1.Now()
	completionTime := metav1.Now()

	var replicas int32 = 64
	mpiJob := newGroupJob("test", &replicas, &startTime, &completionTime)
	f.setUpGroupJob(mpiJob)
	f.setUpService(newJobService(mpiJob))

	configMap := newConfigMap(mpiJob, replicas)
	updateDiscoverHostsInConfigMap(configMap, mpiJob, nil)
	configMap.OwnerReferences = nil
	f.setUpConfigMap(configMap)

	f.runExpectError(getKey(mpiJob, t))
}

func TestWorkerServiceNotControlledByUs(t *testing.T) {
	f := newFixture(t, "")
	startTime := metav1.Now()
	completionTime := metav1.Now()

	var replicas int32 = 2
	mpiJob := newGroupJob("test", &replicas, &startTime, &completionTime)
	f.setUpGroupJob(mpiJob)

	mpiJobCopy := mpiJob.DeepCopy()
	scheme.Scheme.Default(mpiJobCopy)
	service := newJobService(mpiJobCopy)
	service.OwnerReferences = nil
	f.setUpService(service)

	f.runExpectError(getKey(mpiJob, t))
}

func TestLauncherServiceNotControlledByUs(t *testing.T) {
	f := newFixture(t, "")
	startTime := metav1.Now()
	completionTime := metav1.Now()

	var replicas int32 = 2
	mpiJob := newGroupJob("test", &replicas, &startTime, &completionTime)
	mpiJob.Spec.MPIImplementation = kubeflow.MPIImplementationIntel
	f.setUpGroupJob(mpiJob)

	mpiJobCopy := mpiJob.DeepCopy()
	scheme.Scheme.Default(mpiJobCopy)
	service := newJobService(mpiJobCopy)
	service.OwnerReferences = nil
	f.setUpService(service)
	configMap := newConfigMap(mpiJobCopy, replicas)
	secret, err := newSSHAuthSecret(mpiJobCopy)
	if err != nil {
		t.Fatalf("Creating SSH auth Secret: %v", err)
	}
	f.setUpSecret(secret)
	updateDiscoverHostsInConfigMap(configMap, mpiJobCopy, nil)
	f.setUpConfigMap(configMap)
	fmjc := f.newFakeGroupJobController()
	for i := 0; i < int(replicas); i++ {
		worker := fmjc.newWorker(mpiJobCopy, i)
		f.setUpPod(worker)
	}

	f.runExpectError(getKey(mpiJob, t))
}

func TestSecretNotControlledByUs(t *testing.T) {
	f := newFixture(t, "")
	startTime := metav1.Now()
	completionTime := metav1.Now()

	var replicas int32 = 64
	mpiJob := newGroupJob("test", &replicas, &startTime, &completionTime)
	f.setUpGroupJob(mpiJob)

	mpiJobCopy := mpiJob.DeepCopy()
	scheme.Scheme.Default(mpiJobCopy)
	configMap := newConfigMap(mpiJobCopy, replicas)
	updateDiscoverHostsInConfigMap(configMap, mpiJobCopy, nil)
	f.setUpConfigMap(configMap)
	f.setUpService(newJobService(mpiJobCopy))

	secret, err := newSSHAuthSecret(mpiJobCopy)
	if err != nil {
		t.Fatalf("Creating SSH auth Secret: %v", err)
	}
	secret.OwnerReferences = nil
	f.setUpSecret(secret)

	f.runExpectError(getKey(mpiJob, t))
}

func TestShutdownWorker(t *testing.T) {
	f := newFixture(t, "")
	startTime := metav1.Now()
	completionTime := metav1.Now()

	var replicas int32 = 8
	mpiJob := newGroupJob("test", &replicas, &startTime, &completionTime)
	msg := fmt.Sprintf("GroupJob %s/%s successfully completed.", mpiJob.Namespace, mpiJob.Name)

	updateGroupJobConditions(mpiJob, kubeflow.JobSucceeded, corev1.ConditionTrue, mpiJobSucceededReason, msg)
	f.setUpGroupJob(mpiJob)

	fmjc := f.newFakeGroupJobController()
	mpiJobCopy := mpiJob.DeepCopy()
	scheme.Scheme.Default(mpiJobCopy)
	launcher := fmjc.newLauncherJob(mpiJobCopy)
	launcher.Status.Conditions = append(launcher.Status.Conditions, batchv1.JobCondition{
		Type:   batchv1.JobComplete,
		Status: corev1.ConditionTrue,
	})
	f.setUpLauncher(launcher)

	for i := 0; i < int(replicas); i++ {
		worker := fmjc.newWorker(mpiJobCopy, i)
		f.setUpPod(worker)
	}

	for i := 0; i < int(replicas); i++ {
		name := fmt.Sprintf("%s-%d", mpiJob.Name+workerSuffix, i)
		f.kubeActions = append(f.kubeActions, core.NewDeleteAction(schema.GroupVersionResource{Resource: "pods"}, mpiJob.Namespace, name))
	}

	mpiJobCopy.Status.ReplicaStatuses = map[kubeflow.MPIReplicaType]*kubeflow.ReplicaStatus{
		kubeflow.MPIReplicaTypeWorker: {
			Active:    0,
			Succeeded: 0,
			Failed:    0,
		},
	}
	setUpGroupJobTimestamp(mpiJobCopy, &startTime, &completionTime)
	f.expectUpdateGroupJobStatusAction(mpiJobCopy)

	f.run(getKey(mpiJob, t))
}

func TestCreateSuspendedGroupJob(t *testing.T) {
	impls := []kubeflow.MPIImplementation{kubeflow.MPIImplementationOpenMPI, kubeflow.MPIImplementationIntel, kubeflow.MPIImplementationMPICH}
	for _, implementation := range impls {
		t.Run(string(implementation), func(t *testing.T) {
			f := newFixture(t, "")

			// create a suspended job
			var replicas int32 = 8
			mpiJob := newGroupJob("test", &replicas, nil, nil)
			mpiJob.Spec.RunPolicy.Suspend = ptr.To(true)
			mpiJob.Spec.MPIImplementation = implementation
			f.setUpGroupJob(mpiJob)

			// expect creation of objects
			scheme.Scheme.Default(mpiJob)
			f.expectCreateServiceAction(newJobService(mpiJob))
			cfgMap := newConfigMap(mpiJob, replicas)
			updateDiscoverHostsInConfigMap(cfgMap, mpiJob, nil)
			f.expectCreateConfigMapAction(cfgMap)
			secret, err := newSSHAuthSecret(mpiJob)
			if err != nil {
				t.Fatalf("Failed creating secret")
			}
			f.expectCreateSecretAction(secret)

			// expect creating of the launcher
			fmjc := f.newFakeGroupJobController()
			launcher := fmjc.newLauncherJob(mpiJob)
			launcher.Spec.Suspend = ptr.To(true)
			f.expectCreateJobAction(launcher)

			// expect an update to add the conditions
			mpiJobCopy := mpiJob.DeepCopy()
			mpiJobCopy.Status.ReplicaStatuses = map[kubeflow.MPIReplicaType]*kubeflow.ReplicaStatus{
				kubeflow.MPIReplicaTypeLauncher: {},
				kubeflow.MPIReplicaTypeWorker:   {},
			}
			msg := fmt.Sprintf("GroupJob %s/%s is created.", mpiJob.Namespace, mpiJob.Name)
			updateGroupJobConditions(mpiJobCopy, kubeflow.JobCreated, corev1.ConditionTrue, mpiJobCreatedReason, msg)
			updateGroupJobConditions(mpiJobCopy, kubeflow.JobSuspended, corev1.ConditionTrue, mpiJobSuspendedReason, "GroupJob suspended")
			msg = fmt.Sprintf("GroupJob %s/%s is suspended.", mpiJob.Namespace, mpiJob.Name)
			updateGroupJobConditions(mpiJobCopy, kubeflow.JobRunning, corev1.ConditionFalse, mpiJobSuspendedReason, msg)
			f.expectUpdateGroupJobStatusAction(mpiJobCopy)

			f.run(getKey(mpiJob, t))
		})
	}
}

func TestSuspendedRunningGroupJob(t *testing.T) {
	f := newFixture(t, "")

	// setup a running GroupJob with a launcher
	var replicas int32 = 8
	startTime := metav1.Now()
	mpiJob := newGroupJob("test", &replicas, &startTime, nil)
	mpiJob.Spec.RunPolicy.Suspend = ptr.To(false)
	msg := fmt.Sprintf("GroupJob %s/%s is created.", mpiJob.Namespace, mpiJob.Name)
	updateGroupJobConditions(mpiJob, kubeflow.JobCreated, corev1.ConditionTrue, mpiJobCreatedReason, msg)
	msg = fmt.Sprintf("GroupJob %s/%s is running.", mpiJob.Namespace, mpiJob.Name)
	updateGroupJobConditions(mpiJob, kubeflow.JobRunning, corev1.ConditionTrue, mpiJobRunningReason, msg)

	mpiJob.Status.ReplicaStatuses = map[kubeflow.MPIReplicaType]*kubeflow.ReplicaStatus{
		kubeflow.MPIReplicaTypeLauncher: {
			Active: 1,
		},
		kubeflow.MPIReplicaTypeWorker: {
			Active: replicas,
		},
	}

	f.setUpGroupJob(mpiJob)

	// setup workers
	fmjc := f.newFakeGroupJobController()
	var runningPodList []*corev1.Pod
	for i := 0; i < int(replicas); i++ {
		worker := fmjc.newWorker(mpiJob, i)
		worker.Status.Phase = corev1.PodRunning
		runningPodList = append(runningPodList, worker)
		f.setUpPod(worker)
	}

	// setup objects
	scheme.Scheme.Default(mpiJob)
	f.setUpService(newJobService(mpiJob))

	cfgMap := newConfigMap(mpiJob, replicas)
	updateDiscoverHostsInConfigMap(cfgMap, mpiJob, runningPodList)
	f.setUpConfigMap(cfgMap)
	secret, err := newSSHAuthSecret(mpiJob)
	if err != nil {
		t.Fatalf("Failed creating secret")
	}
	f.setUpSecret(secret)

	// setup launcher and its pod
	launcher := fmjc.newLauncherJob(mpiJob)
	launcher.Spec.Suspend = ptr.To(false)
	launcherPod := mockJobPod(launcher)
	launcherPod.Status.Phase = corev1.PodRunning
	f.setUpLauncher(launcher)
	f.setUpPod(launcherPod)

	// transition the GroupJob into suspended state
	mpiJob.Spec.RunPolicy.Suspend = ptr.To(true)

	// expect moving the launcher pod into suspended state
	launcherCopy := launcher.DeepCopy()
	launcherCopy.Spec.Suspend = ptr.To(true)
	f.expectUpdateJobAction(launcherCopy)

	// expect removal of the pods
	for i := 0; i < int(replicas); i++ {
		name := fmt.Sprintf("%s-%d", mpiJob.Name+workerSuffix, i)
		f.kubeActions = append(f.kubeActions, core.NewDeleteAction(schema.GroupVersionResource{Resource: "pods"}, mpiJob.Namespace, name))
	}

	// expect MPI job status update to add the suspend condition
	mpiJobCopy := mpiJob.DeepCopy()
	updateGroupJobConditions(mpiJobCopy, kubeflow.JobSuspended, corev1.ConditionTrue, mpiJobSuspendedReason, "GroupJob suspended")
	msg = fmt.Sprintf("GroupJob %s/%s is suspended.", mpiJobCopy.Namespace, mpiJobCopy.Name)
	updateGroupJobConditions(mpiJobCopy, kubeflow.JobRunning, corev1.ConditionFalse, mpiJobSuspendedReason, msg)
	mpiJobCopy.Status.ReplicaStatuses = map[kubeflow.MPIReplicaType]*kubeflow.ReplicaStatus{
		// the launcher pod remains active. In live system it gets deleted by
		// the launcher's Job controller.
		kubeflow.MPIReplicaTypeLauncher: {
			Active: 1,
		},
		kubeflow.MPIReplicaTypeWorker: {},
	}
	f.expectUpdateGroupJobStatusAction(mpiJobCopy)

	f.run(getKey(mpiJob, t))
}

func TestResumeGroupJob(t *testing.T) {
	fakeClock := clocktesting.NewFakeClock(time.Now().Truncate(time.Second))
	f := newFixture(t, "")

	// create a suspended job
	var replicas int32 = 8
	startTime := metav1.Now()
	mpiJob := newGroupJob("test", &replicas, &startTime, nil)
	mpiJob.Spec.RunPolicy.Suspend = ptr.To(true)
	msg := fmt.Sprintf("GroupJob %s/%s is created.", mpiJob.Namespace, mpiJob.Name)
	updateGroupJobConditions(mpiJob, kubeflow.JobCreated, corev1.ConditionTrue, mpiJobCreatedReason, msg)
	updateGroupJobConditions(mpiJob, kubeflow.JobSuspended, corev1.ConditionTrue, mpiJobSuspendedReason, "GroupJob suspended")
	msg = fmt.Sprintf("GroupJob %s/%s is suspended.", mpiJob.Namespace, mpiJob.Name)
	updateGroupJobConditions(mpiJob, kubeflow.JobRunning, corev1.ConditionFalse, mpiJobSuspendedReason, msg)
	mpiJob.Status.ReplicaStatuses = map[kubeflow.MPIReplicaType]*kubeflow.ReplicaStatus{
		kubeflow.MPIReplicaTypeLauncher: {},
		kubeflow.MPIReplicaTypeWorker:   {},
	}
	f.setUpGroupJob(mpiJob)

	// expect creation of objects
	scheme.Scheme.Default(mpiJob)
	f.expectCreateServiceAction(newJobService(mpiJob))
	cfgMap := newConfigMap(mpiJob, replicas)
	updateDiscoverHostsInConfigMap(cfgMap, mpiJob, nil)
	f.setUpConfigMap(cfgMap)
	secret, err := newSSHAuthSecret(mpiJob)
	if err != nil {
		t.Fatalf("Failed creating secret")
	}
	f.setUpSecret(secret)

	// expect creating of the launcher
	fmjc := f.newFakeGroupJobController()
	launcher := fmjc.newLauncherJob(mpiJob)
	launcher.Spec.Suspend = ptr.To(true)
	f.setUpLauncher(launcher)

	// move the timer by a second so that the StartTime is updated after resume
	fakeClock.Sleep(time.Second)

	// resume the GroupJob
	mpiJob.Spec.RunPolicy.Suspend = ptr.To(false)

	// expect creation of the pods
	for i := 0; i < int(replicas); i++ {
		worker := fmjc.newWorker(mpiJob, i)
		f.kubeActions = append(f.kubeActions, core.NewCreateAction(schema.GroupVersionResource{Resource: "pods"}, mpiJob.Namespace, worker))
	}

	// expect the launcher update to resume it
	launcherCopy := launcher.DeepCopy()
	launcherCopy.Spec.Suspend = ptr.To(false)
	f.expectUpdateJobAction(launcherCopy)

	// expect an update to add the conditions
	mpiJobCopy := mpiJob.DeepCopy()
	mpiJobCopy.Status.StartTime = &metav1.Time{Time: fakeClock.Now()}
	updateGroupJobConditions(mpiJobCopy, kubeflow.JobSuspended, corev1.ConditionFalse, "GroupJobResumed", "GroupJob resumed")
	f.expectUpdateGroupJobStatusAction(mpiJobCopy)

	f.runWithClock(getKey(mpiJob, t), fakeClock)
}

func TestWorkerNotControlledByUs(t *testing.T) {
	f := newFixture(t, "")
	startTime := metav1.Now()
	completionTime := metav1.Now()

	var replicas int32 = 8
	mpiJob := newGroupJob("test", &replicas, &startTime, &completionTime)
	f.setUpGroupJob(mpiJob)

	mpiJobCopy := mpiJob.DeepCopy()
	scheme.Scheme.Default(mpiJobCopy)
	configMap := newConfigMap(mpiJobCopy, replicas)
	updateDiscoverHostsInConfigMap(configMap, mpiJobCopy, nil)
	f.setUpConfigMap(configMap)
	f.setUpService(newJobService(mpiJobCopy))
	secret, err := newSSHAuthSecret(mpiJobCopy)
	if err != nil {
		t.Fatalf("Creating SSH auth secret: %v", err)
	}
	f.setUpSecret(secret)
	fmjc := f.newFakeGroupJobController()

	for i := 0; i < int(replicas); i++ {
		worker := fmjc.newWorker(mpiJobCopy, i)
		worker.OwnerReferences = nil
		f.setUpPod(worker)
	}

	f.runExpectError(getKey(mpiJob, t))
}

func TestLauncherActiveWorkerNotReady(t *testing.T) {
	f := newFixture(t, "")
	startTime := metav1.Now()
	completionTime := metav1.Now()

	var replicas int32 = 8
	mpiJob := newGroupJob("test", &replicas, &startTime, &completionTime)
	f.setUpGroupJob(mpiJob)

	mpiJobCopy := mpiJob.DeepCopy()
	scheme.Scheme.Default(mpiJobCopy)
	configMap := newConfigMap(mpiJobCopy, replicas)
	updateDiscoverHostsInConfigMap(configMap, mpiJobCopy, nil)
	f.setUpConfigMap(configMap)
	f.setUpService(newJobService(mpiJobCopy))
	secret, err := newSSHAuthSecret(mpiJobCopy)
	if err != nil {
		t.Fatalf("Creating SSH auth secret: %v", err)
	}
	f.setUpSecret(secret)

	fmjc := f.newFakeGroupJobController()
	launcher := fmjc.newLauncherJob(mpiJobCopy)
	launcherPod := mockJobPod(launcher)
	launcherPod.Status.Phase = corev1.PodRunning
	f.setUpLauncher(launcher)
	f.setUpPod(launcherPod)

	for i := 0; i < int(replicas); i++ {
		worker := fmjc.newWorker(mpiJobCopy, i)
		worker.Status.Phase = corev1.PodPending
		f.setUpPod(worker)
	}
	msg := fmt.Sprintf("GroupJob %s/%s is created.", mpiJob.Namespace, mpiJob.Name)
	updateGroupJobConditions(mpiJobCopy, kubeflow.JobCreated, corev1.ConditionTrue, mpiJobCreatedReason, msg)
	mpiJobCopy.Status.ReplicaStatuses = map[kubeflow.MPIReplicaType]*kubeflow.ReplicaStatus{
		kubeflow.MPIReplicaTypeLauncher: {
			Active:    1,
			Succeeded: 0,
			Failed:    0,
		},
		kubeflow.MPIReplicaTypeWorker: {
			Active:    0,
			Succeeded: 0,
			Failed:    0,
		},
	}
	setUpGroupJobTimestamp(mpiJobCopy, &startTime, &completionTime)
	f.expectUpdateGroupJobStatusAction(mpiJobCopy)

	f.run(getKey(mpiJob, t))
}

func TestLauncherActiveWorkerReady(t *testing.T) {
	f := newFixture(t, "")
	startTime := metav1.Now()
	completionTime := metav1.Now()

	var replicas int32 = 8
	mpiJob := newGroupJob("test", &replicas, &startTime, &completionTime)
	f.setUpGroupJob(mpiJob)

	mpiJobCopy := mpiJob.DeepCopy()
	scheme.Scheme.Default(mpiJobCopy)
	f.setUpService(newJobService(mpiJobCopy))
	secret, err := newSSHAuthSecret(mpiJobCopy)
	if err != nil {
		t.Fatalf("Creating SSH auth secret: %v", err)
	}
	f.setUpSecret(secret)

	fmjc := f.newFakeGroupJobController()
	launcher := fmjc.newLauncherJob(mpiJobCopy)
	launcherPod := mockJobPod(launcher)
	launcherPod.Status.Phase = corev1.PodRunning
	f.setUpLauncher(launcher)
	f.setUpPod(launcherPod)

	var runningPodList []*corev1.Pod
	for i := 0; i < int(replicas); i++ {
		worker := fmjc.newWorker(mpiJobCopy, i)
		worker.Status.Phase = corev1.PodRunning
		runningPodList = append(runningPodList, worker)
		f.setUpPod(worker)
	}

	configMap := newConfigMap(mpiJobCopy, replicas)
	updateDiscoverHostsInConfigMap(configMap, mpiJobCopy, runningPodList)
	f.setUpConfigMap(configMap)

	mpiJobCopy.Status.ReplicaStatuses = map[kubeflow.MPIReplicaType]*kubeflow.ReplicaStatus{
		kubeflow.MPIReplicaTypeLauncher: {
			Active:    1,
			Succeeded: 0,
			Failed:    0,
		},
		kubeflow.MPIReplicaTypeWorker: {
			Active:    8,
			Succeeded: 0,
			Failed:    0,
		},
	}
	setUpGroupJobTimestamp(mpiJobCopy, &startTime, &completionTime)
	msg := fmt.Sprintf("GroupJob %s/%s is created.", mpiJob.Namespace, mpiJob.Name)
	updateGroupJobConditions(mpiJobCopy, kubeflow.JobCreated, corev1.ConditionTrue, mpiJobCreatedReason, msg)
	msg = fmt.Sprintf("GroupJob %s/%s is running.", mpiJob.Namespace, mpiJob.Name)
	updateGroupJobConditions(mpiJobCopy, kubeflow.JobRunning, corev1.ConditionTrue, mpiJobRunningReason, msg)
	f.expectUpdateGroupJobStatusAction(mpiJobCopy)

	f.run(getKey(mpiJob, t))
}

func TestWorkerReady(t *testing.T) {
	f := newFixture(t, "")
	startTime := metav1.Now()
	completionTime := metav1.Now()

	var replicas int32 = 16
	mpiJob := newGroupJob("test", &replicas, &startTime, &completionTime)
	f.setUpGroupJob(mpiJob)

	mpiJobCopy := mpiJob.DeepCopy()
	scheme.Scheme.Default(mpiJobCopy)
	f.setUpService(newJobService(mpiJobCopy))
	secret, err := newSSHAuthSecret(mpiJobCopy)
	if err != nil {
		t.Fatalf("Creating SSH auth secret: %v", err)
	}
	f.setUpSecret(secret)

	fmjc := f.newFakeGroupJobController()

	var runningPodList []*corev1.Pod
	for i := 0; i < 16; i++ {
		worker := fmjc.newWorker(mpiJobCopy, i)
		worker.Status.Phase = corev1.PodRunning
		runningPodList = append(runningPodList, worker)
		f.setUpPod(worker)
	}

	configMap := newConfigMap(mpiJobCopy, replicas)
	updateDiscoverHostsInConfigMap(configMap, mpiJobCopy, runningPodList)
	f.setUpConfigMap(configMap)

	expLauncher := fmjc.newLauncherJob(mpiJobCopy)
	f.expectCreateJobAction(expLauncher)

	mpiJobCopy.Status.ReplicaStatuses = map[kubeflow.MPIReplicaType]*kubeflow.ReplicaStatus{
		kubeflow.MPIReplicaTypeLauncher: {
			Active:    0,
			Succeeded: 0,
			Failed:    0,
		},
		kubeflow.MPIReplicaTypeWorker: {
			Active:    16,
			Succeeded: 0,
			Failed:    0,
		},
	}
	msg := fmt.Sprintf("GroupJob %s/%s is created.", mpiJob.Namespace, mpiJob.Name)
	updateGroupJobConditions(mpiJobCopy, kubeflow.JobCreated, corev1.ConditionTrue, mpiJobCreatedReason, msg)
	setUpGroupJobTimestamp(mpiJobCopy, &startTime, &completionTime)
	f.expectUpdateGroupJobStatusAction(mpiJobCopy)

	f.run(getKey(mpiJob, t))
}

func TestNewLauncherAndWorker(t *testing.T) {
	cases := map[string]struct {
		job          kubeflow.GroupJob
		workerIndex  int
		wantLauncher batchv1.Job
		wantWorker   corev1.Pod
	}{
		"defaults": {
			job: kubeflow.GroupJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
				Spec: kubeflow.GroupJobSpec{
					MPIReplicaSpecs: map[kubeflow.MPIReplicaType]*kubeflow.ReplicaSpec{
						kubeflow.MPIReplicaTypeLauncher: {
							Template: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Containers: []corev1.Container{{}},
								},
							},
						},
						kubeflow.MPIReplicaTypeWorker: {
							Template: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Containers: []corev1.Container{{}},
								},
							},
						},
					},
				},
			},
			wantLauncher: batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-launcher",
					Namespace: "bar",
					Labels: map[string]string{
						"app": "foo",
					},
				},
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								kubeflow.OperatorNameLabel: kubeflow.OperatorName,
								kubeflow.JobNameLabel:      "foo",
								kubeflow.JobRoleLabel:      "launcher",
							},
						},
						Spec: corev1.PodSpec{
							Hostname:      "foo-launcher",
							Subdomain:     "foo",
							RestartPolicy: corev1.RestartPolicyOnFailure,
							Containers: []corev1.Container{
								{
									Env: joinEnvVars(
										launcherEnvVars,
										ompiEnvVars,
										corev1.EnvVar{Name: openMPISlotsEnv, Value: "1"},
										nvidiaDisableEnvVars),
									VolumeMounts: []corev1.VolumeMount{
										{Name: "ssh-auth", MountPath: "/root/.ssh"},
										{Name: "mpi-job-config", MountPath: "/etc/mpi"},
									},
								},
							},
							Volumes: []corev1.Volume{
								{
									Name: "ssh-auth",
									VolumeSource: corev1.VolumeSource{
										Secret: &corev1.SecretVolumeSource{
											DefaultMode: ptr.To[int32](0600),
											SecretName:  "foo-ssh",
											Items:       sshVolumeItems,
										},
									},
								},
								{
									Name: "mpi-job-config",
									VolumeSource: corev1.VolumeSource{
										ConfigMap: &corev1.ConfigMapVolumeSource{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: "foo-config",
											},
											Items: configVolumeItems,
										},
									},
								},
							},
						},
					},
				},
			},
			wantWorker: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-worker-0",
					Namespace: "bar",
					Labels: map[string]string{
						kubeflow.OperatorNameLabel: kubeflow.OperatorName,
						kubeflow.JobNameLabel:      "foo",
						kubeflow.JobRoleLabel:      "worker",
						kubeflow.ReplicaIndexLabel: "0",
					},
				},
				Spec: corev1.PodSpec{
					Hostname:  "foo-worker-0",
					Subdomain: "foo",
					DNSConfig: &corev1.PodDNSConfig{
						Searches: []string{"foo.bar.svc.cluster.local"},
					},
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Command: []string{"/usr/sbin/sshd", "-De"},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "ssh-auth", MountPath: "/root/.ssh"},
							},
							Env: workerEnvVars,
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "ssh-auth",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									DefaultMode: ptr.To[int32](0600),
									SecretName:  "foo-ssh",
									Items:       sshVolumeItems,
								},
							},
						},
					},
				},
			},
		},
		"launcher-as-worker": {
			job: kubeflow.GroupJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
				Spec: kubeflow.GroupJobSpec{
					RunLauncherAsWorker: ptr.To(true),
					MPIReplicaSpecs: map[kubeflow.MPIReplicaType]*kubeflow.ReplicaSpec{
						kubeflow.MPIReplicaTypeLauncher: {
							Template: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Containers: []corev1.Container{{}},
								},
							},
						},
						kubeflow.MPIReplicaTypeWorker: {
							Template: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Containers: []corev1.Container{{}},
								},
							},
						},
					},
				},
			},
			wantLauncher: batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-launcher",
					Namespace: "bar",
					Labels: map[string]string{
						"app": "foo",
					},
				},
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								kubeflow.OperatorNameLabel: kubeflow.OperatorName,
								kubeflow.JobNameLabel:      "foo",
								kubeflow.JobRoleLabel:      "launcher",
								kubeflow.ReplicaIndexLabel: "0",
							},
						},
						Spec: corev1.PodSpec{
							Hostname:      "foo-launcher",
							Subdomain:     "foo",
							RestartPolicy: corev1.RestartPolicyOnFailure,
							Containers: []corev1.Container{
								{
									Env: joinEnvVars(
										launcherEnvVars,
										ompiEnvVars,
										corev1.EnvVar{Name: openMPISlotsEnv, Value: "1"},
									),
									VolumeMounts: []corev1.VolumeMount{
										{Name: "ssh-auth", MountPath: "/root/.ssh"},
										{Name: "mpi-job-config", MountPath: "/etc/mpi"},
									},
								},
							},
							Volumes: []corev1.Volume{
								{
									Name: "ssh-auth",
									VolumeSource: corev1.VolumeSource{
										Secret: &corev1.SecretVolumeSource{
											DefaultMode: ptr.To[int32](0600),
											SecretName:  "foo-ssh",
											Items:       sshVolumeItems,
										},
									},
								},
								{
									Name: "mpi-job-config",
									VolumeSource: corev1.VolumeSource{
										ConfigMap: &corev1.ConfigMapVolumeSource{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: "foo-config",
											},
											Items: configVolumeItems,
										},
									},
								},
							},
						},
					},
				},
			},
			wantWorker: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-worker-0",
					Namespace: "bar",
					Labels: map[string]string{
						kubeflow.OperatorNameLabel: kubeflow.OperatorName,
						kubeflow.JobNameLabel:      "foo",
						kubeflow.JobRoleLabel:      "worker",
						kubeflow.ReplicaIndexLabel: "1",
					},
				},
				Spec: corev1.PodSpec{
					Hostname:  "foo-worker-0",
					Subdomain: "foo",
					DNSConfig: &corev1.PodDNSConfig{
						Searches: []string{"foo.bar.svc.cluster.local"},
					},
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Command: []string{"/usr/sbin/sshd", "-De"},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "ssh-auth", MountPath: "/root/.ssh"},
							},
							Env: workerEnvVars,
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "ssh-auth",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									DefaultMode: ptr.To[int32](0600),
									SecretName:  "foo-ssh",
									Items:       sshVolumeItems,
								},
							},
						},
					},
				},
			},
		},
		"overrides": {
			job: kubeflow.GroupJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bar",
					Namespace: "foo",
				},
				Spec: kubeflow.GroupJobSpec{
					SSHAuthMountPath:  "/home/mpiuser/.ssh",
					SlotsPerWorker:    ptr.To[int32](5),
					MPIImplementation: kubeflow.MPIImplementationIntel,
					RunPolicy: kubeflow.RunPolicy{
						TTLSecondsAfterFinished: ptr.To[int32](1),
						ActiveDeadlineSeconds:   ptr.To[int64](2),
						BackoffLimit:            ptr.To[int32](3),
					},
					MPIReplicaSpecs: map[kubeflow.MPIReplicaType]*kubeflow.ReplicaSpec{
						kubeflow.MPIReplicaTypeLauncher: {
							RestartPolicy: kubeflow.RestartPolicyOnFailure,
							Template: corev1.PodTemplateSpec{
								ObjectMeta: metav1.ObjectMeta{
									Labels: map[string]string{"foo": "bar"},
								},
								Spec: corev1.PodSpec{
									HostNetwork: true,
									Containers: []corev1.Container{
										{
											Env: []corev1.EnvVar{
												{Name: "FOO", Value: "bar"},
											},
											SecurityContext: &corev1.SecurityContext{
												RunAsUser: ptr.To[int64](1000),
											},
											VolumeMounts: []corev1.VolumeMount{
												{Name: "fool-vol", MountPath: "/mnt/foo"},
											},
										},
										{},
									},
									Volumes: []corev1.Volume{
										{Name: "foo-vol"},
									},
								},
							},
						},
						kubeflow.MPIReplicaTypeWorker: {
							Template: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									HostNetwork: true,
									Containers: []corev1.Container{
										{
											Command: []string{"/entrypoint.sh"},
											Env: []corev1.EnvVar{
												{Name: "FOO", Value: "bar"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			workerIndex: 12,
			wantLauncher: batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bar-launcher",
					Namespace: "foo",
					Labels: map[string]string{
						"app": "bar",
					},
				},
				Spec: batchv1.JobSpec{
					TTLSecondsAfterFinished: ptr.To[int32](1),
					ActiveDeadlineSeconds:   ptr.To[int64](2),
					BackoffLimit:            ptr.To[int32](3),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"foo":                      "bar",
								kubeflow.OperatorNameLabel: kubeflow.OperatorName,
								kubeflow.JobNameLabel:      "bar",
								kubeflow.JobRoleLabel:      "launcher",
							},
						},
						Spec: corev1.PodSpec{
							HostNetwork:   true,
							DNSPolicy:     corev1.DNSClusterFirstWithHostNet,
							Hostname:      "bar-launcher",
							Subdomain:     "bar",
							RestartPolicy: corev1.RestartPolicyOnFailure,
							Containers: []corev1.Container{
								{
									SecurityContext: &corev1.SecurityContext{
										RunAsUser: ptr.To[int64](1000),
									},
									Env: joinEnvVars(
										corev1.EnvVar{Name: "FOO", Value: "bar"},
										launcherEnvVars,
										intelEnvVars,
										corev1.EnvVar{Name: "I_MPI_PERHOST", Value: "5"},
										nvidiaDisableEnvVars),
									VolumeMounts: []corev1.VolumeMount{
										{Name: "fool-vol", MountPath: "/mnt/foo"},
										{Name: "ssh-auth", MountPath: "/home/mpiuser/.ssh"},
										{Name: "mpi-job-config", MountPath: "/etc/mpi"},
									},
								},
								{},
							},
							Volumes: []corev1.Volume{
								{Name: "foo-vol"},
								{
									Name: "ssh-auth",
									VolumeSource: corev1.VolumeSource{
										Secret: &corev1.SecretVolumeSource{
											SecretName: "bar-ssh",
											Items:      sshVolumeItems,
										},
									},
								},
								{
									Name: "mpi-job-config",
									VolumeSource: corev1.VolumeSource{
										ConfigMap: &corev1.ConfigMapVolumeSource{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: "bar-config",
											},
											Items: configVolumeItems,
										},
									},
								},
							},
						},
					},
				},
			},
			wantWorker: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bar-worker-12",
					Namespace: "foo",
					Labels: map[string]string{
						kubeflow.OperatorNameLabel: kubeflow.OperatorName,
						kubeflow.JobNameLabel:      "bar",
						kubeflow.JobRoleLabel:      "worker",
						kubeflow.ReplicaIndexLabel: "12",
					},
				},
				Spec: corev1.PodSpec{
					HostNetwork: true,
					DNSPolicy:   corev1.DNSClusterFirstWithHostNet,
					Hostname:    "bar-worker-12",
					Subdomain:   "bar",
					DNSConfig: &corev1.PodDNSConfig{
						Searches: []string{"bar.foo.svc.cluster.local"},
					},
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Command: []string{"/entrypoint.sh"},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "ssh-auth", MountPath: "/home/mpiuser/.ssh"},
							},
							Env: joinEnvVars(corev1.EnvVar{Name: "FOO", Value: "bar"}, workerEnvVars),
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "ssh-auth",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: "bar-ssh",
									Items:      sshVolumeItems,
								},
							},
						},
					},
				},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			job := tc.job.DeepCopy()
			scheme.Scheme.Default(job)
			ctrl := &GroupJobController{}
			launcher := ctrl.newLauncherJob(job)
			if !metav1.IsControlledBy(launcher, job) {
				t.Errorf("Created launcher Pod is not controlled by Job")
			}
			if diff := cmp.Diff(&tc.wantLauncher, launcher, ignoreReferences); diff != "" {
				t.Errorf("Unexpected launcher pod (-want,+got):\n%s", diff)
			}
			worker := ctrl.newWorker(job, tc.workerIndex)
			if !metav1.IsControlledBy(worker, job) {
				t.Errorf("Created worker Pod is not controlled by Job")
			}
			if diff := cmp.Diff(&tc.wantWorker, worker, ignoreReferences); diff != "" {
				t.Errorf("Unexpected launcher pod (-want,+got):\n%s", diff)
			}
		})
	}
}

func TestNewConfigMap(t *testing.T) {
	testCases := map[string]struct {
		mpiJob         *kubeflow.GroupJob
		workerReplicas int32
		wantCM         *corev1.ConfigMap
	}{
		"OpenMPI without slots, enable launcher as worker": {
			mpiJob: &kubeflow.GroupJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "openmpi-without-slots",
					Namespace: "tenant-a",
				},
				Spec: kubeflow.GroupJobSpec{
					MPIImplementation:   kubeflow.MPIImplementationOpenMPI,
					RunLauncherAsWorker: ptr.To(true),
				},
			},
			workerReplicas: 2,
			wantCM: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "openmpi-without-slots-config",
					Namespace: "tenant-a",
					Labels: map[string]string{
						"app": "openmpi-without-slots",
					},
				},
				Data: map[string]string{
					"hostfile": "openmpi-without-slots-launcher.openmpi-without-slots.tenant-a.svc slots=1\nopenmpi-without-slots-worker-0.openmpi-without-slots.tenant-a.svc slots=1\nopenmpi-without-slots-worker-1.openmpi-without-slots.tenant-a.svc slots=1\n",
				},
			},
		},
		"OpenMPI without slots, zero explicit workers": {
			mpiJob: &kubeflow.GroupJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "openmpi-without-slots",
					Namespace: "tenant-a",
				},
				Spec: kubeflow.GroupJobSpec{
					MPIImplementation:   kubeflow.MPIImplementationOpenMPI,
					RunLauncherAsWorker: ptr.To(true),
				},
			},
			workerReplicas: 0,
			wantCM: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "openmpi-without-slots-config",
					Namespace: "tenant-a",
					Labels: map[string]string{
						"app": "openmpi-without-slots",
					},
				},
				Data: map[string]string{
					"hostfile": "openmpi-without-slots-launcher.openmpi-without-slots.tenant-a.svc slots=1\n",
				},
			},
		},
		"OpenMPI without slots, disable launcher as worker": {
			mpiJob: &kubeflow.GroupJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "openmpi-without-slots",
					Namespace: "tenant-a",
				},
				Spec: kubeflow.GroupJobSpec{
					MPIImplementation: kubeflow.MPIImplementationOpenMPI,
				},
			},
			workerReplicas: 2,
			wantCM: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "openmpi-without-slots-config",
					Namespace: "tenant-a",
					Labels: map[string]string{
						"app": "openmpi-without-slots",
					},
				},
				Data: map[string]string{
					"hostfile": "openmpi-without-slots-worker-0.openmpi-without-slots.tenant-a.svc slots=1\nopenmpi-without-slots-worker-1.openmpi-without-slots.tenant-a.svc slots=1\n",
				},
			},
		},
		"IntelMPI with slots": {
			mpiJob: &kubeflow.GroupJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "intelmpi-with-slots",
					Namespace: "project-x",
				},
				Spec: kubeflow.GroupJobSpec{
					SlotsPerWorker:    ptr.To[int32](10),
					MPIImplementation: kubeflow.MPIImplementationIntel,
				},
			},
			workerReplicas: 1,
			wantCM: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "intelmpi-with-slots-config",
					Namespace: "project-x",
					Labels: map[string]string{
						"app": "intelmpi-with-slots",
					},
				},
				Data: map[string]string{
					"hostfile": "intelmpi-with-slots-worker-0.intelmpi-with-slots.project-x.svc:10\n",
				},
			},
		},
		"MPICH with slots": {
			mpiJob: &kubeflow.GroupJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mpich-with-slots",
					Namespace: "project-x",
				},
				Spec: kubeflow.GroupJobSpec{
					SlotsPerWorker:    ptr.To[int32](10),
					MPIImplementation: kubeflow.MPIImplementationMPICH,
				},
			},
			workerReplicas: 1,
			wantCM: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mpich-with-slots-config",
					Namespace: "project-x",
					Labels: map[string]string{
						"app": "mpich-with-slots",
					},
				},
				Data: map[string]string{
					"hostfile": "mpich-with-slots-worker-0.mpich-with-slots.project-x.svc:10\n",
				},
			},
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			cm := newConfigMap(tc.mpiJob, tc.workerReplicas)
			if !metav1.IsControlledBy(cm, tc.mpiJob) {
				t.Errorf("Created configMap is not controlled by GroupJob")
			}
			if diff := cmp.Diff(tc.wantCM, cm, ignoreReferences); len(diff) != 0 {
				t.Errorf("Unexpected configMap (-want,+got):\n%s", diff)
			}
		})
	}
}

func joinEnvVars(evs ...interface{}) []corev1.EnvVar {
	var result []corev1.EnvVar
	for _, ev := range evs {
		switch v := ev.(type) {
		case corev1.EnvVar:
			result = append(result, v)
		case []corev1.EnvVar:
			result = append(result, v...)
		default:
			panic("must by of type EnvVar or []EnvVar")
		}
	}
	return result
}

func mockJobPod(job *batchv1.Job) *corev1.Pod {
	job.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"controller-uid": string(uuid.NewUUID()),
		},
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      job.Name + "-" + rand.String(5),
			Labels:    job.Spec.Selector.MatchLabels,
			Namespace: job.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(job, batchv1.SchemeGroupVersion.WithKind("Job")),
			},
		},
	}
}

func (f *fixture) newFakeGroupJobController() *GroupJobController {
	kubeClient := k8sfake.NewSimpleClientset(f.kubeObjects...)

	k8sI := kubeinformers.NewSharedInformerFactory(kubeClient, noResyncPeriodFunc())
	return &GroupJobController{
		recorder:  &record.FakeRecorder{},
		podLister: k8sI.Core().V1().Pods().Lister(),
	}
}
