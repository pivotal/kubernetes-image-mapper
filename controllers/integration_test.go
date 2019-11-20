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
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	mapperv1alpha1 "github.com/pivotal/kubernetes-image-mapper/api/v1alpha1"
	"github.com/pivotal/kubernetes-image-mapper/controllers"
	"github.com/pivotal/kubernetes-image-mapper/pkg/unimap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sync"
)

const (
	waitTime        = "60s"
	pollingInterval = "100ms"
)

// ultimately is Eventually at "kubernetes speed"
func ultimately(actual interface{}) AsyncAssertion {
	return Eventually(actual, waitTime, pollingInterval)
}

var _ = Describe("Integration Test", func() {
	const (
		testImageMap                = "myimagemap"
		testNamespace               = "namespace"
		testOriginalImageReference  = "from"
		testRelocatedImageReference = "to"
		testGeneration              = int64(1)
	)

	var (
		clnt       client.Client
		composite  unimap.Composite
		stopCh     chan struct{}
		stopMgr    chan struct{}
		mgrStopped *sync.WaitGroup
	)

	BeforeEach(func() {
		// Setup the Manager and Controller.
		mgr, err := manager.New(cfg, manager.Options{})
		Expect(err).NotTo(HaveOccurred())
		clnt = mgr.GetClient()

		stopCh = make(chan struct{})
		composite = unimap.New(stopCh)

		err = (&controllers.ImageMapReconciler{
			Client: mgr.GetClient(),
			Log:    ctrl.Log.WithName("controllers").WithName("ImageMap"),
			Map:    composite,
		}).SetupWithManager(mgr)
		Expect(err).NotTo(HaveOccurred())

		stopMgr, mgrStopped = StartTestManager(mgr, NewWithT(GinkgoT()))
	})

	AfterEach(func() {
		close(stopCh)
		close(stopMgr)
		mgrStopped.Wait()
	})

	Context("when an image map is created", func() {
		var imageMap *mapperv1alpha1.ImageMap

		BeforeEach(func() {
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

			createImageMap(clnt, imageMap)
		})

		AfterEach(func() {
			safelyDeleteImageMap(clnt, imageMap)
		})

		It("should add the image map to the composite", func() {
			ultimately(func() string {
				return composite.Map(testNamespace, testOriginalImageReference)
			}).Should(Equal(testRelocatedImageReference))
		})

		Context("when the image map is deleted", func() {
			BeforeEach(func() {
				Expect(clnt.Delete(context.TODO(), imageMap)).To(Succeed())
			})

			It("should remove the image map from the composite", func() {
				ultimately(func() string {
					return composite.Map(testNamespace, testOriginalImageReference)
				}).Should(Equal(testOriginalImageReference))
			})
		})

		Context("when a conflicting image map is added", func() {
			const conflictingImageMapName = "conflicting"

			var conflictingImageMap *mapperv1alpha1.ImageMap

			BeforeEach(func() {
				conflictingImageMap = &mapperv1alpha1.ImageMap{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:       conflictingImageMapName,
						Namespace:  testNamespace,
						Generation: testGeneration,
					},
					Spec: mapperv1alpha1.ImageMapSpec{
						Map: []mapperv1alpha1.Maplet{{
							From: testOriginalImageReference,
							To:   "other",
						}},
					},
					Status: mapperv1alpha1.ImageMapStatus{},
				}

				createImageMap(clnt, conflictingImageMap)
			})

			AfterEach(func() {
				safelyDeleteImageMap(clnt, conflictingImageMap)
			})

			It("should set the status of the conflicting image map appropriately", func() {
				ultimately(func() error {
					notReady := errors.New("not ready")
					var im mapperv1alpha1.ImageMap
					clnt.Get(context.TODO(), client.ObjectKey{Namespace: testNamespace, Name: conflictingImageMapName}, &im)
					if len(im.Status.Conditions) != 1 {
						return notReady
					}
					cond := im.Status.Conditions[0]
					Expect(cond.Type).To(Equal(mapperv1alpha1.ConditionReady))
					Expect(cond.Status).To(Equal(corev1.ConditionFalse))
					Expect(cond.Message).To(Equal(`imagemap "conflicting" in namespace "namespace" was rejected: it maps "from" to "other" but myimagemap maps "from" to "to"`))
					return nil
				}).Should(Succeed())
			})
		})
	})
})

func createImageMap(clnt client.Client, imap *mapperv1alpha1.ImageMap) {
	// wait until the image map can be created as it may still be being deleted
	ultimately(func() error {
		err := clnt.Create(context.TODO(), imap)
		Expect(err).To(Or(Succeed(), MatchError(ContainSubstring("object is being deleted"))))
		return err
	}).Should(Succeed())

}

func safelyDeleteImageMap(clnt client.Client, imap *mapperv1alpha1.ImageMap) {
	_ = clnt.Delete(context.TODO(), imap) // ignore any error in case the test has deleted the image map
}
