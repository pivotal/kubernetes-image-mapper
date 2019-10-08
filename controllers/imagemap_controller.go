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

package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"k8s.io/apimachinery/pkg/runtime"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mapperv1alpha1 "github.com/pivotal/kubernetes-image-mapper/api/v1alpha1"
	"github.com/pivotal/kubernetes-image-mapper/pkg/unimap"
)

// Client supports CRUD operations on kubernetes objects
type Client interface {
	Get(ctx context.Context, key client.ObjectKey, obj runtime.Object) error
	Update(context.Context, runtime.Object, ...client.UpdateOption) error
	Status() client.StatusWriter
}

// ImageMapReconciler reconciles an ImageMap object
type ImageMapReconciler struct {
	Client
	Log logr.Logger
	Map unimap.Composite
}

const finalizerName = "mapper.imagerelocation.pivotal.io"

// +kubebuilder:rbac:groups=mapper.imagerelocation.pivotal.io,resources=imagemaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mapper.imagerelocation.pivotal.io,resources=imagemaps/status,verbs=get;update;patch

func (r *ImageMapReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	resp, err := r.reconcile(req)
	if r.Log.V(1).Enabled() && err == nil {
		jsonReq, _ := json.Marshal(req)
		jsonResp, _ := json.Marshal(resp)
		r.Log.V(1).Info(fmt.Sprintf("req: %s resp: %s\n", jsonReq, jsonResp))
	}
	return resp, err
}

func (r *ImageMapReconciler) reconcile(req ctrl.Request) (ctrl.Result, error) {

	ctx := context.Background()
	log := r.Log.WithValues("imagemap", req.NamespacedName)

	var imageMap mapperv1alpha1.ImageMap
	if err := r.Get(ctx, req.NamespacedName, &imageMap); err != nil {
		if apierrs.IsNotFound(err) {
			log.Info("deleting")
			_ = r.Map.Delete(req.Namespace, req.Name) // ignore error in case it has already been deleted
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to get imagemap")
		return ctrl.Result{}, err
	}

	// copy the image map in case we need to mutate it
	imageMapCopy := *imageMap.DeepCopy()

	// record the image map generation being reconciled
	imageMapCopy.Status.ObservedGeneration = imageMapCopy.ObjectMeta.Generation

	// imageMap is being deleted if and only if the deletion timestamp is set. If it is not being deleted, register our
	// finalizer, else delete the imageMap state and remove our finalizer.
	if imageMapCopy.ObjectMeta.DeletionTimestamp.IsZero() {
		// ensure our finalizer is present so the imageMap will not be deleted until we have processed the deletion
		if !containsString(imageMapCopy.ObjectMeta.Finalizers, finalizerName) {
			imageMapCopy.ObjectMeta.Finalizers = append(imageMapCopy.ObjectMeta.Finalizers, finalizerName)
			if err := r.Update(ctx, &imageMapCopy); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		log.Info("deleting")
		_ = r.Map.Delete(req.Namespace, req.Name) // ignore error in case it has already been deleted

		// remove our finalizer if it's present
		if imageMapCopy.ObjectMeta.Finalizers != nil && containsString(imageMapCopy.ObjectMeta.Finalizers, finalizerName) {
			imageMapCopy.ObjectMeta.Finalizers = removeString(imageMapCopy.ObjectMeta.Finalizers, finalizerName)
			if err := r.Update(ctx, &imageMapCopy); err != nil {
				return ctrl.Result{}, err
			}
		}

		return ctrl.Result{}, nil
	}

	log.Info("adding")
	result := ctrl.Result{}
	if err := r.Map.Add(req.Namespace, req.Name, mapFromMaplets(imageMapCopy.Spec.Map)); err != nil {
		log.Error(err, "unable to add imagemap")
		addCondition(&imageMapCopy.Status.Conditions, mapperv1alpha1.Condition{
			Type:            mapperv1alpha1.ConditionReady,
			Status:          corev1.ConditionFalse,
			ObservationTime: metav1.NewTime(time.Now()),
			Message:         err.Error(),
		})
		// retry in case a conflicting imagemap is subsequently deleted or modified so that the conflict goes away
		// TODO: replace requeueing with tracking of failed imagemaps and explicit resync when appropriate (https://github.com/pivotal/kubernetes-image-mapper/issues/7)
		result.Requeue = true
		result.RequeueAfter = time.Minute
	} else {
		addCondition(&imageMapCopy.Status.Conditions, mapperv1alpha1.Condition{
			Type:            mapperv1alpha1.ConditionReady,
			Status:          corev1.ConditionTrue,
			ObservationTime: metav1.NewTime(time.Now()),
		})
	}
	if r.Log.V(1).Enabled() {
		r.Log.V(1).Info("mappings", "map", r.Map.Dump())
	}

	if different(&imageMapCopy.Status, &imageMap.Status) {
		log.Info("updating imagemap status")
		if updateErr := r.Status().Update(ctx, &imageMapCopy); updateErr != nil {
			log.Error(updateErr, "unable to update imagemap status")
			return ctrl.Result{Requeue: true}, updateErr
		}
	}

	return result, nil
}

// different compares two image map status values and returns true if and only if they are significantly different,
// i.e. if they differ in other than the order and observed time of conditions.
func different(s1 *mapperv1alpha1.ImageMapStatus, s2 *mapperv1alpha1.ImageMapStatus) bool {
	if s1.ObservedGeneration != s2.ObservedGeneration {
		return true
	}
	if len(s1.Conditions) != len(s2.Conditions) {
		return true
	}
	for _, c := range s1.Conditions {
		if c2, ok := findCondition(s2.Conditions, c.Type); ok {
			// compare conditions ignoring observation time (not significant if the rest of the condition is unchanged)
			if c.Status != (*c2).Status ||
				c.Message != (*c2).Message {
				return true
			}
		} else {
			return true
		}
	}
	return false
}

func mapFromMaplets(maplets []mapperv1alpha1.Maplet) map[string]string {
	result := make(map[string]string)
	for _, maplet := range maplets {
		result[maplet.From] = maplet.To
	}
	return result
}

func (r *ImageMapReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mapperv1alpha1.ImageMap{}).
		Complete(r)
}

func findCondition(conds []mapperv1alpha1.Condition, typ mapperv1alpha1.ConditionType) (*mapperv1alpha1.Condition, bool) {
	for _, c := range conds {
		if c.Type == typ {
			return &c, true
		}
	}
	return nil, false
}

func addCondition(conds *[]mapperv1alpha1.Condition, cond mapperv1alpha1.Condition) {
	others := removeCondition(*conds, cond.Type)
	*conds = append(others, cond)
}

func removeCondition(conds []mapperv1alpha1.Condition, typ mapperv1alpha1.ConditionType) []mapperv1alpha1.Condition {
	remainder := []mapperv1alpha1.Condition{}
	for _, c := range conds {
		if c.Type != typ {
			remainder = append(remainder, c)
		}
	}
	return remainder
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func removeString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}
