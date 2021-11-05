/*
Copyright 2021 The Kubernetes Authors.

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

package yaml

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"

	kubejson "sigs.k8s.io/json"

	yamlv2 "gopkg.in/yaml.v2"
	"gopkg.in/yaml.v3"
)

type disallowUnknownFieldsOption bool

const (
	// disallowUnknownFields returns strict errors if data contains unknown fields when decoding into typed structs
	disallowUnknownFields disallowUnknownFieldsOption = true

	// allowUnknownFields returns no errors if data contains unknown fields when decoding into typed structs
	allowUnknownFields disallowUnknownFieldsOption = false
)

// Marshal marshals obj into JSON using stdlib json.Marshal, and then converts JSON to YAML using JSONToYAML (see that method for more reference)
func Marshal(obj interface{}) ([]byte, error) {
	if yamlNode, ok := obj.(*yaml.Node); ok {
		var buf bytes.Buffer
		if err := yamlNode.Decode(&buf); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}

	jsonBytes, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("error marshaling into JSON: %w", err)
	}

	return JSONToYAML(jsonBytes)
}

// Unmarshal first converts the given YAML to JSON, and then unmarshals the JSON into obj. Options for the
// standard library json.Decoder can be optionally specified, e.g. to decode untyped numbers into json.Number instead of float64, or to disallow unknown fields (but for that purpose, see also UnmarshalStrict). obj must be a non-nil pointer.
//
// Important notes about the Unmarshal logic:
//
//  - Decoding is case-sensitive, just like the rest of the Kubernetes API machinery decoder logic, but unlike the standard library JSON encoder.
//  - Duplicate fields (only case-sensitive matches) in objects result in a fatal error, as defined in the YAML spec.
//  - The sequence indentation style is wide, which means that the "- " marker for a YAML sequence will NOT be on the same indentation level as the sequence field name, but two spaces more indented.
//  - Unknown fields, i.e. serialized data that do not map to a field in obj, are ignored. Use UnmarshalStrict to override.
//  - YAML non-string keys, e.g. ints, bools and floats, are converted to strings implicitly during the YAML to JSON conversion process.
//  - There are no compatibility guarantees for returned error values.
func Unmarshal(yamlBytes []byte, obj interface{}) error {
	_, err := unmarshal(yamlBytes, obj, allowUnknownFields)
	return err
}

// UnmarshalStrict is similar to Unmarshal (please read its documentation for reference), with the following exceptions:
//
//  - If obj, or any of its recursive children, is a struct, presence of fields in the serialized data unknown to the struct will yield a strict error.
func UnmarshalStrict(yamlBytes []byte, obj interface{}) (strictErrors []error, err error) {
	return unmarshal(yamlBytes, obj, disallowUnknownFields)
}

// unmarshal unmarshals the given YAML byte stream into the given interface,
// optionally performing the unmarshalling strictly
func unmarshal(yamlBytes []byte, obj interface{}, unknownFieldsOption disallowUnknownFieldsOption) (strictErrors []error, err error) {
	if yamlNode, ok := obj.(*yaml.Node); ok {
		if err := yamlNode.Encode(yamlBytes); err != nil {
			return nil, err
		}
		return nil, nil
	}

	jsonTarget := reflect.ValueOf(obj)
	if jsonTarget.Kind() != reflect.Ptr || jsonTarget.IsNil() {
		return nil, fmt.Errorf("provided object is not a valid pointer")
	}

	jsonBytes, err := yamlToJSONTarget(yamlBytes, &jsonTarget)
	if err != nil {
		return nil, err
	}

	// Decode jsonBytes into obj.
	if unknownFieldsOption == disallowUnknownFields {
		strictErrors, err = kubejson.UnmarshalStrict(jsonBytes, &obj, kubejson.DisallowUnknownFields)
	} else {
		strictErrors, err = kubejson.UnmarshalStrict(jsonBytes, &obj)
	}

	if err != nil {
		return strictErrors, fmt.Errorf("error unmarshaling JSON: %w", err)
	}

	return strictErrors, nil
}

// JSONToYAML converts JSON to YAML. Notable implementation details:
//
//  - Duplicate fields (only case-sensitive matches) in objects result in a fatal error, as defined in the YAML spec.
func JSONToYAML(jsonBytes []byte) ([]byte, error) {
	// Convert the JSON to an object.
	var jsonObj interface{}

	if err := kubejson.UnmarshalCaseSensitivePreserveInts(jsonBytes, &jsonObj); err != nil {
		return nil, fmt.Errorf("error converting JSON to YAML: %w", err)
	}

	// Marshal this object into YAML.
	yamlBytes, err := yamlv2.Marshal(jsonObj)
	if err != nil {
		return nil, fmt.Errorf("error converting JSON to YAML: %w", err)
	}

	return yamlBytes, nil
}

// YAMLToJSON converts YAML to JSON. Since JSON is a subset of YAML,
// passing JSON through this method should be a no-op.
//
// Some things YAML can do that are not supported by JSON:
// * In YAML you can have binary and null keys in your maps. These are invalid
//   in JSON, and therefore int, bool and float keys are converted to strings implicitly.
// * Binary data in YAML with the !!binary tag is not supported. If you want to
//   use binary data with this library, encode the data as base64 as usual but do
//   not use the !!binary tag in your YAML. This will ensure the original base64
//   encoded data makes it all the way through to the JSON.
// * And more... read the YAML specification for more details.
//
// Notable about the implementation:
//
// - Duplicate fields (only case-sensitive matches) in objects result in a fatal error, as defined in the YAML spec.
// - There are no compatibility guarantees for returned error values.
func YAMLToJSON(yamlBytes []byte) ([]byte, error) {
	return yamlToJSONTarget(yamlBytes, nil)
}

func yamlToJSONTarget(yamlBytes []byte, jsonTarget *reflect.Value) (jsonBytes []byte, err error) {
	// Convert the YAML to an object.
	var yamlObj interface{}
	err = yaml.Unmarshal(yamlBytes, &yamlObj)
	if err != nil {
		return nil, fmt.Errorf("error converting YAML to JSON: %w", err)
	}

	// YAML objects are not completely compatible with JSON objects (e.g. you
	// can have non-string keys in YAML). So, convert the YAML-compatible object
	// to a JSON-compatible object, failing with an error if irrecoverable
	// incompatibilties happen along the way.
	jsonObj, err := convertToJSONableObject(yamlObj, jsonTarget)
	if err != nil {
		return nil, fmt.Errorf("error converting YAML to JSON: %w", err)
	}

	// Convert this object to JSON and return the data.
	jsonBytes, err = json.Marshal(jsonObj)
	if err != nil {
		return nil, fmt.Errorf("error converting YAML to JSON: %w", err)
	}
	return jsonBytes, nil
}

func convertToJSONableObject(yamlObj interface{}, jsonTarget *reflect.Value) (interface{}, error) {
	var err error

	// Resolve jsonTarget to a concrete value (i.e. not a pointer or an
	// interface). We pass decodingNull as false because we're not actually
	// decoding into the value, we're just checking if the ultimate target is a
	// string.
	if jsonTarget != nil {
		jsonTarget = indirect(*jsonTarget)
	}

	// Transform map[string]interface{} into map[interface{}]interface{}
	if stringMap, ok := yamlObj.(map[string]interface{}); ok {
		interfaceMap := make(map[interface{}]interface{})
		for k, v := range stringMap {
			interfaceMap[k] = v
		}
		yamlObj = interfaceMap
	}

	// If yamlObj is a number or a boolean, check if jsonTarget is a string -
	// if so, coerce.  Else return normal.
	// If yamlObj is a map or array, find the field that each key is
	// unmarshaling to, and when you recurse pass the reflect.Value for that
	// field back into this function.
	switch typedYAMLObj := yamlObj.(type) {
	case map[interface{}]interface{}:
		// JSON does not support arbitrary keys in a map, so we must convert
		// these keys to strings.
		//
		// From my reading of go-yaml v2 (specifically the resolve function),
		// keys can only have the types string, int, int64, float64, binary
		// (unsupported), or null (unsupported).
		strMap := make(map[string]interface{})
		for k, v := range typedYAMLObj {
			// Resolve the key to a string first.
			var keyString string
			switch typedKey := k.(type) {
			case string:
				keyString = typedKey
			case int:
				keyString = strconv.Itoa(typedKey)
			case int64:
				// go-yaml will only return an int64 as a key if the system
				// architecture is 32-bit and the key's value is between 32-bit
				// and 64-bit. Otherwise the key type will simply be int.
				keyString = strconv.FormatInt(typedKey, 10)
			case float64:
				// Stolen from go-yaml to use the same conversion to string as
				// the go-yaml library uses to convert float to string when
				// Marshaling.
				s := strconv.FormatFloat(typedKey, 'g', -1, 32)
				switch s {
				case "+Inf":
					s = ".inf"
				case "-Inf":
					s = "-.inf"
				case "NaN":
					s = ".nan"
				}
				keyString = s
			case bool:
				if typedKey {
					keyString = "true"
				} else {
					keyString = "false"
				}
			default:
				return nil, fmt.Errorf("unsupported map key of type: %s, key: %+#v, value: %+#v",
					reflect.TypeOf(k), k, v)
			}

			// jsonTarget should be a struct or a map. If it's a struct, find
			// the field it's going to map to and pass its reflect.Value. If
			// it's a map, find the element type of the map and pass the
			// reflect.Value created from that type. If it's neither, just pass
			// nil - JSON conversion will error for us if it's a real issue.
			if jsonTarget != nil {
				t := *jsonTarget
				if t.Kind() == reflect.Struct {
					// Find the field that the JSON library would use.
					var f *field
					fields := cachedTypeFields(t.Type())
					for i := range fields {
						f = &fields[i]
						if f.name == keyString {
							break
						}
					}

					if f != nil {
						// Find the reflect.Value of the most preferential
						// struct field.
						jtf := t
						for _, i := range f.index {
							if jtf.Kind() == reflect.Ptr {
								if jtf.IsNil() {
									jtf = reflect.New(jtf.Type().Elem())
								}
								jtf = jtf.Elem()
							}
							jtf = jtf.Field(i)
						}

						strMap[keyString], err = convertToJSONableObject(v, &jtf)
						if err != nil {
							return nil, err
						}
						continue
					}
				} else if t.Kind() == reflect.Map {
					// Create a zero value of the map's element type to use as
					// the JSON target.
					jtv := reflect.Zero(t.Type().Elem())
					strMap[keyString], err = convertToJSONableObject(v, &jtv)
					if err != nil {
						return nil, err
					}
					continue
				}
			}
			strMap[keyString], err = convertToJSONableObject(v, nil)
			if err != nil {
				return nil, err
			}
		}
		return strMap, nil
	case []interface{}:
		// We need to recurse into arrays in case there are any
		// map[interface{}]interface{}'s inside and to convert any
		// numbers to strings.

		// If jsonTarget is a slice (which it really should be), find the
		// thing it's going to map to. If it's not a slice, just pass nil
		// - JSON conversion will error for us if it's a real issue.
		var jsonSliceElemValue *reflect.Value
		if jsonTarget != nil {
			t := *jsonTarget
			if t.Kind() == reflect.Slice {
				// By default slices point to nil, but we need a reflect.Value
				// pointing to a value of the slice type, so we create one here.
				ev := reflect.Indirect(reflect.New(t.Type().Elem()))
				jsonSliceElemValue = &ev
			}
		}

		// Make and use a new array.
		arr := make([]interface{}, len(typedYAMLObj))
		for i, v := range typedYAMLObj {
			arr[i], err = convertToJSONableObject(v, jsonSliceElemValue)
			if err != nil {
				return nil, err
			}
		}
		return arr, nil
	default:
		// If the target type is a string and the YAML type is a number,
		// convert the YAML type to a string.
		if jsonTarget != nil && (*jsonTarget).Kind() == reflect.String {
			// Based on my reading of go-yaml, it may return int, int64,
			// float64, or uint64.
			var s string
			switch typedVal := typedYAMLObj.(type) {
			case int:
				s = strconv.FormatInt(int64(typedVal), 10)
			case int64:
				s = strconv.FormatInt(typedVal, 10)
			case float64:
				s = strconv.FormatFloat(typedVal, 'g', -1, 64)
			case uint64:
				s = strconv.FormatUint(typedVal, 10)
			case bool:
				if typedVal {
					s = "true"
				} else {
					s = "false"
				}
			}
			if len(s) > 0 {
				yamlObj = interface{}(s)
			}
		}

		return yamlObj, nil
	}
}
