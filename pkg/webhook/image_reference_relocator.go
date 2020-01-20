/*
 * Copyright (c) 2019-Present Pivotal Software, Inc. All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package relocatingwebhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/pivotal/kubernetes-image-mapper/pkg/unimap"

	"gomodules.xyz/jsonpatch/v2"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:verbs=create;update,path=/image-mapper,mutating=true,failurePolicy=fail,groups="",resources=pods,versions=v1,name=image-mapper.imagerelocation.pivotal.io

const kubeSystemNamespace = "kube-system"

type imageReferenceRelocator struct {
	comp    unimap.Composite
	client  client.Client
	decoder *admission.Decoder
}

func NewImageReferenceRelocator(comp unimap.Composite) *imageReferenceRelocator {
	return &imageReferenceRelocator{
		comp: comp,
	}
}

var _ ExtendedHandler = &imageReferenceRelocator{}

var (
	podResource = metav1.GroupVersionResource{Version: "v1", Resource: "pods"}
)

func (i *imageReferenceRelocator) Handle(ctx context.Context, req admission.Request) admission.Response {
	// Ignore non-pod resources.
	if req.Resource != podResource {
		return admission.Allowed(fmt.Sprintf("unexpected resource (not a pod): %v", req.Resource))
	}

	// Ignore kube-system pods.
	if req.Namespace == kubeSystemNamespace {
		return admission.Allowed(fmt.Sprintf("ignoring pod in %s namespace", kubeSystemNamespace))
	}

	rawPod := req.Object.Raw
	pod := corev1.Pod{}

	if len(rawPod) > 0 {
		err := i.decoder.Decode(req, &pod)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, fmt.Errorf("decoding pod: %s", err))
		}
	} else {
		err := i.client.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: req.Name}, &pod)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, fmt.Errorf("getting pod: %s", err))
		}
	}

	modified := false

	for j, _ := range pod.Spec.InitContainers {
		container := &pod.Spec.InitContainers[j]
		if relocated := i.comp.Map(req.Namespace, container.Image); relocated != container.Image {
			modified = true
			container.Image = relocated
		}
	}

	for j, _ := range pod.Spec.Containers {
		container := &pod.Spec.Containers[j]
		if relocated := i.comp.Map(req.Namespace, container.Image); relocated != container.Image {
			modified = true
			container.Image = relocated
		}
	}

	if !modified {
		return admission.Allowed("unmodified")
	}

	updatedPodRaw, err := json.Marshal(pod)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("marshaling pod: %s", err))
	}

	patch, err := jsonpatch.CreatePatch(rawPod, updatedPodRaw)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("creating patch: %s", err))
	}

	return admission.Patched("pod relocated", patch...)
}

func (i *imageReferenceRelocator) InjectClient(client client.Client) error {
	i.client = client
	return nil
}

func (i *imageReferenceRelocator) InjectDecoder(decoder *admission.Decoder) error {
	i.decoder = decoder
	return nil
}
