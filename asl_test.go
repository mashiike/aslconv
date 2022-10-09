package aslconv_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/mashiike/aslconv"
	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"
)

func ptr[T any](t T) *T {
	return &t
}

func requireASLEq(t *testing.T, expected *aslconv.AmazonStatesLanguage, actual *aslconv.AmazonStatesLanguage) {
	t.Helper()
	diff := cmp.Diff(
		expected, actual,
		cmpopts.SortSlices(func(x, y *aslconv.State) bool {
			return x.Name < y.Name
		}),
		cmpopts.AcyclicTransformer("RawJSON", func(data aslconv.RawMessage) interface{} {
			var v interface{}
			json.Unmarshal(data, &v)
			return v
		}),
	)
	if diff != "" {
		require.FailNow(t, diff)
	}
}

var sampleASL = &aslconv.AmazonStatesLanguage{
	Comment: ptr("An example of the Amazon States Language using a choice state."),
	StartAt: "FirstState",
	States: []*aslconv.State{
		{
			Name:     "FirstState",
			Type:     "Task",
			Resource: ptr("arn:aws:lambda:us-east-1:123456789012:function:FUNCTION_NAME"),
			Next:     ptr("ChoiceState"),
		},
		{
			Name: "ChoiceState",
			Type: "Choice",
			Choices: []aslconv.RawMessage{
				aslconv.RawMessage(`{
					"Variable": "$.foo",
					"NumericEquals": 1,
					"Next": "FirstMatchState"
				}`),
				aslconv.RawMessage(`{
					"Variable": "$.foo",
					"NumericEquals": 2,
					"Next": "SecondMatchState"
				  }`),
			},
			Default: ptr("DefaultState"),
		},
		{
			Name:     "FirstMatchState",
			Type:     "Task",
			Resource: ptr("arn:aws:lambda:us-east-1:123456789012:function:OnFirstMatch"),
			Next:     ptr("NextState"),
		},
		{
			Name:     "SecondMatchState",
			Type:     "Task",
			Resource: ptr("arn:aws:lambda:us-east-1:123456789012:function:OnSecondMatch"),
			Next:     ptr("NextState"),
		},
		{
			Name:  "DefaultState",
			Type:  "Fail",
			Error: ptr("DefaultStateError"),
			Cause: ptr("No Matches!"),
		},
		{
			Name:     "NextState",
			Type:     "Task",
			Resource: ptr("arn:aws:lambda:us-east-1:123456789012:function:FUNCTION_NAME"),
			End:      ptr(true),
		},
	},
}

var parallelASL = &aslconv.AmazonStatesLanguage{
	Comment: ptr("Parallel Example."),
	StartAt: "LookupCustomerInfo",
	States: aslconv.States{
		{
			Name: "LookupCustomerInfo",
			Type: "Parallel",
			End:  ptr(true),
			Branches: []*aslconv.AmazonStatesLanguage{
				{
					StartAt: "LookupAddress",
					States: aslconv.States{
						{
							Name:     "LookupAddress",
							Type:     "Task",
							Resource: ptr("arn:aws:lambda:us-east-1:123456789012:function:AddressFinder"),
							End:      ptr(true),
						},
					},
				},
				{
					StartAt: "LookupPhone",
					States: aslconv.States{
						{
							Name:     "LookupPhone",
							Type:     "Task",
							Resource: ptr("arn:aws:lambda:us-east-1:123456789012:function:PhoneFinder"),
							End:      ptr(true),
						},
					},
				},
			},
		},
	},
}

var othersASL = &aslconv.AmazonStatesLanguage{
	Comment: ptr("An example of the Amazon States Language using a map state."),
	StartAt: "Validate-All",
	States: aslconv.States{
		{
			Name:           "Validate-All",
			Type:           "Map",
			InputPath:      ptr("$.detail"),
			ItemsPath:      ptr("$.shipped"),
			MaxConcurrency: ptr(int64(0)),
			Iterator: &aslconv.AmazonStatesLanguage{
				StartAt: "Validate",
				States: aslconv.States{
					{
						Name:           "Validate",
						Type:           "Task",
						Resource:       ptr("arn:aws:lambda:us-east-1:123456789012:function:ship-val"),
						Parameters:     aslconv.RawMessage(`{"input.$": "$"}`),
						ResultSelector: aslconv.RawMessage(`{"data.$": "$"}`),
						OutputPath:     ptr("$.items"),
						Retry: []aslconv.RawMessage{
							aslconv.RawMessage(`{
								"ErrorEquals": [ "ErrorA", "ErrorB" ],
								"IntervalSeconds": 1,
								"BackoffRate": 2,
								"MaxAttempts": 2
							}`),
							aslconv.RawMessage(`{
								"ErrorEquals": [ "ErrorC" ],
								"IntervalSeconds": 5
							}`),
						},
						Catch: []aslconv.RawMessage{
							aslconv.RawMessage(`{
								"ErrorEquals": [ "States.ALL" ],
								"Next": "Z"
							}`),
						},
						Next: ptr("Wait"),
					},
					{
						Name:    "Wait",
						Type:    "Wait",
						Seconds: ptr(int64(10)),
						Next:    ptr("Pass"),
					},
					{
						Name:       "Pass",
						Type:       "Pass",
						Result:     aslconv.RawMessage(`{"x-datum": 0.381018,"y-datum": 622.2269926397355}`),
						ResultPath: ptr("$.coords"),
						Next:       ptr("Success"),
					},
					{
						Name: "Success",
						Type: "Succeed",
					},
				},
			},
			ResultPath: ptr("$.detail.shipped"),
			End:        ptr(true),
		},
	},
}

func TestMarshalJSON(t *testing.T) {
	cases := []struct {
		casename string
		asl      *aslconv.AmazonStatesLanguage
		expected string
	}{
		{
			casename: "sample",
			asl:      sampleASL,
			expected: "testdata/sample.asl.json",
		},
		{
			casename: "parallel",
			asl:      parallelASL,
			expected: "testdata/parallel.asl.json",
		},
		{
			casename: "others",
			asl:      othersASL,
			expected: "testdata/others.asl.json",
		},
	}
	for _, c := range cases {
		t.Run(c.casename, func(t *testing.T) {
			actual, err := json.MarshalIndent(c.asl, "", "  ")
			t.Log(string(actual))
			require.NoError(t, err)
			bs, err := os.ReadFile(c.expected)
			require.NoError(t, err)
			require.JSONEq(t, string(bs), string(actual))
		})
	}
}

func TestUnmarshalJSON(t *testing.T) {
	cases := []struct {
		casename string
		source   string
		expected *aslconv.AmazonStatesLanguage
	}{
		{
			casename: "sample",
			source:   "testdata/sample.asl.json",
			expected: sampleASL,
		},
		{
			casename: "parallel",
			source:   "testdata/parallel.asl.json",
			expected: parallelASL,
		},
		{
			casename: "others",
			source:   "testdata/others.asl.json",
			expected: othersASL,
		},
	}
	for _, c := range cases {
		t.Run(c.casename, func(t *testing.T) {

			bs, err := os.ReadFile(c.source)
			require.NoError(t, err)
			var actual aslconv.AmazonStatesLanguage
			err = json.Unmarshal(bs, &actual)

			require.NoError(t, err)
			requireASLEq(t, c.expected, &actual)
		})
	}
}

func requireNoHasErrors(t *testing.T, files map[string]*hcl.File, diags hcl.Diagnostics) {
	t.Helper()
	if !diags.HasErrors() {
		return
	}
	var builder strings.Builder
	writer := hcl.NewDiagnosticTextWriter(&builder, files, 400, false)
	writer.WriteDiagnostics(diags)
	t.Log(builder.String())
	require.FailNow(t, "diagnotics has errors")
}

func TestDecodeBody(t *testing.T) {
	cases := []struct {
		casename string
		source   string
		expected *aslconv.AmazonStatesLanguage
	}{
		{
			casename: "sample",
			source:   "testdata/sample.asl.hcl",
			expected: sampleASL,
		},
		{
			casename: "use_locals",
			source:   "testdata/advanced.asl.hcl",
			expected: sampleASL,
		},
		{
			casename: "parallel",
			source:   "testdata/parallel.asl.hcl",
			expected: parallelASL,
		},
		{
			casename: "others",
			source:   "testdata/others.asl.hcl",
			expected: othersASL,
		},
	}
	for _, c := range cases {
		t.Run(c.casename, func(t *testing.T) {
			parser := hclparse.NewParser()
			file, diags := parser.ParseHCLFile(c.source)
			requireNoHasErrors(t, parser.Files(), diags)
			var actual aslconv.AmazonStatesLanguage
			ctx := &hcl.EvalContext{
				Functions: map[string]function.Function{
					"jsonencode": stdlib.JSONEncodeFunc,
				},
			}
			diags = actual.DecodeBody(file.Body, ctx)

			requireNoHasErrors(t, parser.Files(), diags)
			requireASLEq(t, c.expected, &actual)
		})
	}
}

func TestEncodeBody(t *testing.T) {
	cases := []struct {
		casename string
		source   *aslconv.AmazonStatesLanguage
		expected string
	}{
		{
			casename: "sample",
			source:   sampleASL,
			expected: "testdata/sample.asl.hcl",
		},
		{
			casename: "parallel",
			source:   parallelASL,
			expected: "testdata/parallel.asl.hcl",
		},
		{
			casename: "others",
			source:   othersASL,
			expected: "testdata/others.asl.hcl",
		},
	}
	for _, c := range cases {
		t.Run(c.casename, func(t *testing.T) {
			f := hclwrite.NewEmptyFile()
			err := c.source.EncodeBody(f.Body())
			require.NoError(t, err)
			actual := string(f.Bytes())
			bs, err := os.ReadFile(c.expected)
			require.NoError(t, err)
			expected := string(bs)
			t.Logf("actual: \n%s", actual)
			if expected != actual {
				dmp := diffmatchpatch.New()
				a, b, c := dmp.DiffLinesToChars(expected, actual)
				diffs := dmp.DiffMain(a, b, false)
				diffs = dmp.DiffCharsToLines(diffs, c)

				t.Errorf("mismatch output:\n\n%s", dmp.DiffPrettyText(diffs))
			}
		})
	}
}
