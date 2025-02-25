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

package pipe

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/apache/camel-k/v2/pkg/apis/camel/v1"

	"github.com/apache/camel-k/v2/pkg/client"
	"github.com/apache/camel-k/v2/pkg/platform"
	"github.com/apache/camel-k/v2/pkg/util"
	"github.com/apache/camel-k/v2/pkg/util/bindings"
	"github.com/apache/camel-k/v2/pkg/util/knative"
	"github.com/apache/camel-k/v2/pkg/util/kubernetes"
	"github.com/apache/camel-k/v2/pkg/util/property"
)

var (
	endpointTypeSourceContext = bindings.EndpointContext{Type: v1.EndpointTypeSource}
	endpointTypeSinkContext   = bindings.EndpointContext{Type: v1.EndpointTypeSink}
)

// CreateIntegrationFor creates and Integration from a Pipe.
func CreateIntegrationFor(ctx context.Context, c client.Client, binding *v1.Pipe) (*v1.Integration, error) {
	controller := true
	blockOwnerDeletion := true
	annotations := util.CopyMap(binding.Annotations)
	// avoid propagating the icon to the integration as it's heavyweight and not needed
	delete(annotations, v1.AnnotationIcon)

	it := v1.Integration{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   binding.Namespace,
			Name:        binding.Name,
			Annotations: annotations,
			Labels:      util.CopyMap(binding.Labels),
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         binding.APIVersion,
					Kind:               binding.Kind,
					Name:               binding.Name,
					UID:                binding.UID,
					Controller:         &controller,
					BlockOwnerDeletion: &blockOwnerDeletion,
				},
			},
		},
	}

	// creator labels
	if it.GetLabels() == nil {
		it.SetLabels(make(map[string]string))
	}
	it.GetLabels()[kubernetes.CamelCreatorLabelKind] = binding.Kind
	it.GetLabels()[kubernetes.CamelCreatorLabelName] = binding.Name

	// Deprecated
	// start from the integration spec defined in the binding
	if binding.Spec.Integration != nil {
		it.Spec = *binding.Spec.Integration.DeepCopy()
	}

	// Set replicas (or override podspecable value) if present
	if binding.Spec.Replicas != nil {
		replicas := *binding.Spec.Replicas
		it.Spec.Replicas = &replicas
	}

	profile, err := determineTraitProfile(ctx, c, binding)
	if err != nil {
		return nil, err
	}
	it.Spec.Profile = profile

	if binding.Spec.ServiceAccountName != "" {
		it.Spec.ServiceAccountName = binding.Spec.ServiceAccountName
	}

	bindingContext := bindings.BindingContext{
		Ctx:       ctx,
		Client:    c,
		Namespace: it.Namespace,
		Profile:   profile,
		Metadata:  it.Annotations,
	}

	from, err := bindings.Translate(bindingContext, endpointTypeSourceContext, binding.Spec.Source)
	if err != nil {
		return nil, fmt.Errorf("could not determine source URI: %w", err)
	}
	to, err := bindings.Translate(bindingContext, endpointTypeSinkContext, binding.Spec.Sink)
	if err != nil {
		return nil, fmt.Errorf("could not determine sink URI: %w", err)
	}
	// error handler is optional
	errorHandler, err := maybeErrorHandler(binding.Spec.ErrorHandler, bindingContext)
	if err != nil {
		return nil, fmt.Errorf("could not determine error handler: %w", err)
	}

	steps := make([]*bindings.Binding, 0, len(binding.Spec.Steps))
	for idx, step := range binding.Spec.Steps {
		position := idx
		stepBinding, err := bindings.Translate(bindingContext, bindings.EndpointContext{
			Type:     v1.EndpointTypeAction,
			Position: &position,
		}, step)
		if err != nil {
			return nil, fmt.Errorf("could not determine URI for step %d: %w", idx, err)
		}
		steps = append(steps, stepBinding)
	}

	if to.Step == nil && to.URI == "" {
		return nil, fmt.Errorf("illegal step definition for sink step: either Step or URI should be provided")
	}
	if from.URI == "" {
		return nil, fmt.Errorf("illegal step definition for source step: URI should be provided")
	}
	for index, step := range steps {
		if step.Step == nil && step.URI == "" {
			return nil, fmt.Errorf("illegal step definition for step %d: either Step or URI should be provided", index)
		}
	}

	if err := configureBinding(&it, from); err != nil {
		return nil, err
	}

	if err := configureBinding(&it, steps...); err != nil {
		return nil, err
	}

	if err := configureBinding(&it, to); err != nil {
		return nil, err
	}

	if err := configureBinding(&it, errorHandler); err != nil {
		return nil, err
	}

	if it.Spec.Configuration != nil {
		sort.SliceStable(it.Spec.Configuration, func(i, j int) bool {
			mi, mj := it.Spec.Configuration[i], it.Spec.Configuration[j]
			switch {
			case mi.Type != mj.Type:
				return mi.Type < mj.Type
			default:
				return mi.Value < mj.Value
			}
		})
	}

	dslSteps := make([]map[string]interface{}, 0)

	if from.Step != nil {
		dslSteps = append(dslSteps, from.AsYamlDSL())
	}

	for _, step := range steps {
		dslSteps = append(dslSteps, step.AsYamlDSL())
	}

	if to.Step != nil {
		dslSteps = append(dslSteps, to.AsYamlDSL())
	}
	dslSteps = append(dslSteps, map[string]interface{}{
		"to": to.URI,
	})

	fromWrapper := map[string]interface{}{
		"uri":   from.URI,
		"steps": dslSteps,
	}

	flowRoute := map[string]interface{}{
		"route": map[string]interface{}{
			"id":   "binding",
			"from": fromWrapper,
		},
	}
	encodedRoute, err := json.Marshal(flowRoute)
	if err != nil {
		return nil, err
	}
	it.Spec.Flows = append(it.Spec.Flows, v1.Flow{RawMessage: encodedRoute})

	return &it, nil
}

func configureBinding(integration *v1.Integration, bindings ...*bindings.Binding) error {
	for _, b := range bindings {
		if b == nil {
			continue
		}
		if err := integration.Spec.Traits.Merge(b.Traits); err != nil {
			return err
		}
		for k, v := range b.ApplicationProperties {
			entry, err := property.EncodePropertyFileEntry(k, v)
			if err != nil {
				return err
			}

			integration.Spec.Configuration = append(integration.Spec.Configuration, v1.ConfigurationSpec{
				Type:  "property",
				Value: entry,
			})
		}

	}

	return nil
}

func determineTraitProfile(ctx context.Context, c client.Client, binding *v1.Pipe) (v1.TraitProfile, error) {
	if binding.Spec.Integration != nil && binding.Spec.Integration.Profile != "" {
		return binding.Spec.Integration.Profile, nil
	}
	pl, err := platform.GetForResource(ctx, c, binding)
	if err != nil && !k8serrors.IsNotFound(err) {
		return "", fmt.Errorf("error while retrieving the integration platform: %w", err)
	}
	if pl != nil {
		if pl.Status.Profile != "" {
			return pl.Status.Profile, nil
		}
		if pl.Spec.Profile != "" {
			return pl.Spec.Profile, nil
		}
	}
	if ok, err := knative.IsServingInstalled(c); err != nil {
		return "", err
	} else if ok {
		return v1.TraitProfileKnative, nil
	}
	if pl != nil {
		// Determine profile from cluster type
		plProfile := platform.GetTraitProfile(pl)
		if plProfile != "" {
			return plProfile, nil
		}
	}
	return v1.DefaultTraitProfile, nil
}
