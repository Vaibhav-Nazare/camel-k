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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	v1 "github.com/apache/camel-k/v2/pkg/apis/camel/v1"
	traitv1 "github.com/apache/camel-k/v2/pkg/apis/camel/v1/trait"
	"github.com/apache/camel-k/v2/pkg/util/camel"
	"github.com/apache/camel-k/v2/pkg/util/gzip"
	"github.com/apache/camel-k/v2/pkg/util/kubernetes"
	"github.com/apache/camel-k/v2/pkg/util/test"
)

const (
	ServiceTestNamespace = "ns"
	ServiceTestName      = "test"
)

func TestServiceWithDefaults(t *testing.T) {
	catalog, err := camel.DefaultCatalog()
	require.NoError(t, err)

	client, _ := test.NewFakeClient()
	traitCatalog := NewCatalog(nil)

	compressedRoute, err := gzip.CompressBase64([]byte(`from("netty-http:test").log("hello")`))
	require.NoError(t, err)

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
				Sources: []v1.SourceSpec{
					{
						DataSpec: v1.DataSpec{
							Name:        "routes.js",
							Content:     string(compressedRoute),
							Compression: true,
						},
						Language: v1.LanguageJavaScript,
					},
				},
				Traits: v1.Traits{
					Service: &traitv1.ServiceTrait{
						Trait: traitv1.Trait{
							Enabled: pointer.Bool(true),
						},
						Auto: pointer.Bool(false),
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
	assert.NotNil(t, environment.GetTrait("service"))
	assert.NotNil(t, environment.GetTrait("container"))

	s := environment.Resources.GetService(func(service *corev1.Service) bool {
		return service.Name == ServiceTestName
	})
	d := environment.Resources.GetDeployment(func(deployment *appsv1.Deployment) bool {
		return deployment.Name == ServiceTestName
	})

	assert.NotNil(t, d)
	assert.NotNil(t, s)

	assert.Len(t, s.Spec.Ports, 1)
	assert.Equal(t, int32(80), s.Spec.Ports[0].Port)
	assert.Equal(t, "http", s.Spec.Ports[0].Name)
	assert.Equal(t, "http", s.Spec.Ports[0].TargetPort.String())

	assert.Len(t, d.Spec.Template.Spec.Containers, 1)
	assert.Len(t, d.Spec.Template.Spec.Containers[0].Ports, 1)
	assert.Equal(t, int32(8080), d.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort)
	assert.Equal(t, "http", d.Spec.Template.Spec.Containers[0].Ports[0].Name)

	assert.Empty(t, s.Spec.Type) // empty means ClusterIP
}

func TestService(t *testing.T) {
	catalog, err := camel.DefaultCatalog()
	require.NoError(t, err)

	client, _ := test.NewFakeClient()
	traitCatalog := NewCatalog(nil)

	compressedRoute, err := gzip.CompressBase64([]byte(`from("netty-http:test").log("hello")`))
	require.NoError(t, err)

	environment := Environment{
		CamelCatalog: catalog,
		Catalog:      traitCatalog,
		Client:       client,
		Integration: &v1.Integration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ServiceTestName,
				Namespace: ServiceTestNamespace,
			},
			Status: v1.IntegrationStatus{
				Phase: v1.IntegrationPhaseDeploying,
			},
			Spec: v1.IntegrationSpec{
				Profile: v1.TraitProfileKubernetes,
				Sources: []v1.SourceSpec{
					{
						DataSpec: v1.DataSpec{
							Name:        "routes.js",
							Content:     string(compressedRoute),
							Compression: true,
						},
						Language: v1.LanguageJavaScript,
					},
				},
				Traits: v1.Traits{
					Service: &traitv1.ServiceTrait{
						Trait: traitv1.Trait{
							Enabled: pointer.Bool(true),
						},
					},
					Container: &traitv1.ContainerTrait{
						PlatformBaseTrait: traitv1.PlatformBaseTrait{},
						Auto:              pointer.Bool(false),
						Expose:            pointer.Bool(true),
						Port:              8081,
						PortName:          "http-8081",
						ServicePort:       81,
						ServicePortName:   "http-81",
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
	assert.NotNil(t, environment.GetTrait("service"))
	assert.NotNil(t, environment.GetTrait("container"))

	s := environment.Resources.GetService(func(service *corev1.Service) bool {
		return service.Name == ServiceTestName
	})
	d := environment.Resources.GetDeployment(func(deployment *appsv1.Deployment) bool {
		return deployment.Name == ServiceTestName
	})

	assert.NotNil(t, d)
	assert.NotNil(t, s)

	assert.Len(t, s.Spec.Ports, 1)
	assert.Equal(t, int32(81), s.Spec.Ports[0].Port)
	assert.Equal(t, "http-81", s.Spec.Ports[0].Name)
	assert.Equal(t, "http-8081", s.Spec.Ports[0].TargetPort.String())

	assert.Len(t, d.Spec.Template.Spec.Containers, 1)
	assert.Len(t, d.Spec.Template.Spec.Containers[0].Ports, 1)
	assert.Equal(t, int32(8081), d.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort)
	assert.Equal(t, "http-8081", d.Spec.Template.Spec.Containers[0].Ports[0].Name)
}

func TestServiceWithCustomContainerName(t *testing.T) {
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
				Namespace: ServiceTestNamespace,
			},
			Status: v1.IntegrationStatus{
				Phase: v1.IntegrationPhaseDeploying,
			},
			Spec: v1.IntegrationSpec{
				Profile: v1.TraitProfileKubernetes,
				Traits: v1.Traits{
					Service: &traitv1.ServiceTrait{
						Trait: traitv1.Trait{
							Enabled: pointer.Bool(true),
						},
						Auto: pointer.Bool(false),
					},
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
	assert.NotNil(t, environment.GetTrait("service"))
	assert.NotNil(t, environment.GetTrait("container"))

	d := environment.Resources.GetDeploymentForIntegration(environment.Integration)
	assert.NotNil(t, d)

	assert.Len(t, d.Spec.Template.Spec.Containers, 1)

	trait := environment.Integration.Spec.Traits.Container
	assert.Equal(
		t,
		trait.Name,
		d.Spec.Template.Spec.Containers[0].Name,
	)
}

func TestServiceWithNodePort(t *testing.T) {
	catalog, err := camel.DefaultCatalog()
	require.NoError(t, err)

	client, _ := test.NewFakeClient()
	traitCatalog := NewCatalog(nil)

	compressedRoute, err := gzip.CompressBase64([]byte(`from("netty-http:test").log("hello")`))
	require.NoError(t, err)

	serviceType := traitv1.ServiceTypeNodePort
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
				Sources: []v1.SourceSpec{
					{
						DataSpec: v1.DataSpec{
							Name:        "routes.js",
							Content:     string(compressedRoute),
							Compression: true,
						},
						Language: v1.LanguageJavaScript,
					},
				},
				Traits: v1.Traits{
					Service: &traitv1.ServiceTrait{
						Trait: traitv1.Trait{
							Enabled: pointer.Bool(true),
						},
						Auto: pointer.Bool(false),
						Type: &serviceType,
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
	assert.NotNil(t, environment.GetTrait("service"))
	assert.NotNil(t, environment.GetTrait("container"))

	s := environment.Resources.GetService(func(service *corev1.Service) bool {
		return service.Name == ServiceTestName
	})
	d := environment.Resources.GetDeployment(func(deployment *appsv1.Deployment) bool {
		return deployment.Name == ServiceTestName
	})

	assert.NotNil(t, d)
	assert.NotNil(t, s)

	assert.Len(t, s.Spec.Ports, 1)
	assert.Equal(t, int32(80), s.Spec.Ports[0].Port)

	assert.Equal(t, corev1.ServiceTypeNodePort, s.Spec.Type)
}

// When service and knative-service are enabled at the integration scope in knative profile
// knative-service has the priority and the k8s service is not run.
func TestServiceWithKnativeServiceEnabled(t *testing.T) {
	catalog, err := camel.DefaultCatalog()
	require.NoError(t, err)

	client, _ := test.NewFakeClient()
	traitCatalog := NewCatalog(nil)

	compressedRoute, err := gzip.CompressBase64([]byte(`from("netty-http:test").log("hello")`))
	require.NoError(t, err)

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
				Profile: v1.TraitProfileKnative,
				Sources: []v1.SourceSpec{
					{
						DataSpec: v1.DataSpec{
							Name:        "routes.js",
							Content:     string(compressedRoute),
							Compression: true,
						},
						Language: v1.LanguageJavaScript,
					},
				},
				Traits: v1.Traits{
					Service: &traitv1.ServiceTrait{
						Trait: traitv1.Trait{
							Enabled: pointer.Bool(true),
						},
						Auto: pointer.Bool(false),
					},
					KnativeService: &traitv1.KnativeServiceTrait{
						Trait: traitv1.Trait{
							Enabled: pointer.Bool(true),
						},
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

	deploymentCondition := NewIntegrationCondition(
		v1.IntegrationConditionDeploymentAvailable,
		corev1.ConditionFalse,
		"deploymentTraitConfiguration",
		"controller strategy: knative-service",
	)
	serviceCondition := NewIntegrationCondition(
		v1.IntegrationConditionTraitInfo,
		corev1.ConditionTrue,
		"serviceTraitConfiguration",
		"explicitly disabled by the platform: knative-service trait has priority over this trait",
	)
	conditions, err := traitCatalog.apply(&environment)

	require.NoError(t, err)
	assert.Len(t, conditions, 2)
	assert.Contains(t, conditions, deploymentCondition)
	assert.Contains(t, conditions, serviceCondition)
	assert.NotEmpty(t, environment.ExecutedTraits)
	assert.Nil(t, environment.GetTrait(serviceTraitID))
	assert.NotNil(t, environment.GetTrait(knativeServiceTraitID))
}

func TestServicesWithKnativeProfile(t *testing.T) {
	catalog, err := camel.DefaultCatalog()
	require.NoError(t, err)

	client, _ := test.NewFakeClient()
	traitCatalog := NewCatalog(nil)

	compressedRoute, err := gzip.CompressBase64([]byte(`from("netty-http:test").log("hello")`))
	require.NoError(t, err)

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
				Profile: v1.TraitProfileKnative,
				Sources: []v1.SourceSpec{
					{
						DataSpec: v1.DataSpec{
							Name:        "routes.js",
							Content:     string(compressedRoute),
							Compression: true,
						},
						Language: v1.LanguageJavaScript,
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

	deploymentCondition := NewIntegrationCondition(
		v1.IntegrationConditionDeploymentAvailable,
		corev1.ConditionFalse,
		"deploymentTraitConfiguration",
		"controller strategy: knative-service",
	)
	serviceCondition := NewIntegrationCondition(
		v1.IntegrationConditionTraitInfo,
		corev1.ConditionTrue,
		"serviceTraitConfiguration",
		"explicitly disabled by the platform: knative-service trait has priority over this trait",
	)
	conditions, err := traitCatalog.apply(&environment)

	require.NoError(t, err)
	assert.Len(t, conditions, 2)
	assert.Contains(t, conditions, deploymentCondition)
	assert.Contains(t, conditions, serviceCondition)
	assert.NotEmpty(t, environment.ExecutedTraits)
	assert.Nil(t, environment.GetTrait(serviceTraitID))
	assert.NotNil(t, environment.GetTrait(knativeServiceTraitID))
}

// When the knative-service is disabled at the IntegrationPlatform, the k8s service is enabled.
func TestServiceWithKnativeServiceDisabledInIntegrationPlatform(t *testing.T) {
	catalog, err := camel.DefaultCatalog()
	require.NoError(t, err)

	client, _ := test.NewFakeClient()
	traitCatalog := NewCatalog(nil)

	compressedRoute, err := gzip.CompressBase64([]byte(`from("netty-http:test").log("hello")`))
	require.NoError(t, err)

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
				Profile: v1.TraitProfileKnative,
				Sources: []v1.SourceSpec{
					{
						DataSpec: v1.DataSpec{
							Name:        "routes.js",
							Content:     string(compressedRoute),
							Compression: true,
						},
						Language: v1.LanguageJavaScript,
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
				Traits: v1.Traits{
					KnativeService: &traitv1.KnativeServiceTrait{
						Trait: traitv1.Trait{
							Enabled: pointer.Bool(false),
						},
					},
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

	expectedCondition := NewIntegrationCondition(
		v1.IntegrationConditionKnativeServiceAvailable,
		corev1.ConditionFalse,
		"knative-serviceTraitConfiguration",
		"explicitly disabled",
	)
	conditions, err := traitCatalog.apply(&environment)

	require.NoError(t, err)
	assert.Len(t, conditions, 1)
	assert.Contains(t, conditions, expectedCondition)
	assert.NotEmpty(t, environment.ExecutedTraits)
	assert.NotNil(t, environment.GetTrait(serviceTraitID))
	assert.Nil(t, environment.GetTrait(knativeServiceTraitID))
}
