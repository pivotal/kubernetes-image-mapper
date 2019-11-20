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

package unimap

import (
	"errors"
	"fmt"
)

// Composite is an interface to a collection, indexed by namespace, of consistent mappings from string to string. The
// mappings are consistent in the sense that no two mappings map a given pair to distinct results.
type Composite interface {
	// Add updates the composite mapping by adding a mapping with the given namespace and name. If a mapping with the
	// given namespace and name already exists, it is replaced providing it is consistent with all other mappings. If
	// the named mapping is not consistent with all other mappings in the same namespace, the mapping is removed from
	// the composite mapping for the namespace and error is returned.
	Add(namespace string, name string, mapping map[string]string) error

	// Delete updates the composite mapping by removing a mapping with the given namespace and name. If there is no
	// mapping with the given namespace and name, an error is returned.
	Delete(namespace string, name string) error

	// Map applies the composite mapping for the given namespace to the given name and returns the mapped value. If the
	// given namespace is not known or the given name is not in the domain of the composite mapping for the namespace,
	// the given name is returned. In other words, the default composite mapping for any namespace is the identity
	// function.
	Map(namespace string, name string) string

	// Dump returns a string representing the state of the composite.
	Dump() string
}

type errCh chan error

type addOp struct {
	namespace string
	name      string
	mapping   relmap
	errCh     errCh
}

type deleteOp struct {
	namespace string
	name      string
	errCh     errCh
}

type mapOp struct {
	namespace string
	name      string
	resultCh  chan string
}

type dumpOp struct {
	resultCh chan string
}

type namespace string

// a relmap is a relocation mapping which maps image references to image references
type relmap map[string]string

// a unimap is a consistent collection of relocation mappings, consistent in the sense that no two distinct keys of the
// unimap have corresponding relocation mappings which a particular image reference to distinct values
type unimap map[string]relmap

const stoppedMsg = "composite is stopped"

var stoppedErr = errors.New(stoppedMsg)

type composite struct {
	// a collection of unimaps indexed by namespace
	mappings map[namespace]unimap

	// a collection of mappings indexed by namespace each of which is the composition of all the relocation mappings in the namespace's unimap
	composite map[namespace]relmap

	addCh    chan *addOp
	deleteCh chan *deleteOp
	mapCh    chan *mapOp
	dumpCh   chan *dumpOp
	stopCh   <-chan struct{}
}

// New creates a Composite and starts a monitor. When the given channel is closed, the monitor is stopped after which
// all Composite methods return an error or no-op.
func New(stopCh <-chan struct{}) Composite {
	c := &composite{
		mappings:  make(map[namespace]unimap),
		composite: make(map[namespace]relmap),

		addCh:    make(chan *addOp),
		deleteCh: make(chan *deleteOp),
		mapCh:    make(chan *mapOp),
		dumpCh:   make(chan *dumpOp),
		stopCh:   stopCh,
	}

	go c.monitor()

	return c
}

func (c *composite) Add(namespace string, name string, mapping map[string]string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = stoppedErr
		}
	}()
	errCh := make(chan error)
	c.addCh <- &addOp{
		namespace: namespace,
		name:      name,
		mapping:   mapping,
		errCh:     errCh,
	}
	return <-errCh
}
func (c *composite) Delete(namespace string, name string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = stoppedErr
		}
	}()
	errCh := make(chan error)
	c.deleteCh <- &deleteOp{
		namespace: namespace,
		name:      name,
		errCh:     errCh,
	}
	return <-errCh
}

func (c *composite) Map(namespace string, value string) (result string) {
	defer func() {
		if r := recover(); r != nil {
			result = value
		}
	}()
	resultCh := make(chan string)
	c.mapCh <- &mapOp{
		namespace: namespace,
		name:      value,
		resultCh:  resultCh,
	}
	return <-resultCh
}

func (c *composite) Dump() (result string) {
	defer func() {
		if r := recover(); r != nil {
			result = stoppedMsg
		}
	}()
	resultCh := make(chan string)
	c.dumpCh <- &dumpOp{
		resultCh: resultCh,
	}
	return <-resultCh
}

func (c *composite) monitor() {
	for {
		select {
		case addOp := <-c.addCh:
			addOp.errCh <- c.add(addOp.namespace, addOp.name, addOp.mapping)

		case deleteOp := <-c.deleteCh:
			deleteOp.errCh <- c.delete(deleteOp.namespace, deleteOp.name)

		case mapOp := <-c.mapCh:
			mapOp.resultCh <- c.doMap(mapOp.namespace, mapOp.name)

		case dumpOp := <-c.dumpCh:
			dumpOp.resultCh <- c.dump()

		case <-c.stopCh:
			close(c.addCh)
			close(c.deleteCh)
			close(c.mapCh)
			close(c.dumpCh)
			return
		}
	}
}

func (c *composite) add(ns string, name string, mapping relmap) error {
	_ = c.delete(ns, name) // name may not be present, so ignore any error
	n := namespace(ns)
	if err := c.checkConsistency(n, name, mapping); err != nil {
		return err
	}

	// save a copy of mapping
	nsMapping := c.getNamespaceMapping(n)
	nsMapping[name] = make(relmap, len(mapping))
	for k, v := range mapping {
		nsMapping[name][k] = v
	}

	c.merge(n)
	return nil
}

func (c *composite) getNamespaceMapping(ns namespace) unimap {
	nsMapping, ok := c.mappings[ns]
	if !ok {
		nsMapping = make(map[string]relmap)
		c.mappings[ns] = nsMapping
		c.composite[ns] = make(relmap)
	}
	return nsMapping
}

func (c *composite) delete(ns string, name string) error {
	n := namespace(ns)
	nsMapping := c.getNamespaceMapping(n)
	if _, ok := nsMapping[name]; !ok {
		return fmt.Errorf("mapping not found: %s", name)
	}
	delete(nsMapping, name)
	c.merge(n)
	return nil
}

func (c *composite) doMap(ns string, value string) string {
	n := namespace(ns)
	_ = c.getNamespaceMapping(n)
	if result, ok := c.composite[n][value]; ok {
		return result
	}
	return value
}

func (c *composite) dump() string {
	return fmt.Sprintf("%#v", c)
}

func (c *composite) merge(ns namespace) {
	c.composite[ns] = make(map[string]string)
	empty := true
	for _, m := range c.mappings[ns] {
		empty = false
		for k, v := range m {
			c.composite[ns][k] = v
		}
	}
	if empty {
		// avoid leaking memory when namespaces go away
		delete(c.mappings, ns)
	}
}

func (c *composite) checkConsistency(ns namespace, name string, mapping map[string]string) error {
	collisions := ""
	for k, v := range mapping {
		if w, ok := c.composite[ns][k]; ok && v != w {
			for n, m := range c.mappings[ns] {
				if w, ok := m[k]; ok && v != w {
					sep := ""
					if collisions != "" {
						sep = ", "
					}
					collisions = fmt.Sprintf("%s%sit maps %q to %q but %s maps %q to %q", collisions, sep, k, v, n, k, w)
					break
				}
			}
		}
	}
	if collisions != "" {
		return fmt.Errorf("imagemap %q in namespace %q was rejected: %s", name, ns, collisions)
	}
	return nil
}
