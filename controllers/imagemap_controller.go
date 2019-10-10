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

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mapperv1alpha1 "github.com/pivotal/kubernetes-image-mapper/api/v1alpha1"
)

// ImageMapReconciler reconciles a ImageMap object
type ImageMapReconciler struct {
	client.Client
	Log logr.Logger
}

// +kubebuilder:rbac:groups=mapper.imagerelocation.pivotal.io,resources=imagemaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mapper.imagerelocation.pivotal.io,resources=imagemaps/status,verbs=get;update;patch

func (r *ImageMapReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = r.Log.WithValues("imagemap", req.NamespacedName)

	// your logic here

	return ctrl.Result{}, nil
}

func (r *ImageMapReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mapperv1alpha1.ImageMap{}).
		Complete(r)
}
