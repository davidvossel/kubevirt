/*
 * This file is part of the kubevirt project
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
 *
 * Copyright 2017 Red Hat, Inc.
 *
 */

package watchdog

import (
	"io/ioutil"
	"os"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"kubevirt.io/kubevirt/pkg/controller"
	"kubevirt.io/kubevirt/pkg/virt-handler/virtwrap/api"
)

var _ = Describe("Inotify", func() {

	Context("When watching files in a directory", func() {

		var tmpDir string
		var informer cache.SharedIndexInformer
		var stopInformer chan struct{}
		var queue workqueue.RateLimitingInterface

		TestForKeyEvent := func(expectedKey string, shouldExist bool) bool {
			// wait for key to either enter or exit the store.
			Eventually(func() bool {
				_, exists, _ := informer.GetStore().GetByKey(expectedKey)

				if shouldExist == exists {
					return true
				}
				return false
			}).Should(BeTrue())

			// ensure queue item for key exists
			len := queue.Len()
			for i := len; i > 0; i-- {
				key, _ := queue.Get()
				defer queue.Done(key)
				if key == expectedKey {
					return true
				}
			}
			return false
		}

		startWatchdogInformer := func() {
			var err error
			stopInformer = make(chan struct{})
			tmpDir, err = ioutil.TempDir("", "kubevirt")
			Expect(err).ToNot(HaveOccurred())

			queue = workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
			informer = cache.NewSharedIndexInformer(
				NewWatchdogListWatchFromClient(tmpDir, 1),
				&api.Domain{},
				0,
				cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})

			informer.AddEventHandler(controller.NewResourceEventHandlerFuncsForWorkqueue(queue))
			go informer.Run(stopInformer)
			Expect(cache.WaitForCacheSync(stopInformer, informer.HasSynced)).To(BeTrue())
		}

		It("should detect expired watchdog files", func() {
			startWatchdogInformer()

			keyExpired := "default/expiredvm"
			fileName := tmpDir + "/default_expiredvm"
			Expect(os.Create(fileName)).ToNot(BeNil())

			files, err := detectExpiredFiles(1, tmpDir)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(files)).To(Equal(0))

			time.Sleep(time.Second * 3)

			Expect(TestForKeyEvent(keyExpired, true)).To(Equal(true))

			files, err = detectExpiredFiles(1, tmpDir)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(files)).To(Equal(1))

			Expect(os.Create(fileName)).ToNot(BeNil())
			files, err = detectExpiredFiles(1, tmpDir)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(files)).To(Equal(0))
		})

		AfterEach(func() {
			close(stopInformer)
			os.RemoveAll(tmpDir)
		})

	})
})
