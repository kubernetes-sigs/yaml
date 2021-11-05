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
	"fmt"
	"math"
	"reflect"
	"strconv"
	"testing"

	"gopkg.in/yaml.v3"
)

/* Test helper functions */

func strPtr(str string) *string {
	return &str
}

type errorType int

const (
	noErrorsType     errorType = 0
	strictErrorsType errorType = 1 << iota
	fatalErrorsType
	strictAndFatalErrorsType errorType = strictErrorsType | fatalErrorsType
)

type unmarshalTestCase struct {
	encoded    []byte
	decodeInto interface{}
	decoded    interface{}
	err        errorType
}

type testUnmarshalFunc = func(yamlBytes []byte, obj interface{}) (strictErrors []error, err error)

var (
	funcUnmarshal testUnmarshalFunc = func(yamlBytes []byte, obj interface{}) (strictErrors []error, err error) {
		err = Unmarshal(yamlBytes, obj)
		return []error{}, err
	}

	funcUnmarshalStrict testUnmarshalFunc = func(yamlBytes []byte, obj interface{}) (strictErrors []error, err error) {
		return UnmarshalStrict(yamlBytes, obj)
	}
)

func testUnmarshal(t *testing.T, f testUnmarshalFunc, tests map[string]unmarshalTestCase) {
	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			typ := reflect.TypeOf(test.decodeInto)
			if typ.Kind() != reflect.Ptr {
				t.Errorf("unmarshalTest.ptr %T is not a pointer type", test.decodeInto)
			}

			value := reflect.New(typ.Elem())

			if !reflect.DeepEqual(test.decodeInto, value.Interface()) {
				// There's no reason for ptr to point to non-zero data,
				// as we decode into new(right-type), so the data is
				// discarded.
				// This can easily mean tests that silently don't test
				// what they should. To test decoding into existing
				// data, see TestPrefilled.
				t.Errorf("unmarshalTest.ptr %#v is not a pointer to a zero value", test.decodeInto)
			}

			strictErrors, err := f(test.encoded, value.Interface())
			if err != nil && test.err&fatalErrorsType == 0 {
				t.Errorf("unexpected fatal error, unmarshaling YAML: %v", err)
			}
			if len(strictErrors) > 0 && test.err&strictErrorsType == 0 {
				t.Errorf("unexpected strict error, unmarshaling YAML: %v", strictErrors)
			}
			if err == nil && test.err&fatalErrorsType != 0 {
				t.Errorf("expected a fatal error, but no fatal error was returned, yaml: `%s`", test.encoded)
			}
			if len(strictErrors) == 0 && test.err&strictErrorsType != 0 {
				t.Errorf("expected strict errors, but no strict error was returned, yaml: `%s`", test.encoded)
			}

			if test.err&fatalErrorsType != 0 {
				// Don't check output if error is fatal
				return
			}

			if !reflect.DeepEqual(value.Elem().Interface(), test.decoded) {
				t.Errorf("unmarshal YAML was unsuccessful, expected: %+#v, got: %+#v", test.decoded, value.Elem().Interface())
			}
		})
	}
}

type yamlToJSONTestcase struct {
	yaml string
	json string
	// By default we test that reversing the output == input. But if there is a
	// difference in the reversed output, you can optionally specify it here.
	yamlReverseOverwrite *string
	err                  errorType
}

type testYAMLToJSONFunc = func(yamlBytes []byte) (json []byte, strictErrors []error, err error)

var (
	funcYAMLToJSON testYAMLToJSONFunc = func(yamlBytes []byte) (json []byte, strictErrors []error, err error) {
		json, err = YAMLToJSON(yamlBytes)
		return json, []error{}, err
	}
)

func testYAMLToJSON(t *testing.T, f testYAMLToJSONFunc, tests map[string]yamlToJSONTestcase) {
	for testName, test := range tests {
		t.Run(fmt.Sprintf("%s_YAMLToJSON", testName), func(t *testing.T) {
			// Convert Yaml to Json
			jsonBytes, strictErrors, err := f([]byte(test.yaml))
			if err != nil && test.err&fatalErrorsType == 0 {
				t.Errorf("unexpected fatal error, convert YAML to JSON, yaml: `%s`, err: %v", test.yaml, err)
			}
			if len(strictErrors) > 0 && test.err&strictErrorsType == 0 {
				t.Errorf("unexpected strict error, convert YAML to JSON, yaml: `%s`, err: %v", test.yaml, strictErrors)
			}
			if err == nil && test.err&fatalErrorsType != 0 {
				t.Errorf("expected a fatal error, but no fatal error was returned, yaml: `%s`", test.yaml)
			}
			if len(strictErrors) == 0 && test.err&strictErrorsType != 0 {
				t.Errorf("expected strict errors, but no strict error was returned, yaml: `%s`", test.yaml)
			}

			if test.err&fatalErrorsType != 0 {
				// Don't check output if error is fatal
				return
			}

			// Check it against the expected output.
			if string(jsonBytes) != test.json {
				t.Errorf("Failed to convert YAML to JSON, yaml: `%s`, expected json `%s`, got `%s`", test.yaml, test.json, string(jsonBytes))
			}
		})

		t.Run(fmt.Sprintf("%s_JSONToYAML", testName), func(t *testing.T) {
			// Convert JSON to YAML
			yamlBytes, err := JSONToYAML([]byte(test.json))
			if err != nil {
				t.Errorf("Failed to convert JSON to YAML, json: `%s`, err: %v", test.json, err)
			}

			// Set the string that we will compare the reversed output to.
			correctYamlString := test.yaml

			// If a special reverse string was specified, use that instead.
			if test.yamlReverseOverwrite != nil {
				correctYamlString = *test.yamlReverseOverwrite
			}

			// Check it against the expected output.
			if string(yamlBytes) != correctYamlString {
				t.Errorf("Failed to convert JSON to YAML, json: `%s`, expected yaml `%s`, got `%s`", test.json, correctYamlString, string(yamlBytes))
			}
		})
	}
}

/* Start tests */

type MarshalTest struct {
	A string
	B int64
	C float64
}

func TestMarshal(t *testing.T) {
	f64String := strconv.FormatFloat(math.MaxFloat64, 'g', -1, 64)
	s := MarshalTest{"a", math.MaxInt64, math.MaxFloat64}
	e := []byte(fmt.Sprintf("A: a\nB: %d\nC: %s\n", math.MaxInt64, f64String))

	y, err := Marshal(s)
	if err != nil {
		t.Errorf("error marshaling YAML: %v", err)
	}

	if !reflect.DeepEqual(y, e) {
		t.Errorf("marshal YAML was unsuccessful, expected: %#v, got: %#v",
			string(e), string(y))
	}
}

type MarshalTestArray struct {
	A []int
}

func TestMarshalArray(t *testing.T) {
	arr := MarshalTestArray{
		A: []int{1, 2, 3},
	}
	e := []byte("A:\n- 1\n- 2\n- 3\n")

	y, err := Marshal(arr)
	if err != nil {
		t.Errorf("error marshaling YAML: %v", err)
	}

	if !reflect.DeepEqual(y, e) {
		t.Errorf("marshal YAML was unsuccessful, expected: %#v, got: %#v",
			string(e), string(y))
	}
}

type UnmarshalUntaggedStruct struct {
	A    string
	True string
}

type UnmarshalTaggedStruct struct {
	AUpper            string `json:"A"`
	ALower            string `json:"a"`
	TrueUpper         string `json:"True"`
	TrueLower         string `json:"true"`
	YesUpper          string `json:"Yes"`
	YesLower          string `json:"yes"`
	Int3              string `json:"3"`
	IntBig1           string `json:"9007199254740993"` // 2^53 + 1
	IntBig2           string `json:"1000000000000000000000000000000000000"`
	IntBig2Scientific string `json:"1e+36"`
	Float3dot3        string `json:"3.3"`
}

type UnmarshalStruct struct {
	A string  `json:"a"`
	B *string `json:"b"`
	C string  `json:"c"`
}

type UnmarshalStringMap struct {
	A map[string]string `json:"a"`
}

type UnmarshalNestedStruct struct {
	A UnmarshalStruct `json:"a"`
}

type UnmarshalSlice struct {
	A []UnmarshalStruct `json:"a"`
}

type UnmarshalEmbedStruct struct {
	UnmarshalStruct
	B string `json:"b"`
}

type UnmarshalEmbedStructPointer struct {
	*UnmarshalStruct
	B string `json:"b"`
}

type UnmarshalEmbedRecursiveStruct struct {
	*UnmarshalEmbedRecursiveStruct `json:"a"`
	B                              string `json:"b"`
}

func TestUnmarshal(t *testing.T) {
	tests := map[string]unmarshalTestCase{
		// casematched untagged keys
		"untagged casematched string key": {
			encoded:    []byte("A: test"),
			decodeInto: new(UnmarshalUntaggedStruct),
			decoded:    UnmarshalUntaggedStruct{A: "test"},
		},

		// casematched / non-casematched tagged keys
		"tagged casematched string key": {
			encoded:    []byte("A: test"),
			decodeInto: new(UnmarshalTaggedStruct),
			decoded:    UnmarshalTaggedStruct{AUpper: "test"},
		},
		"tagged non-casematched string key": {
			encoded:    []byte("a: test"),
			decodeInto: new(UnmarshalTaggedStruct),
			decoded:    UnmarshalTaggedStruct{ALower: "test"},
		},
		"tagged casematched boolean key": {
			encoded:    []byte("True: test"),
			decodeInto: new(UnmarshalTaggedStruct),
			decoded:    UnmarshalTaggedStruct{TrueLower: "test"},
		},
		"tagged non-casematched boolean key": {
			encoded:    []byte("true: test"),
			decodeInto: new(UnmarshalTaggedStruct),
			decoded:    UnmarshalTaggedStruct{TrueLower: "test"},
		},
		"tagged casematched boolean key (yes)": {
			encoded:    []byte("Yes: test"),
			decodeInto: new(UnmarshalTaggedStruct),
			decoded:    UnmarshalTaggedStruct{YesUpper: "test"},
		},
		"tagged non-casematched boolean key (yes)": {
			encoded:    []byte("yes: test"),
			decodeInto: new(UnmarshalTaggedStruct),
			decoded:    UnmarshalTaggedStruct{YesLower: "test"},
		},
		"tagged integer key": {
			encoded:    []byte("3: test"),
			decodeInto: new(UnmarshalTaggedStruct),
			decoded:    UnmarshalTaggedStruct{Int3: "test"},
		},
		"tagged big integer key 2^53 + 1": {
			encoded:    []byte("9007199254740993: test"),
			decodeInto: new(UnmarshalTaggedStruct),
			decoded:    UnmarshalTaggedStruct{IntBig1: "test"},
		},
		"tagged big integer key 1000000000000000000000000000000000000": {
			encoded:    []byte("1000000000000000000000000000000000000: test"),
			decodeInto: new(UnmarshalTaggedStruct),
			decoded:    UnmarshalTaggedStruct{IntBig2Scientific: "test"},
		},
		"tagged float key": {
			encoded:    []byte("3.3: test"),
			decodeInto: new(UnmarshalTaggedStruct),
			decoded:    UnmarshalTaggedStruct{Float3dot3: "test"},
		},

		// decode into string field
		"string value into string field": {
			encoded:    []byte("a: test"),
			decodeInto: new(UnmarshalStruct),
			decoded:    UnmarshalStruct{A: "test"},
		},
		"integer value into string field": {
			encoded:    []byte("a: 1"),
			decodeInto: new(UnmarshalStruct),
			decoded:    UnmarshalStruct{A: "1"},
		},
		"boolean value into string field": {
			encoded:    []byte("a: true"),
			decodeInto: new(UnmarshalStruct),
			decoded:    UnmarshalStruct{A: "true"},
		},
		"boolean value (no) into string field": {
			encoded:    []byte("a: no"),
			decodeInto: new(UnmarshalStruct),
			decoded:    UnmarshalStruct{A: "no"},
		},

		// decode into complex fields
		"decode into nested struct": {
			encoded:    []byte("a:\n  a: 1"),
			decodeInto: new(UnmarshalNestedStruct),
			decoded:    UnmarshalNestedStruct{UnmarshalStruct{A: "1"}},
		},
		"decode into slice": {
			encoded:    []byte("a:\n  - a: abc\n    b: def\n  - a: 123"),
			decodeInto: new(UnmarshalSlice),
			decoded:    UnmarshalSlice{[]UnmarshalStruct{{A: "abc", B: strPtr("def")}, {A: "123"}}},
		},
		"decode into string map": {
			encoded:    []byte("a:\n  b: 1"),
			decodeInto: new(UnmarshalStringMap),
			decoded:    UnmarshalStringMap{map[string]string{"b": "1"}},
		},
		"decode into struct pointer map": {
			encoded:    []byte("a:\n  a: TestA\nb:\n  a: TestB\n  b: TestC"),
			decodeInto: new(map[string]*UnmarshalStruct),
			decoded: map[string]*UnmarshalStruct{
				"a": {A: "TestA"},
				"b": {A: "TestB", B: strPtr("TestC")},
			},
		},

		// decoding into string map
		"string map: decode string key": {
			encoded:    []byte("a:"),
			decodeInto: new(map[string]struct{}),
			decoded: map[string]struct{}{
				"a": {},
			},
		},
		"string map: decode boolean key": {
			encoded:    []byte("True:"),
			decodeInto: new(map[string]struct{}),
			decoded: map[string]struct{}{
				"true": {},
			},
		},
		"string map: decode boolean key (yes)": {
			encoded:    []byte("Yes:"),
			decodeInto: new(map[string]struct{}),
			decoded: map[string]struct{}{
				"Yes": {},
			},
		},
		"string map: decode integer key": {
			encoded:    []byte("44:"),
			decodeInto: new(map[string]struct{}),
			decoded: map[string]struct{}{
				"44": {},
			},
		},
		"string map: decode float key": {
			encoded:    []byte("444.444:"),
			decodeInto: new(map[string]struct{}),
			decoded: map[string]struct{}{
				"444.444": {},
			},
		},

		// decoding integers
		"decode 2^53 + 1 into int": {
			encoded:    []byte("9007199254740993"),
			decodeInto: new(int),
			decoded:    9007199254740993,
		},
		"decode 2^53 + 1 into interface": {
			encoded:    []byte("9007199254740993"),
			decodeInto: new(interface{}),
			decoded:    int64(9007199254740993),
		},

		// decode into interface
		"float into interface": {
			encoded:    []byte("3.0"),
			decodeInto: new(interface{}),
			decoded:    int64(3),
		},
		"integer into interface": {
			encoded:    []byte("3"),
			decodeInto: new(interface{}),
			decoded:    int64(3),
		},
		"empty vs empty string into interface": {
			encoded:    []byte("a: \"\"\nb: \n"),
			decodeInto: new(interface{}),
			decoded: map[string]interface{}{
				"a": "",
				"b": nil,
			},
		},

		// decoding embeded structs
		"decode embeded struct": {
			encoded:    []byte("a: testA\nb: testB"),
			decodeInto: new(UnmarshalEmbedStruct),
			decoded: UnmarshalEmbedStruct{
				UnmarshalStruct: UnmarshalStruct{
					A: "testA",
				},
				B: "testB",
			},
		},
		"decode embeded structpointer": {
			encoded:    []byte("a: testA\nb: testB"),
			decodeInto: new(UnmarshalEmbedStructPointer),
			decoded: UnmarshalEmbedStructPointer{
				UnmarshalStruct: &UnmarshalStruct{
					A: "testA",
				},
				B: "testB",
			},
		},
		"decode recursive embeded structpointer": {
			encoded:    []byte("b: testB\na:\n  b: testA"),
			decodeInto: new(UnmarshalEmbedRecursiveStruct),
			decoded: UnmarshalEmbedRecursiveStruct{
				UnmarshalEmbedRecursiveStruct: &UnmarshalEmbedRecursiveStruct{
					B: "testA",
				},
				B: "testB",
			},
		},
		"decode embeded struct and cast integer to string": {
			encoded:    []byte("a: 11\nb: testB"),
			decodeInto: new(UnmarshalEmbedStruct),
			decoded: UnmarshalEmbedStruct{
				UnmarshalStruct: UnmarshalStruct{
					A: "11",
				},
				B: "testB",
			},
		},
		"decode embeded structpointer and cast integer to string": {
			encoded:    []byte("a: 11\nb: testB"),
			decodeInto: new(UnmarshalEmbedStructPointer),
			decoded: UnmarshalEmbedStructPointer{
				UnmarshalStruct: &UnmarshalStruct{
					A: "11",
				},
				B: "testB",
			},
		},

		// decoding into incompatible type
		"decode into stringmap with incompatible type": {
			encoded:    []byte("a:\n  a:\n    a: 3"),
			decodeInto: new(UnmarshalStringMap),
			err:        fatalErrorsType,
		},

		// decoding with duplicate values
		"decode into struct pointer map with duplicate string value": {
			encoded:    []byte("a:\n  a: TestA\n  b: ID-A\n  b: ID-1"),
			decodeInto: new(map[string]*UnmarshalStruct),
			decoded: map[string]*UnmarshalStruct{
				"a": {A: "TestA", B: strPtr("ID-1")},
			},
			err: fatalErrorsType,
		},
		"decode into string field with duplicate boolean value": {
			encoded:    []byte("a: true\na: false"),
			decodeInto: new(UnmarshalStruct),
			decoded:    UnmarshalStruct{A: "false"},
			err:        fatalErrorsType,
		},
		"decode into slice with duplicate string-boolean value": {
			encoded:    []byte("a:\n- b: abc\n  a: 32\n  b: 123"),
			decodeInto: new(UnmarshalSlice),
			decoded:    UnmarshalSlice{[]UnmarshalStruct{{A: "32", B: strPtr("123")}}},
			err:        fatalErrorsType,
		},

		// decoding with duplicate complex values
		"decode duplicate into nested struct": {
			encoded:    []byte("a:\n  a: 1\na:\n  a: 2"),
			decodeInto: new(UnmarshalNestedStruct),
			decoded:    UnmarshalNestedStruct{A: UnmarshalStruct{A: "2"}},
			err:        fatalErrorsType,
		},
		"decode duplicate into nested slice": {
			encoded:    []byte("a:\n  - a: abc\n    b: def\na:\n  - a: 123"),
			decodeInto: new(UnmarshalSlice),
			decoded:    UnmarshalSlice{[]UnmarshalStruct{{A: "123"}}},
			err:        fatalErrorsType,
		},
		"decode duplicate into nested string map": {
			encoded:    []byte("a:\n  b: 1\na:\n  c: 1"),
			decodeInto: new(UnmarshalStringMap),
			decoded:    UnmarshalStringMap{map[string]string{"c": "1"}},
			err:        fatalErrorsType,
		},
		"decode duplicate into string map": {
			encoded:    []byte("a: test\nb: test\na: test2"),
			decodeInto: new(map[string]string),
			decoded: map[string]string{
				"a": "test2",
				"b": "test",
			},
			err: fatalErrorsType,
		},

		// duplicate (non-casematched) keys
		"decode duplicate (non-casematched) into string map": {
			encoded:    []byte("a: test\nb: test\nA: test2"),
			decodeInto: new(map[string]string),
			decoded: map[string]string{
				"a": "test",
				"A": "test2",
				"b": "test",
			},
		},
	}

	t.Run("Unmarshal", func(t *testing.T) {
		testUnmarshal(t, funcUnmarshal, tests)
	})

	t.Run("UnmarshalStrict", func(t *testing.T) {
		testUnmarshal(t, funcUnmarshalStrict, tests)
	})
}

func TestUnmarshalStrictFails(t *testing.T) {
	tests := map[string]unmarshalTestCase{
		// non-casematched untagged keys
		"untagged non-casematched string key": {
			encoded:    []byte("a: test"),
			decodeInto: new(UnmarshalUntaggedStruct),
			decoded:    UnmarshalUntaggedStruct{},
		},
		"untagged casematched boolean key": {
			encoded:    []byte("True: test"),
			decodeInto: new(UnmarshalUntaggedStruct),
			decoded:    UnmarshalUntaggedStruct{}, // BUG: because True is a boolean, it is converted to the string "true" which does not match the fieldname
		},
		"untagged non-casematched boolean key": {
			encoded:    []byte("true: test"),
			decodeInto: new(UnmarshalUntaggedStruct),
			decoded:    UnmarshalUntaggedStruct{},
		},

		// duplicate (non-casematched) keys -> cause unknown field errors
		"decode duplicate (non-casematched) into nested struct 1": {
			encoded:    []byte("a:\n  a: 1\n  b: 1\n  c: test\n\nA:\n  a: 2"),
			decodeInto: new(UnmarshalNestedStruct),
			decoded:    UnmarshalNestedStruct{A: UnmarshalStruct{A: "1", B: strPtr("1"), C: "test"}},
		},
		"decode duplicate (non-casematched) into nested struct 2": {
			encoded:    []byte("A:\n  a: 1\n  b: 1\n  c: test\na:\n  a: 2"),
			decodeInto: new(UnmarshalNestedStruct),
			decoded:    UnmarshalNestedStruct{A: UnmarshalStruct{A: "2"}},
		},
		"decode duplicate (non-casematched) into nested slice 1": {
			encoded:    []byte("a:\n  - a: abc\n    b: def\nA:\n  - a: 123"),
			decodeInto: new(UnmarshalSlice),
			decoded:    UnmarshalSlice{[]UnmarshalStruct{{A: "abc", B: strPtr("def")}}},
		},
		"decode duplicate (non-casematched) into nested slice 2": {
			encoded:    []byte("A:\n  - a: abc\n    b: def\na:\n  - a: 123"),
			decodeInto: new(UnmarshalSlice),
			decoded:    UnmarshalSlice{[]UnmarshalStruct{{A: "123"}}},
		},
		"decode duplicate (non-casematched) into nested string map 1": {
			encoded:    []byte("a:\n  b: 1\nA:\n  c: 1"),
			decodeInto: new(UnmarshalStringMap),
			decoded:    UnmarshalStringMap{map[string]string{"b": "1"}},
		},
		"decode duplicate (non-casematched) into nested string map 2": {
			encoded:    []byte("A:\n  b: 1\na:\n  c: 1"),
			decodeInto: new(UnmarshalStringMap),
			decoded:    UnmarshalStringMap{map[string]string{"c": "1"}},
		},

		// decoding with unknown fields
		"decode into struct with unknown field": {
			encoded:    []byte("a: TestB\nb: ID-B\nunknown: Some-Value"),
			decodeInto: new(UnmarshalStruct),
			decoded:    UnmarshalStruct{A: "TestB", B: strPtr("ID-B")},
		},
	}

	t.Run("Unmarshal", func(t *testing.T) {
		testUnmarshal(t, funcUnmarshal, tests)
	})

	t.Run("UnmarshalStrict", func(t *testing.T) {
		failTests := map[string]unmarshalTestCase{}
		for name, test := range tests {
			test.err |= strictErrorsType
			failTests[name] = test
		}
		testUnmarshal(t, funcUnmarshalStrict, failTests)
	})
}

func TestYAMLToJSON(t *testing.T) {
	tests := map[string]yamlToJSONTestcase{
		"string value": {
			yaml: "t: a\n",
			json: `{"t":"a"}`,
		},
		"null value": {
			yaml: "t: null\n",
			json: `{"t":null}`,
		},
		"boolean value": {
			yaml:                 "t: True\n",
			json:                 `{"t":true}`,
			yamlReverseOverwrite: strPtr("t: true\n"),
		},
		"boolean value (no)": {
			yaml:                 "t: no\n",
			json:                 `{"t":"no"}`,
			yamlReverseOverwrite: strPtr("t: \"no\"\n"),
		},
		"integer value (2^53 + 1)": {
			yaml:                 "t: 9007199254740993\n",
			json:                 `{"t":9007199254740993}`,
			yamlReverseOverwrite: strPtr("t: 9007199254740993\n"),
		},
		"integer value (1000000000000000000000000000000000000)": {
			yaml:                 "t: 1000000000000000000000000000000000000\n",
			json:                 `{"t":1e+36}`,
			yamlReverseOverwrite: strPtr("t: 1e+36\n"),
		},
		"line-wrapped string value": {
			yaml: "t: this is very long line with spaces and it must be longer than 80 so we will repeat\n  that it must be longer that 80\n",
			json: `{"t":"this is very long line with spaces and it must be longer than 80 so we will repeat that it must be longer that 80"}`,
		},
		"empty yaml value": {
			yaml:                 "t: ",
			json:                 `{"t":null}`,
			yamlReverseOverwrite: strPtr("t: null\n"),
		},
		"boolean key": {
			yaml:                 "True: a",
			json:                 `{"true":"a"}`,
			yamlReverseOverwrite: strPtr("\"true\": a\n"),
		},
		"boolean key (no)": {
			yaml:                 "no: a",
			json:                 `{"no":"a"}`,
			yamlReverseOverwrite: strPtr("\"no\": a\n"),
		},
		"integer key": {
			yaml:                 "1: a",
			json:                 `{"1":"a"}`,
			yamlReverseOverwrite: strPtr("\"1\": a\n"),
		},
		"float key": {
			yaml:                 "1.2: a",
			json:                 `{"1.2":"a"}`,
			yamlReverseOverwrite: strPtr("\"1.2\": a\n"),
		},
		"large integer key": {
			yaml:                 "1000000000000000000000000000000000000: a",
			json:                 `{"1e+36":"a"}`,
			yamlReverseOverwrite: strPtr("\"1e+36\": a\n"),
		},
		"large integer key (scientific notation)": {
			yaml:                 "1e+36: a",
			json:                 `{"1e+36":"a"}`,
			yamlReverseOverwrite: strPtr("\"1e+36\": a\n"),
		},
		"string key (large integer as string)": {
			yaml: "\"1e+36\": a\n",
			json: `{"1e+36":"a"}`,
		},
		"string key (float as string)": {
			yaml: "\"1.2\": a\n",
			json: `{"1.2":"a"}`,
		},
		"array": {
			yaml: "- t: a\n",
			json: `[{"t":"a"}]`,
		},
		"nested struct array": {
			yaml: "- t: a\n- t:\n    b: 1\n    c: 2\n",
			json: `[{"t":"a"},{"t":{"b":1,"c":2}}]`,
		},
		"nested struct array (json notation)": {
			yaml:                 `[{t: a}, {t: {b: 1, c: 2}}]`,
			json:                 `[{"t":"a"},{"t":{"b":1,"c":2}}]`,
			yamlReverseOverwrite: strPtr("- t: a\n- t:\n    b: 1\n    c: 2\n"),
		},
		"empty struct value": {
			yaml:                 "- t: ",
			json:                 `[{"t":null}]`,
			yamlReverseOverwrite: strPtr("- t: null\n"),
		},
		"null struct value": {
			yaml: "- t: null\n",
			json: `[{"t":null}]`,
		},
		"binary data": {
			yaml:                 "a: !!binary gIGC",
			json:                 `{"a":"\ufffd\ufffd\ufffd"}`,
			yamlReverseOverwrite: strPtr("a: \ufffd\ufffd\ufffd\n"),
		},

		// Cases that should produce errors.
		"~ key": {
			yaml:                 "~: a",
			json:                 `{"null":"a"}`,
			yamlReverseOverwrite: strPtr("\"null\": a\n"),
			err:                  fatalErrorsType,
		},
		"null key": {
			yaml:                 "null: a",
			json:                 `{"null":"a"}`,
			yamlReverseOverwrite: strPtr("\"null\": a\n"),
			err:                  fatalErrorsType,
		},

		// expect YAMLtoJSON to fail on duplicate field names
		"duplicate struct value": {
			yaml:                 "foo: bar\nfoo: baz\n",
			json:                 `{"foo":"baz"}`,
			yamlReverseOverwrite: strPtr("foo: baz\n"),
			err:                  fatalErrorsType,
		},
	}

	t.Run("YAMLToJSON", func(t *testing.T) {
		testYAMLToJSON(t, funcYAMLToJSON, tests)
	})
}

func TestYamlNode(t *testing.T) {
	var yamlNode yaml.Node
	data := []byte("a:\n    b: test\n    c:\n        - t:\n")

	if err := yaml.Unmarshal(data, &yamlNode); err != nil {
		t.Error(err)
	}

	dataOut, err := yaml.Marshal(&yamlNode)
	if err != nil {
		t.Error(err)
	}

	if string(data) != string(dataOut) {
		t.Errorf("yaml.Node roudtrip failed: expected yaml `%s`, got `%s`", string(data), string(dataOut))
	}
}
