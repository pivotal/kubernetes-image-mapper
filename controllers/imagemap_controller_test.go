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

package controllers_test

import (
	"context"
	"errors"
	"reflect"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	mapperv1alpha1 "github.com/pivotal/kubernetes-image-mapper/api/v1alpha1"
	"github.com/pivotal/kubernetes-image-mapper/controllers"
	"github.com/pivotal/kubernetes-image-mapper/pkg/unimap"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("ImagemapController", func() {
	const (
		testImageMap                = "myimagemap"
		testNamespace               = "namespace"
		testOriginalImageReference  = "from"
		testRelocatedImageReference = "to"
		testGeneration              = int64(1)
	)

	var (
		stopCh     chan struct{}
		reconciler *controllers.ImageMapReconciler
		composite  unimap.Composite
		req        ctrl.Request
		get        func(context.Context, client.ObjectKey, runtime.Object) error
		update     func(context.Context, runtime.Object, ...client.UpdateOption) error
		status     func() client.StatusWriter
		result     ctrl.Result
		err        error
		imageMap   *mapperv1alpha1.ImageMap
		testErr    error
	)

	BeforeEach(func() {
		stopCh = make(chan struct{})
		imageMap = &mapperv1alpha1.ImageMap{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name:       testImageMap,
				Namespace:  testNamespace,
				Generation: testGeneration,
			},
			Spec: mapperv1alpha1.ImageMapSpec{
				Map: []mapperv1alpha1.Maplet{{
					From: testOriginalImageReference,
					To:   testRelocatedImageReference,
				}},
			},
			Status: mapperv1alpha1.ImageMapStatus{},
		}

		composite = unimap.New(stopCh)
		reconciler = &controllers.ImageMapReconciler{
			Client: testClient{
				get: func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
					return get(ctx, key, obj)
				},
				update: func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
					return update(ctx, obj, opts...)
				},
				status: func() client.StatusWriter {
					return status()
				},
			},
			Log: ctrl.Log.WithName("testing"),
			Map: composite,
		}
		req = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: testNamespace,
				Name:      testImageMap,
			},
		}
		testErr = errors.New("epic fail")
	})

	AfterEach(func() {
		stopCh <- struct{}{}
	})

	JustBeforeEach(func() {
		result, err = reconciler.Reconcile(req)
	})

	Context("when the image map named by the request exists", func() {
		var (
			updateErr         error
			updateCalls       int
			statusUpdateErr   error
			statusUpdateCalls int
		)

		BeforeEach(func() {
			get = func(ctx context.Context, objectKey client.ObjectKey, out runtime.Object) error {
				Expect(objectKey).To(Equal(req.NamespacedName))
				outVal := reflect.ValueOf(out)
				reflect.Indirect(outVal).Set(reflect.Indirect(reflect.ValueOf(imageMap)))
				return nil
			}
			updateErr = nil
			updateCalls = 0
			update = func(_ context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
				updateCalls += 1
				var ok bool
				imageMap, ok = obj.(*mapperv1alpha1.ImageMap)
				Expect(ok).To(BeTrue())
				Expect(opts).To(BeEmpty())
				return updateErr
			}
			statusUpdateErr = nil
			statusUpdateCalls = 0
			status = func() client.StatusWriter {
				update := func(_ context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
					statusUpdateCalls += 1
					updatedStatusImageMap, ok := obj.(*mapperv1alpha1.ImageMap)
					Expect(ok).To(BeTrue())
					imageMap.Status = updatedStatusImageMap.Status
					Expect(opts).To(BeEmpty())
					return statusUpdateErr
				}
				patch := func(ctx context.Context, obj runtime.Object, patch client.Patch, opts ...client.PatchOption) error {
					return nil
				}
				return newStatusWriter(update, patch)
			}
		})

		It("should succeed and not be requeued", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
		})

		It("should set the finalizer", func() {
			Expect(updateCalls).To(Equal(1))
			Expect(imageMap.ObjectMeta.Finalizers).To(ConsistOf("mapper.imagerelocation.pivotal.io"))
		})

		It("should update the status accordingly", func() {
			Expect(statusUpdateCalls).To(Equal(1))
			Expect(imageMap.Status.ObservedGeneration).To(Equal(testGeneration))
			Expect(len(imageMap.Status.Conditions)).To(Equal(1))
			cond := imageMap.Status.Conditions[0]
			Expect(cond.Type).To(Equal(mapperv1alpha1.ConditionReady))
			Expect(cond.Status).To(Equal(corev1.ConditionTrue))
			Expect(cond.Message).To(BeEmpty())
		})

		It("should update the composite", func() {
			Expect(composite.Map(testNamespace, testOriginalImageReference)).To(Equal(testRelocatedImageReference))
		})

		Context("when updating the image map return an error", func() {
			BeforeEach(func() {
				updateErr = testErr
			})

			It("should return the error", func() {
				Expect(err).To(MatchError(testErr))
			})
		})

		Context("when updating the status returns an error", func() {
			BeforeEach(func() {
				statusUpdateErr = testErr
			})

			It("should return the error", func() {
				Expect(err).To(MatchError(testErr))
			})
		})

		Context("when reconcile is called twice", func() {
			BeforeEach(func() {
				result, err = reconciler.Reconcile(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeFalse())
			})

			It("should be idempotent", func() {
				Expect(updateCalls).To(Equal(1))
				Expect(statusUpdateCalls).To(Equal(1))
			})

			Context("when image map has been deleted before the second call to reconcile", func() {
				BeforeEach(func() {
					now := metav1.NewTime(time.Now())
					imageMap.ObjectMeta.DeletionTimestamp = &now
				})

				It("should remove the finalizer", func() {
					Expect(updateCalls).To(Equal(2))
					Expect(imageMap.ObjectMeta.Finalizers).To(BeEmpty())
				})

				It("should not update the image map status again", func() {
					Expect(statusUpdateCalls).To(Equal(1))
				})

				It("should delete the image map from the composite", func() {
					Expect(composite.Map(testNamespace, testOriginalImageReference)).To(Equal(testOriginalImageReference))
				})

				Context("when updating the image map to remove the finalizer return an error", func() {
					BeforeEach(func() {
						updateErr = testErr
					})

					It("should return the error", func() {
						Expect(err).To(MatchError(testErr))
					})
				})
			})
		})

		Context("when there is a conflict", func() {
			BeforeEach(func() {
				composite.Add(testNamespace, "conflicting", map[string]string{testOriginalImageReference: "conflict"})
			})

			It("should succeed", func() {
				Expect(err).NotTo(HaveOccurred())
			})

			It("should be requeued", func() { // until https://github.com/pivotal/kubernetes-image-mapper/issues/7
				Expect(result.Requeue).To(BeTrue())
			})

			It("should set the finalizer", func() {
				Expect(updateCalls).To(Equal(1))
				Expect(imageMap.ObjectMeta.Finalizers).To(ConsistOf("mapper.imagerelocation.pivotal.io"))
			})

			It("should update the status accordingly", func() {
				Expect(statusUpdateCalls).To(Equal(1))
				Expect(imageMap.Status.ObservedGeneration).To(Equal(testGeneration))
				Expect(len(imageMap.Status.Conditions)).To(Equal(1))
				cond := imageMap.Status.Conditions[0]
				Expect(cond.Type).To(Equal(mapperv1alpha1.ConditionReady))
				Expect(cond.Status).To(Equal(corev1.ConditionFalse))
				Expect(cond.Message).To(Equal(`imagemap "myimagemap" in namespace "namespace" was rejected: it maps "from" to "to" but conflicting maps "from" to "conflict"`))
			})
		})
	})

	Context("when the image map named by the request is not found", func() {
		var notFoundErr error

		BeforeEach(func() {
			notFoundErr = apierrs.NewNotFound(schema.GroupResource{Group: mapperv1alpha1.GroupVersion.Group, Resource: "ImageMap"}, testImageMap)
			get = func(ctx context.Context, objectKey client.ObjectKey, out runtime.Object) error {
				return notFoundErr
			}
		})

		It("should succeed", func() {
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("when getting the image map named by the request returns an error other than 'not found'", func() {
		BeforeEach(func() {
			get = func(ctx context.Context, objectKey client.ObjectKey, out runtime.Object) error {
				return testErr
			}
		})

		It("should return the error", func() {
			Expect(err).To(MatchError(testErr))
		})
	})
})

type testClient struct {
	get    func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error
	update func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error
	status func() client.StatusWriter
}

func (c testClient) Get(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
	return c.get(ctx, key, obj)
}

func (c testClient) Update(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
	return c.update(ctx, obj, opts...)
}

func (c testClient) Status() client.StatusWriter {
	return c.status()
}

type testStatusWriter struct {
	update func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error
	patch  func(ctx context.Context, obj runtime.Object, patch client.Patch, opts ...client.PatchOption) error
}

func (w testStatusWriter) Update(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
	return w.update(ctx, obj, opts...)
}

func (w testStatusWriter) Patch(ctx context.Context, obj runtime.Object, patch client.Patch, opts ...client.PatchOption) error {
	return w.patch(ctx, obj, patch, opts...)
}

func newStatusWriter(update func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error, patch func(ctx context.Context, obj runtime.Object, patch client.Patch, opts ...client.PatchOption) error) client.StatusWriter {
	return testStatusWriter{
		update: update,
		patch:  patch,
	}
}
