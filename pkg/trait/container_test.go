/*
Licensed to the Apache Software Foundation (ASF) under one or more
contributor license agreements.  See the NOTICE file distributed with
this work for additional information regarding copyright ownership.
The ASF licenses this file to You under the Apache License, Version 2.0
(the "License"); you may not use this file except in compliance with
the License.  You may obtain a copy of the License at

   http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package trait

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"

	ctrl "sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/apache/camel-k/v2/pkg/apis/camel/v1"
	traitv1 "github.com/apache/camel-k/v2/pkg/apis/camel/v1/trait"
	"github.com/apache/camel-k/v2/pkg/util/camel"
	"github.com/apache/camel-k/v2/pkg/util/kubernetes"
	"github.com/apache/camel-k/v2/pkg/util/test"
)

func TestContainerWithDefaults(t *testing.T) {
	catalog, err := camel.DefaultCatalog()
	require.NoError(t, err)

	client, _ := test.NewFakeClient()
	traitCatalog := NewCatalog(nil)

	environment := Environment{
		CamelCatalog: catalog,
		Catalog:      traitCatalog,
		Client:       client,
		Integration: &v1.Integration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ServiceTestName,
				Namespace: "ns",
			},
			Status: v1.IntegrationStatus{
				Phase: v1.IntegrationPhaseDeploying,
			},
			Spec: v1.IntegrationSpec{
				Profile: v1.TraitProfileKubernetes,
			},
		},
		IntegrationKit: &v1.IntegrationKit{
			Status: v1.IntegrationKitStatus{
				Phase: v1.IntegrationKitPhaseReady,
			},
		},
		Platform: &v1.IntegrationPlatform{
			Spec: v1.IntegrationPlatformSpec{
				Cluster: v1.IntegrationPlatformClusterOpenShift,
				Build: v1.IntegrationPlatformBuildSpec{
					PublishStrategy: v1.IntegrationPlatformBuildPublishStrategyS2I,
					Registry:        v1.RegistrySpec{Address: "registry"},
					RuntimeVersion:  catalog.Runtime.Version,
				},
			},
			Status: v1.IntegrationPlatformStatus{
				Phase: v1.IntegrationPlatformPhaseReady,
			},
		},
		EnvVars:        make([]corev1.EnvVar, 0),
		ExecutedTraits: make([]Trait, 0),
		Resources:      kubernetes.NewCollection(),
	}
	environment.Platform.ResyncStatusFullConfig()

	conditions, err := traitCatalog.apply(&environment)

	require.NoError(t, err)
	assert.Empty(t, conditions)
	assert.NotEmpty(t, environment.ExecutedTraits)
	assert.NotNil(t, environment.GetTrait("deployment"))
	assert.NotNil(t, environment.GetTrait("container"))

	d := environment.Resources.GetDeploymentForIntegration(environment.Integration)

	assert.NotNil(t, d)
	assert.Len(t, d.Spec.Template.Spec.Containers, 1)
	assert.Equal(t, defaultContainerName, d.Spec.Template.Spec.Containers[0].Name)
}

func TestContainerWithOpenshift(t *testing.T) {
	catalog, err := camel.DefaultCatalog()
	require.NoError(t, err)

	// Integration is in another constrained namespace
	constrainedIntNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "myuser",
			Annotations: map[string]string{
				"openshift.io/sa.scc.mcs":                 "s0:c26,c5",
				"openshift.io/sa.scc.supplemental-groups": "1000860000/10000",
				"openshift.io/sa.scc.uid-range":           "1000860000/10000",
			},
		},
	}

	client, _ := test.NewFakeClient(constrainedIntNamespace)
	traitCatalog := NewCatalog(nil)

	// enable openshift
	fakeClient := client.(*test.FakeClient) //nolint
	fakeClient.EnableOpenshiftDiscovery()

	environment := Environment{
		CamelCatalog: catalog,
		Catalog:      traitCatalog,
		Client:       client,
		Integration: &v1.Integration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ServiceTestName,
				Namespace: "myuser",
			},
			Status: v1.IntegrationStatus{
				Phase: v1.IntegrationPhaseDeploying,
			},
			Spec: v1.IntegrationSpec{
				Profile: v1.TraitProfileKubernetes,
			},
		},
		IntegrationKit: &v1.IntegrationKit{
			Status: v1.IntegrationKitStatus{
				Phase: v1.IntegrationKitPhaseReady,
			},
		},
		Platform: &v1.IntegrationPlatform{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
			},
			Spec: v1.IntegrationPlatformSpec{
				Cluster: v1.IntegrationPlatformClusterOpenShift,
				Build: v1.IntegrationPlatformBuildSpec{
					PublishStrategy: v1.IntegrationPlatformBuildPublishStrategyS2I,
					Registry:        v1.RegistrySpec{Address: "registry"},
					RuntimeVersion:  catalog.Runtime.Version,
				},
			},
			Status: v1.IntegrationPlatformStatus{
				Phase: v1.IntegrationPlatformPhaseReady,
			},
		},
		EnvVars:        make([]corev1.EnvVar, 0),
		ExecutedTraits: make([]Trait, 0),
		Resources:      kubernetes.NewCollection(),
	}
	environment.Platform.ResyncStatusFullConfig()

	conditions, err := traitCatalog.apply(&environment)

	require.NoError(t, err)
	assert.Empty(t, conditions)
	assert.NotEmpty(t, environment.ExecutedTraits)
	assert.NotNil(t, environment.GetTrait("deployment"))
	assert.NotNil(t, environment.GetTrait("container"))

	d := environment.Resources.GetDeploymentForIntegration(environment.Integration)

	assert.NotNil(t, d)
	assert.Len(t, d.Spec.Template.Spec.Containers, 1)
	assert.Equal(t, defaultContainerName, d.Spec.Template.Spec.Containers[0].Name)
	assert.Equal(t, pointer.Bool(true), d.Spec.Template.Spec.Containers[0].SecurityContext.RunAsNonRoot)
	assert.Equal(t, pointer.Int64(1000860000), d.Spec.Template.Spec.Containers[0].SecurityContext.RunAsUser)
}

func TestContainerWithCustomName(t *testing.T) {
	catalog, err := camel.DefaultCatalog()
	require.NoError(t, err)

	client, _ := test.NewFakeClient()
	traitCatalog := NewCatalog(nil)
	environment := Environment{
		CamelCatalog: catalog,
		Catalog:      traitCatalog,
		Client:       client,
		Integration: &v1.Integration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ServiceTestName,
				Namespace: "ns",
			},
			Status: v1.IntegrationStatus{
				Phase: v1.IntegrationPhaseDeploying,
			},
			Spec: v1.IntegrationSpec{
				Profile: v1.TraitProfileKubernetes,
				Traits: v1.Traits{
					Container: &traitv1.ContainerTrait{
						Name: "my-container-name",
					},
				},
			},
		},
		IntegrationKit: &v1.IntegrationKit{
			Status: v1.IntegrationKitStatus{
				Phase: v1.IntegrationKitPhaseReady,
			},
		},
		Platform: &v1.IntegrationPlatform{
			Spec: v1.IntegrationPlatformSpec{
				Cluster: v1.IntegrationPlatformClusterOpenShift,
				Build: v1.IntegrationPlatformBuildSpec{
					PublishStrategy: v1.IntegrationPlatformBuildPublishStrategyS2I,
					Registry:        v1.RegistrySpec{Address: "registry"},
					RuntimeVersion:  catalog.Runtime.Version,
				},
			},
			Status: v1.IntegrationPlatformStatus{
				Phase: v1.IntegrationPlatformPhaseReady,
			},
		},
		EnvVars:        make([]corev1.EnvVar, 0),
		ExecutedTraits: make([]Trait, 0),
		Resources:      kubernetes.NewCollection(),
	}
	environment.Platform.ResyncStatusFullConfig()

	conditions, err := traitCatalog.apply(&environment)

	require.NoError(t, err)
	assert.Empty(t, conditions)
	assert.NotEmpty(t, environment.ExecutedTraits)
	assert.NotNil(t, environment.GetTrait("deployment"))
	assert.NotNil(t, environment.GetTrait("container"))

	d := environment.Resources.GetDeploymentForIntegration(environment.Integration)

	assert.NotNil(t, d)
	assert.Len(t, d.Spec.Template.Spec.Containers, 1)

	trait := environment.Integration.Spec.Traits.Container
	assert.Equal(t, trait.Name, d.Spec.Template.Spec.Containers[0].Name)
}

func TestContainerWithCustomImage(t *testing.T) {
	catalog, err := camel.DefaultCatalog()
	require.NoError(t, err)

	client, _ := test.NewFakeClient()
	traitCatalog := NewCatalog(nil)

	environment := Environment{
		Ctx:          context.TODO(),
		Client:       client,
		CamelCatalog: catalog,
		Catalog:      traitCatalog,
		Integration: &v1.Integration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ServiceTestName,
				Namespace: "ns",
				UID:       types.UID(uuid.NewString()),
			},
			Status: v1.IntegrationStatus{
				Phase: v1.IntegrationPhaseInitialization,
			},
			Spec: v1.IntegrationSpec{
				Profile: v1.TraitProfileKubernetes,
				Traits: v1.Traits{
					Container: &traitv1.ContainerTrait{
						Image: "foo/bar:1.0.0",
					},
				},
			},
		},
		Platform: &v1.IntegrationPlatform{
			Spec: v1.IntegrationPlatformSpec{
				Cluster: v1.IntegrationPlatformClusterOpenShift,
				Build: v1.IntegrationPlatformBuildSpec{
					PublishStrategy: v1.IntegrationPlatformBuildPublishStrategyS2I,
					Registry:        v1.RegistrySpec{Address: "registry"},
					RuntimeVersion:  catalog.Runtime.Version,
				},
			},
			Status: v1.IntegrationPlatformStatus{
				Phase: v1.IntegrationPlatformPhaseReady,
			},
		},
		EnvVars:        make([]corev1.EnvVar, 0),
		ExecutedTraits: make([]Trait, 0),
		Resources:      kubernetes.NewCollection(),
	}
	environment.Platform.ResyncStatusFullConfig()

	conditions, err := traitCatalog.apply(&environment)

	require.NoError(t, err)
	assert.Empty(t, conditions)

	for _, postAction := range environment.PostActions {
		require.NoError(t, postAction(&environment))
	}

	assert.NotEmpty(t, environment.ExecutedTraits)
	assert.NotNil(t, environment.GetTrait("deployer"))
	assert.NotNil(t, environment.GetTrait("container"))
	assert.Equal(t, "kit-"+ServiceTestName, environment.Integration.Status.IntegrationKit.Name)

	ikt := v1.IntegrationKit{}
	key := ctrl.ObjectKey{
		Namespace: "ns",
		Name:      "kit-" + ServiceTestName,
	}

	err = client.Get(context.TODO(), key, &ikt)
	require.NoError(t, err)
	assert.Equal(t, environment.Integration.ObjectMeta.UID, ikt.ObjectMeta.OwnerReferences[0].UID)

	trait := environment.Integration.Spec.Traits.Container
	assert.Equal(t, trait.Image, ikt.Spec.Image)
}

func TestContainerWithCustomImageAndIntegrationKit(t *testing.T) {
	catalog, err := camel.DefaultCatalog()
	require.NoError(t, err)

	client, _ := test.NewFakeClient()
	traitCatalog := NewCatalog(nil)

	environment := Environment{
		Ctx:          context.TODO(),
		Client:       client,
		CamelCatalog: catalog,
		Catalog:      traitCatalog,
		Integration: &v1.Integration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ServiceTestName,
				Namespace: "ns",
				UID:       types.UID(uuid.NewString()),
			},
			Status: v1.IntegrationStatus{
				Phase: v1.IntegrationPhaseInitialization,
			},
			Spec: v1.IntegrationSpec{
				Profile: v1.TraitProfileKubernetes,
				Traits: v1.Traits{
					Container: &traitv1.ContainerTrait{
						Image: "foo/bar:1.0.0",
					},
				},
				IntegrationKit: &corev1.ObjectReference{
					Name:      "bad-" + ServiceTestName,
					Namespace: "ns",
				},
			},
		},
		Platform: &v1.IntegrationPlatform{
			Spec: v1.IntegrationPlatformSpec{
				Cluster: v1.IntegrationPlatformClusterOpenShift,
				Build: v1.IntegrationPlatformBuildSpec{
					PublishStrategy: v1.IntegrationPlatformBuildPublishStrategyS2I,
					Registry:        v1.RegistrySpec{Address: "registry"},
					RuntimeVersion:  catalog.Runtime.Version,
				},
			},
			Status: v1.IntegrationPlatformStatus{
				Phase: v1.IntegrationPlatformPhaseReady,
			},
		},
		EnvVars:        make([]corev1.EnvVar, 0),
		ExecutedTraits: make([]Trait, 0),
		Resources:      kubernetes.NewCollection(),
	}
	environment.Platform.ResyncStatusFullConfig()

	conditions, err := traitCatalog.apply(&environment)
	require.Error(t, err)
	assert.Empty(t, conditions)
	assert.Contains(t, err.Error(), "unsupported configuration: a container image has been set in conjunction with an IntegrationKit")
}

func TestContainerWithImagePullPolicy(t *testing.T) {
	catalog, err := camel.DefaultCatalog()
	require.NoError(t, err)

	client, _ := test.NewFakeClient()
	traitCatalog := NewCatalog(nil)

	environment := Environment{
		Ctx:          context.TODO(),
		Client:       client,
		CamelCatalog: catalog,
		Catalog:      traitCatalog,
		Integration: &v1.Integration{
			Spec: v1.IntegrationSpec{
				Profile: v1.TraitProfileKubernetes,
				Traits: v1.Traits{
					Container: &traitv1.ContainerTrait{
						ImagePullPolicy: "Always",
					},
				},
			},
		},
		Platform: &v1.IntegrationPlatform{
			Spec: v1.IntegrationPlatformSpec{
				Build: v1.IntegrationPlatformBuildSpec{
					RuntimeVersion: catalog.Runtime.Version,
				},
			},
			Status: v1.IntegrationPlatformStatus{
				Phase: v1.IntegrationPlatformPhaseReady,
			},
		},
		Resources: kubernetes.NewCollection(),
	}
	environment.Integration.Status.Phase = v1.IntegrationPhaseDeploying
	environment.Platform.ResyncStatusFullConfig()

	conditions, err := traitCatalog.apply(&environment)

	require.NoError(t, err)
	assert.Empty(t, conditions)

	container := environment.GetIntegrationContainer()

	assert.Equal(t, corev1.PullAlways, container.ImagePullPolicy)
}

func TestRunKnativeEndpointWithKnativeNotInstalled(t *testing.T) {
	environment := createEnvironment()
	trait, _ := newContainerTrait().(*containerTrait)
	environment.Integration.Spec.Sources = []v1.SourceSpec{
		{
			DataSpec: v1.DataSpec{
				Name: "test.java",
				Content: `
				from("knative:channel/test").to("log:${body};
			`,
			},
			Language: v1.LanguageJavaSource,
		},
	}
	expectedCondition := NewIntegrationCondition(
		v1.IntegrationConditionKnativeAvailable,
		corev1.ConditionFalse,
		v1.IntegrationConditionKnativeNotInstalledReason,
		"integration cannot run, as knative is not installed in the cluster",
	)
	configured, condition, err := trait.Configure(environment)
	require.Error(t, err)
	assert.Equal(t, expectedCondition, condition)
	assert.False(t, configured)
}

func TestRunNonKnativeEndpointWithKnativeNotInstalled(t *testing.T) {

	environment := createEnvironment()
	trait, _ := newContainerTrait().(*containerTrait)
	environment.Integration.Spec.Sources = []v1.SourceSpec{
		{
			DataSpec: v1.DataSpec{
				Name: "test.java",
				Content: `
				from("platform-http://my-site").to("log:${body}");
			`,
			},
			Language: v1.LanguageJavaSource,
		},
	}

	configured, condition, err := trait.Configure(environment)
	require.NoError(t, err)
	assert.Nil(t, condition)
	assert.True(t, configured)
	conditions := environment.Integration.Status.Conditions
	assert.Empty(t, conditions)
}

func createEnvironment() *Environment {

	client, _ := test.NewFakeClient()
	// disable the knative eventing api
	fakeClient := client.(*test.FakeClient) //nolint
	fakeClient.DisableAPIGroupDiscovery("eventing.knative.dev/v1")

	replicas := int32(3)
	catalog, _ := camel.QuarkusCatalog()

	environment := &Environment{
		CamelCatalog: catalog,
		Catalog:      NewCatalog(nil),
		Client:       client,
		Integration: &v1.Integration{
			ObjectMeta: metav1.ObjectMeta{
				Name: "integration-name",
			},
			Spec: v1.IntegrationSpec{
				Replicas: &replicas,
				Traits:   v1.Traits{},
			},
			Status: v1.IntegrationStatus{
				Phase: v1.IntegrationPhaseInitialization,
			},
		},
		Platform: &v1.IntegrationPlatform{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "namespace",
			},
			Spec: v1.IntegrationPlatformSpec{
				Cluster: v1.IntegrationPlatformClusterKubernetes,
				Profile: v1.TraitProfileKubernetes,
			},
		},
		Resources:             kubernetes.NewCollection(),
		ApplicationProperties: make(map[string]string),
	}
	environment.Platform.ResyncStatusFullConfig()

	return environment
}
