/*
Copyright 2020 DevSpace Technologies Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package convert

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/ghodss/yaml"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var diffSeparator = regexp.MustCompile(`\n---`)

// StringToUnstructuredArray splits a YAML file into unstructured objects. Returns a list of all unstructured objects
func StringToUnstructuredArray(out string) ([]*unstructured.Unstructured, error) {
	parts := diffSeparator.Split(out, -1)
	var objs []*unstructured.Unstructured
	var firstErr error
	for _, part := range parts {
		var objMap map[string]interface{}
		err := yaml.Unmarshal([]byte(part), &objMap)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("Failed to unmarshal manifest: %v", err)
			}
			continue
		}
		if len(objMap) == 0 {
			// handles case where theres no content between `---`
			continue
		}
		var obj unstructured.Unstructured
		err = yaml.Unmarshal([]byte(part), &obj)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("Failed to unmarshal manifest: %v", err)
			}
			continue
		}
		objs = append(objs, &obj)
	}
	return objs, firstErr
}

// RuntimeObjectToBytes converts a runtime object into a byte array
func RuntimeObjectToBytes(obj runtime.Object) ([]byte, error) {
	bytes, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}

	return bytes, nil
}

// StringToUnstructured expects a single object via string and parses it into an unstructured object
func StringToUnstructured(out string) (*unstructured.Unstructured, error) {
	var obj unstructured.Unstructured
	err := yaml.Unmarshal([]byte(out), &obj)
	if err != nil {
		return nil, err
	}
	return &obj, nil
}

// ObjectToObject converts one object into another
func ObjectToObject(from interface{}, to interface{}) error {
	fromBytes, err := yaml.Marshal(from)
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(fromBytes, to)
	if err != nil {
		return err
	}

	return nil
}

// BytesToObject converts a given byte array into the object
func BytesToObject(from []byte, to schema.GroupVersionKind, scheme *runtime.Scheme) (runtime.Object, error) {
	new, err := scheme.New(to)
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal(from, new)
	if err != nil {
		return nil, err
	}

	return new, nil
}

// UnstructuredToObject converts a given unstructured object into a runtime object
func UnstructuredToObject(from *unstructured.Unstructured, to schema.GroupVersionKind, scheme *runtime.Scheme) (runtime.Object, error) {
	fromBytes, err := yaml.Marshal(from)
	if err != nil {
		return nil, err
	}

	new, err := scheme.New(to)
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal(fromBytes, new)
	if err != nil {
		return nil, err
	}

	return new, nil
}
