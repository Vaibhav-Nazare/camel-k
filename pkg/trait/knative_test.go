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
	"fmt"
	"net/url"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	eventingduckv1 "knative.dev/eventing/pkg/apis/duck/v1"
	eventing "knative.dev/eventing/pkg/apis/eventing/v1"
	messaging "knative.dev/eventing/pkg/apis/messaging/v1"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	serving "knative.dev/serving/pkg/apis/serving/v1"

	v1 "github.com/apache/camel-k/v2/pkg/apis/camel/v1"
	knativeapi "github.com/apache/camel-k/v2/pkg/apis/camel/v1/knative"
	traitv1 "github.com/apache/camel-k/v2/pkg/apis/camel/v1/trait"
	"github.com/apache/camel-k/v2/pkg/client"
	"github.com/apache/camel-k/v2/pkg/util/camel"
	k8sutils "github.com/apache/camel-k/v2/pkg/util/kubernetes"
	"github.com/apache/camel-k/v2/pkg/util/test"
)

func TestKnativeEnvConfigurationFromTrait(t *testing.T) {
	catalog, err := camel.DefaultCatalog()
	require.NoError(t, err)

	traitCatalog := NewCatalog(nil)

	environment := Environment{
		CamelCatalog: catalog,
		Catalog:      traitCatalog,
		Integration: &v1.Integration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "ns",
			},
			Status: v1.IntegrationStatus{
				Phase: v1.IntegrationPhaseDeploying,
			},
			Spec: v1.IntegrationSpec{
				Profile: v1.TraitProfileKnative,
				Sources: []v1.SourceSpec{},
				Traits: v1.Traits{
					Knative: &traitv1.KnativeTrait{
						Trait: traitv1.Trait{
							Enabled: pointer.Bool(true),
						},
						Auto:            pointer.Bool(false),
						ChannelSources:  []string{"channel-source-1"},
						ChannelSinks:    []string{"channel-sink-1"},
						EndpointSources: []string{"endpoint-source-1"},
						EndpointSinks:   []string{"endpoint-sink-1", "endpoint-sink-2"},
						EventSources:    []string{"knative:event"},
						EventSinks:      []string{"knative:event"},
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
				},
				Profile: v1.TraitProfileKnative,
			},
		},
		EnvVars:        make([]corev1.EnvVar, 0),
		ExecutedTraits: make([]Trait, 0),
		Resources:      k8sutils.NewCollection(),
	}
	environment.Platform.ResyncStatusFullConfig()

	c, err := NewFakeClient("ns")
	require.NoError(t, err)

	tc := NewCatalog(c)

	err = tc.Configure(&environment)
	require.NoError(t, err)

	tr, _ := tc.GetTrait("knative").(*knativeTrait)
	ok, condition, err := tr.Configure(&environment)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Nil(t, condition)

	err = tr.Apply(&environment)
	require.NoError(t, err)

	ne, err := fromCamelProperties(environment.ApplicationProperties)
	require.NoError(t, err)

	cSource1 := ne.FindService("channel-source-1", knativeapi.CamelEndpointKindSource, knativeapi.CamelServiceTypeChannel, "messaging.knative.dev/v1", "Channel")
	assert.NotNil(t, cSource1)
	assert.Empty(t, cSource1.URL)

	cSink1 := ne.FindService("channel-sink-1", knativeapi.CamelEndpointKindSink, knativeapi.CamelServiceTypeChannel, "messaging.knative.dev/v1", "Channel")
	assert.NotNil(t, cSink1)
	assert.Equal(t, "http://channel-sink-1.host/", cSink1.URL)

	eSource1 := ne.FindService("endpoint-source-1", knativeapi.CamelEndpointKindSource, knativeapi.CamelServiceTypeEndpoint, "serving.knative.dev/v1", "Service")
	assert.NotNil(t, eSource1)
	assert.Empty(t, eSource1.URL)

	eSink1 := ne.FindService("endpoint-sink-1", knativeapi.CamelEndpointKindSink, knativeapi.CamelServiceTypeEndpoint, "serving.knative.dev/v1", "Service")
	assert.NotNil(t, eSink1)
	assert.Equal(t, "http://endpoint-sink-1.host/", eSink1.URL)
	eSink2 := ne.FindService("endpoint-sink-2", knativeapi.CamelEndpointKindSink, knativeapi.CamelServiceTypeEndpoint, "serving.knative.dev/v1", "Service")
	assert.NotNil(t, eSink2)
	assert.Equal(t, "http://endpoint-sink-2.host/", eSink2.URL)

	eEventSource := ne.FindService("default", knativeapi.CamelEndpointKindSource, knativeapi.CamelServiceTypeEvent, "eventing.knative.dev/v1", "Broker")
	assert.NotNil(t, eEventSource)
	eEventSink := ne.FindService("default", knativeapi.CamelEndpointKindSink, knativeapi.CamelServiceTypeEvent, "eventing.knative.dev/v1", "Broker")
	assert.NotNil(t, eEventSink)
	assert.Equal(t, "http://broker-default.host/", eEventSink.URL)
}

func TestKnativeEnvConfigurationFromSource(t *testing.T) {
	catalog, err := camel.DefaultCatalog()
	require.NoError(t, err)

	traitCatalog := NewCatalog(nil)

	environment := Environment{
		CamelCatalog: catalog,
		Catalog:      traitCatalog,
		Integration: &v1.Integration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
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
							Name: "route.java",
							Content: `
								public class CartoonMessagesMover extends RouteBuilder {
									public void configure() {
										from("knative:endpoint/s3fileMover1")
											.log("${body}");

										from("knative:channel/channel-source-1")
 											.log("${body}");

										from("knative:event/evt.type")
 											.log("${body}");
									}
								}
							`,
						},
						Language: v1.LanguageJavaSource,
					},
				},
				Traits: v1.Traits{
					Knative: &traitv1.KnativeTrait{
						Trait: traitv1.Trait{
							Enabled: pointer.Bool(true),
						},
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
				},
				Profile: v1.TraitProfileKnative,
			},
		},
		EnvVars:        make([]corev1.EnvVar, 0),
		ExecutedTraits: make([]Trait, 0),
		Resources:      k8sutils.NewCollection(),
	}
	environment.Platform.ResyncStatusFullConfig()

	c, err := NewFakeClient("ns")
	require.NoError(t, err)

	tc := NewCatalog(c)

	err = tc.Configure(&environment)
	require.NoError(t, err)

	tr, _ := tc.GetTrait("knative").(*knativeTrait)

	ok, condition, err := tr.Configure(&environment)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Nil(t, condition)

	err = tr.Apply(&environment)
	require.NoError(t, err)

	ne, err := fromCamelProperties(environment.ApplicationProperties)
	require.NoError(t, err)

	source := ne.FindService("s3fileMover1", knativeapi.CamelEndpointKindSource, knativeapi.CamelServiceTypeEndpoint, "serving.knative.dev/v1", "Service")
	assert.NotNil(t, source)
	assert.Empty(t, source.URL)
	assert.Empty(t, source.Metadata[knativeapi.CamelMetaKnativeReply])

	channel := ne.FindService("channel-source-1", knativeapi.CamelEndpointKindSource, knativeapi.CamelServiceTypeChannel, "", "")
	assert.NotNil(t, channel)
	assert.Equal(t, "false", channel.Metadata[knativeapi.CamelMetaKnativeReply])

	broker := ne.FindService("evt.type", knativeapi.CamelEndpointKindSource, knativeapi.CamelServiceTypeEvent, "", "")
	assert.NotNil(t, broker)
	assert.Equal(t, "false", broker.Metadata[knativeapi.CamelMetaKnativeReply])
}

func TestKnativePlatformHttpConfig(t *testing.T) {
	sources := []v1.SourceSpec{
		{
			DataSpec: v1.DataSpec{
				Name:    "source-endpoint.groovy",
				Content: `from('knative:endpoint/ep').log('${body}')`,
			},
			Language: v1.LanguageGroovy,
		},
		{
			DataSpec: v1.DataSpec{
				Name:    "source-channel.groovy",
				Content: `from('knative:channel/channel-source-1').log('${body}')`,
			},
			Language: v1.LanguageGroovy,
		},
		{
			DataSpec: v1.DataSpec{
				Name:    "source-event.groovy",
				Content: `from('knative:event/event-source-1').log('${body}')`,
			},
			Language: v1.LanguageGroovy,
		},
	}

	for _, ref := range sources {
		source := ref
		t.Run(source.Name, func(t *testing.T) {
			environment := NewFakeEnvironment(t, source)

			c, err := NewFakeClient("ns")
			require.NoError(t, err)

			tc := NewCatalog(c)

			err = tc.Configure(&environment)
			require.NoError(t, err)

			_, err = tc.apply(&environment)
			require.NoError(t, err)
			assert.Contains(t, environment.Integration.Status.Capabilities, v1.CapabilityPlatformHTTP)
		})
	}
}

func TestKnativePlatformHttpDependencies(t *testing.T) {
	sources := []v1.SourceSpec{
		{
			DataSpec: v1.DataSpec{
				Name:    "source-endpoint.groovy",
				Content: `from('knative:endpoint/ep').log('${body}')`,
			},
			Language: v1.LanguageGroovy,
		},
		{
			DataSpec: v1.DataSpec{
				Name:    "source-channel.groovy",
				Content: `from('knative:channel/channel-source-1').log('${body}')`,
			},
			Language: v1.LanguageGroovy,
		},
		{
			DataSpec: v1.DataSpec{
				Name:    "source-event.groovy",
				Content: `from('knative:event/event-source-1').log('${body}')`,
			},
			Language: v1.LanguageGroovy,
		},
	}

	for _, ref := range sources {
		source := ref
		t.Run(source.Name, func(t *testing.T) {
			environment := NewFakeEnvironment(t, source)
			environment.Integration.Status.Phase = v1.IntegrationPhaseInitialization

			c, err := NewFakeClient("ns")
			require.NoError(t, err)

			tc := NewCatalog(c)

			err = tc.Configure(&environment)
			require.NoError(t, err)

			conditions, err := tc.apply(&environment)
			require.NoError(t, err)
			assert.Empty(t, conditions)
			assert.Contains(t, environment.Integration.Status.Capabilities, v1.CapabilityPlatformHTTP)
			assert.Contains(t, environment.Integration.Status.Dependencies, "mvn:org.apache.camel.quarkus:camel-quarkus-platform-http")
		})
	}
}

func NewFakeEnvironment(t *testing.T, source v1.SourceSpec) Environment {
	t.Helper()

	client, _ := NewFakeClient("ns")
	catalog, err := camel.DefaultCatalog()
	require.NoError(t, err)

	traitCatalog := NewCatalog(nil)

	environment := Environment{
		CamelCatalog: catalog,
		Catalog:      traitCatalog,
		Client:       client,
		Integration: &v1.Integration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "ns",
			},
			Status: v1.IntegrationStatus{
				Phase: v1.IntegrationPhaseDeploying,
			},
			Spec: v1.IntegrationSpec{
				Profile: v1.TraitProfileKnative,
				Sources: []v1.SourceSpec{
					source,
				},
				Traits: v1.Traits{
					Knative: &traitv1.KnativeTrait{
						Trait: traitv1.Trait{
							Enabled: pointer.Bool(true),
						},
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
				Profile: v1.TraitProfileKnative,
			},
			Status: v1.IntegrationPlatformStatus{
				Phase: v1.IntegrationPlatformPhaseReady,
			},
		},
		EnvVars:        make([]corev1.EnvVar, 0),
		ExecutedTraits: make([]Trait, 0),
		Resources:      k8sutils.NewCollection(),
	}
	environment.Platform.ResyncStatusFullConfig()

	return environment
}

func NewFakeClient(namespace string) (client.Client, error) {
	channelSourceURL, err := apis.ParseURL("http://channel-source-1.host/")
	if err != nil {
		return nil, err
	}
	channelSinkURL, err := apis.ParseURL("http://channel-sink-1.host/")
	if err != nil {
		return nil, err
	}
	sink1URL, err := apis.ParseURL("http://endpoint-sink-1.host/")
	if err != nil {
		return nil, err
	}
	sink2URL, err := apis.ParseURL("http://endpoint-sink-2.host/")
	if err != nil {
		return nil, err
	}
	brokerURL, err := apis.ParseURL("http://broker-default.host/")
	if err != nil {
		return nil, err
	}

	return test.NewFakeClient(
		&messaging.Channel{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Channel",
				APIVersion: messaging.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "channel-source-1",
			},
			Status: messaging.ChannelStatus{
				ChannelableStatus: eventingduckv1.ChannelableStatus{
					AddressStatus: duckv1.AddressStatus{
						Address: &duckv1.Addressable{
							URL: channelSourceURL,
						},
					},
				},
			},
		},
		&messaging.Channel{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Channel",
				APIVersion: messaging.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "channel-sink-1",
			},
			Status: messaging.ChannelStatus{
				ChannelableStatus: eventingduckv1.ChannelableStatus{
					AddressStatus: duckv1.AddressStatus{
						Address: &duckv1.Addressable{
							URL: channelSinkURL,
						},
					},
				},
			},
		},
		&serving.Service{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Service",
				APIVersion: serving.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "endpoint-sink-1",
			},
			Status: serving.ServiceStatus{
				RouteStatusFields: serving.RouteStatusFields{
					Address: &duckv1.Addressable{
						URL: sink1URL,
					},
				},
			},
		},
		&serving.Service{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Service",
				APIVersion: serving.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "endpoint-sink-2",
			},
			Status: serving.ServiceStatus{
				RouteStatusFields: serving.RouteStatusFields{
					Address: &duckv1.Addressable{
						URL: sink2URL,
					},
				},
			},
		},
		&eventing.Broker{
			TypeMeta: metav1.TypeMeta{
				APIVersion: eventing.SchemeGroupVersion.String(),
				Kind:       "Broker",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "default",
			},
			Spec: eventing.BrokerSpec{},
			Status: eventing.BrokerStatus{
				AddressStatus: duckv1.AddressStatus{
					Address: &duckv1.Addressable{
						URL: brokerURL,
					},
				},
			},
		},
		&eventing.Trigger{
			TypeMeta: metav1.TypeMeta{
				APIVersion: eventing.SchemeGroupVersion.String(),
				Kind:       "Trigger",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "event-source-1",
			},
			Spec: eventing.TriggerSpec{
				Filter: &eventing.TriggerFilter{
					Attributes: eventing.TriggerFilterAttributes{
						"type": "event-source-1",
					},
				},
				Broker: "default",
				Subscriber: duckv1.Destination{
					Ref: &duckv1.KReference{
						APIVersion: serving.SchemeGroupVersion.String(),
						Kind:       "Service",
						Name:       "event-source-1",
					},
				},
			},
		},
	)
}

func SortChannelFakeClient(namespace string) (client.Client, error) {
	channelSourceURL1, err := apis.ParseURL("http://channel-source-1.host/")
	if err != nil {
		return nil, err
	}
	channelSourceURL2, err := apis.ParseURL("http://channel-source-2.host/")
	if err != nil {
		return nil, err
	}
	channelSourceURL3, err := apis.ParseURL("http://channel-source-3.host/")
	if err != nil {
		return nil, err
	}
	channelSourceURL4, err := apis.ParseURL("http://channel-source-4.host/")
	if err != nil {
		return nil, err
	}

	return test.NewFakeClient(
		// Channels unsorted on purpose
		&messaging.Channel{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Channel",
				APIVersion: messaging.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "channel-source-2",
			},
			Status: messaging.ChannelStatus{
				ChannelableStatus: eventingduckv1.ChannelableStatus{
					AddressStatus: duckv1.AddressStatus{
						Address: &duckv1.Addressable{
							URL: channelSourceURL2,
						},
					},
				},
			},
		},
		&messaging.Channel{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Channel",
				APIVersion: messaging.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "channel-source-4",
			},
			Status: messaging.ChannelStatus{
				ChannelableStatus: eventingduckv1.ChannelableStatus{
					AddressStatus: duckv1.AddressStatus{
						Address: &duckv1.Addressable{
							URL: channelSourceURL4,
						},
					},
				},
			},
		},
		&messaging.Channel{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Channel",
				APIVersion: messaging.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "channel-source-1",
			},
			Status: messaging.ChannelStatus{
				ChannelableStatus: eventingduckv1.ChannelableStatus{
					AddressStatus: duckv1.AddressStatus{
						Address: &duckv1.Addressable{
							URL: channelSourceURL1,
						},
					},
				},
			},
		},
		&messaging.Channel{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Channel",
				APIVersion: messaging.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "channel-source-3",
			},
			Status: messaging.ChannelStatus{
				ChannelableStatus: eventingduckv1.ChannelableStatus{
					AddressStatus: duckv1.AddressStatus{
						Address: &duckv1.Addressable{
							URL: channelSourceURL3,
						},
					},
				},
			},
		},
	)
}

func TestKnativeSinkBinding(t *testing.T) {
	source := v1.SourceSpec{
		DataSpec: v1.DataSpec{
			Name:    "sink.groovy",
			Content: `from('timer:foo').to('knative:channel/channel-sink-1?apiVersion=messaging.knative.dev%2Fv1&kind=Channel')`,
		},
		Language: v1.LanguageGroovy,
	}

	environment := NewFakeEnvironment(t, source)
	environment.Integration.Status.Phase = v1.IntegrationPhaseDeploying

	c, err := NewFakeClient("ns")
	require.NoError(t, err)

	tc := NewCatalog(c)

	err = tc.Configure(&environment)
	require.NoError(t, err)

	_, err = tc.apply(&environment)
	require.NoError(t, err)
	baseProp := "camel.component.knative.environment.resources[0]"
	assert.Equal(t, "channel-sink-1", environment.ApplicationProperties[baseProp+".name"])
	assert.Equal(t, "${K_SINK}", environment.ApplicationProperties[baseProp+".url"])
	assert.Equal(t, "${K_CE_OVERRIDES}", environment.ApplicationProperties[baseProp+".ceOverrides"])
	assert.Equal(t, "channel", environment.ApplicationProperties[baseProp+".type"])
	assert.Equal(t, "Channel", environment.ApplicationProperties[baseProp+".objectKind"])
	assert.Equal(t, "messaging.knative.dev/v1", environment.ApplicationProperties[baseProp+".objectApiVersion"])
	assert.Equal(t, "sink", environment.ApplicationProperties[baseProp+".endpointKind"])
}

// fromCamelProperties deserialize from properties to environment.
func fromCamelProperties(appProps map[string]string) (*knativeapi.CamelEnvironment, error) {
	env := knativeapi.NewCamelEnvironment()
	re := regexp.MustCompile(`camel.component.knative.environment.resources\[(\d+)\].name`)
	for k, v := range appProps {
		if re.MatchString(k) {
			index := re.FindStringSubmatch(k)[1]
			baseComp := fmt.Sprintf("camel.component.knative.environment.resources[%s]", index)
			url, err := url.Parse(appProps[fmt.Sprintf("%s.url", baseComp)])
			if err != nil {
				return nil, err
			}
			svc, err := knativeapi.BuildCamelServiceDefinition(
				v,
				knativeapi.CamelEndpointKind(appProps[fmt.Sprintf("%s.endpointKind", baseComp)]),
				knativeapi.CamelServiceType(appProps[fmt.Sprintf("%s.type", baseComp)]),
				*url,
				appProps[fmt.Sprintf("%s.objectApiVersion", baseComp)],
				appProps[fmt.Sprintf("%s.objectKind", baseComp)],
			)
			if err != nil {
				return nil, err
			}
			svc.Metadata[knativeapi.CamelMetaKnativeReply] = appProps[fmt.Sprintf("%s.reply", baseComp)]
			env.Services = append(env.Services, svc)
		}
	}

	return &env, nil
}
