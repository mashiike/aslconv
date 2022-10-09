package aslconv

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agext/levenshtein"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

// https://states-language.net/spec.html#toplevelfields
type AmazonStatesLanguage struct {
	Version        *string `hcl:"version"`
	Comment        *string `hcl:"comment"`
	StartAt        string  `hcl:"start_at"`
	TimeoutSeconds *int64  `hcl:"timeout_seconds"`
	States         States  `hcl:"state,block"`
}

type States []*State

func (top *AmazonStatesLanguage) MarshalJSON() ([]byte, error) {
	data := map[string]interface{}{
		"StartAt": top.StartAt,
	}
	states := make(map[string]json.RawMessage, len(top.States))
	for _, s := range top.States {
		bs, err := json.Marshal(s)
		if err != nil {
			return nil, fmt.Errorf("States[\"%s\"]:%w", s.Name, err)
		}
		states[s.Name] = bs
	}
	data["States"] = states
	if top.Version != nil {
		data["Version"] = *top.Version
	}
	if top.Comment != nil {
		data["Comment"] = top.Comment
	}
	if top.TimeoutSeconds != nil {
		data["TimeSeconds"] = *top.TimeoutSeconds
	}
	return json.Marshal(data)
}

func (top *AmazonStatesLanguage) UnmarshalJSON(bs []byte) error {
	type alieas AmazonStatesLanguage
	data := struct {
		*alieas `json:",inline"`
		States  map[string]*State `json:"States,omitempty"`
	}{
		alieas: (*alieas)(top),
	}
	if err := json.Unmarshal(bs, &data); err != nil {
		return err
	}
	top.States = make([]*State, 0, len(data.States))
	for name, state := range data.States {
		state.Name = name
		top.States = append(top.States, state)
	}
	return nil
}

func (top *AmazonStatesLanguage) DecodeBody(body hcl.Body, ctx *hcl.EvalContext) hcl.Diagnostics {
	variables, diags := evaluteVariables(body, ctx, top)
	if diags.HasErrors() {
		return diags
	}
	ctx = ctx.NewChild()
	ctx.Variables = variables
	for i := 0; !cty.ObjectVal(variables).IsWhollyKnown(); i++ {
		if i >= 100 {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagWarning,
				Summary:  "evalute variables cirkit break",
				Detail:   "pre evalute iterate over 100, may have unknown variables",
			})
			break
		}
		variables, diags = evaluteVariables(body, ctx, top)
		if diags.HasErrors() {
			return diags
		}
		ctx.Variables = variables
	}
	unmarshalDiags := unmarshalHCLBody(body, ctx, top)
	diags = append(diags, unmarshalDiags...)
	return diags
}

func (top *AmazonStatesLanguage) provideBodySchema() (*hcl.BodySchema, bool) {
	schema, partial := gohcl.ImpliedBodySchema(top)
	schema.Blocks = append(schema.Blocks, ([]hcl.BlockHeaderSchema{
		{
			Type: "locals",
		},
	})...)
	return schema, partial
}

func (top *AmazonStatesLanguage) evaluteVariables(content *hcl.BodyContent, ctx *hcl.EvalContext) (map[string]cty.Value, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	variables := make(map[string]map[string]cty.Value)
	locals := make(map[string]cty.Value)
	if parent, ok := ctx.Variables["local"]; ok {
		valueMap := parent.AsValueMap()
		for key, value := range valueMap {
			locals[key] = value
		}
	}
	for _, block := range content.Blocks {
		switch block.Type {
		case "state":
			types, ok := variables[block.Labels[0]]
			if !ok {
				types = make(map[string]cty.Value)
				variables[block.Labels[0]] = types
			}
			types[block.Labels[1]] = cty.StringVal(block.Labels[1])
		case "locals":
			attrs, attrDiags := block.Body.JustAttributes()
			diags = append(diags, attrDiags...)
			for k, attr := range attrs {
				value, _ := attr.Expr.Value(ctx)
				locals[k] = value
			}
		}
	}
	typeVariabls := make(map[string]cty.Value, len(variables))
	for key, value := range variables {
		typeVariabls[key] = cty.ObjectVal(value)
	}
	return map[string]cty.Value{
		"state": cty.ObjectVal(typeVariabls),
		"local": cty.ObjectVal(locals),
	}, nil
}

func (top *AmazonStatesLanguage) unmarshalHCLContent(content *hcl.BodyContent, _ hcl.Body, ctx *hcl.EvalContext) hcl.Diagnostics {
	var diags hcl.Diagnostics
	typeMap := map[string]string{
		"task":     "Task",
		"choice":   "Choice",
		"fail":     "Fail",
		"parallel": "Parallel",
		"map":      "Map",
		"succeed":  "Succeed",
		"wait":     "Wait",
		"pass":     "Pass",
	}
	typeList := make([]string, 0, len(typeMap))
	for t := range typeMap {
		typeList = append(typeList, t)
	}
	stateRange := make(map[string]*hcl.Range, len(content.Blocks))
	for _, block := range content.Blocks {
		switch block.Type {
		case "state":
			var state State
			label := block.Labels[0]
			t, ok := typeMap[label]
			if !ok {
				var isSuggested bool
				for _, suggestion := range typeList {
					dist := levenshtein.Distance(label, suggestion, nil)
					if dist < 3 {
						diags = append(diags, &hcl.Diagnostic{
							Severity: hcl.DiagError,
							Summary:  "Invalid state type",
							Detail:   fmt.Sprintf(`The state type "%s" is invalid. Did you mean "%s"?`, label, suggestion),
							Subject:  block.DefRange.Ptr(),
						})
						isSuggested = true
						break
					}
				}
				if !isSuggested {
					diags = append(diags, &hcl.Diagnostic{
						Severity: hcl.DiagError,
						Summary:  "Invalid state type",
						Detail:   fmt.Sprintf(`The state type "%s" is invalid.  [%s and %s] can be used for the state type`, label, strings.Join(typeList[:len(typeList)-1], ","), typeList[len(typeList)-1]),
						Subject:  block.DefRange.Ptr(),
					})
				}
				continue
			}
			state.Type = t
			state.Name = block.Labels[1]
			if r, ok := stateRange[state.Name]; ok {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  `Duplicate "state" name`,
					Detail:   fmt.Sprintf(`A state named "%s" was already declared at %s. State names must unique`, state.Name, r.String()),
					Subject:  block.DefRange.Ptr(),
				})
			} else {
				stateRange[state.Name] = block.DefRange.Ptr()
			}
			diags = append(diags, unmarshalHCLBody(block.Body, ctx, &state)...)
			top.States = append(top.States, &state)
		}
	}
	for _, attr := range content.Attributes {
		switch attr.Name {
		case "comment":
			decodeDiags := decodeExpression(attr.Expr, ctx, &top.Comment)
			diags = append(diags, decodeDiags...)
		case "version":
			decodeDiags := decodeExpression(attr.Expr, ctx, &top.Version)
			diags = append(diags, decodeDiags...)
		case "timeout_seconds":
			decodeDiags := decodeExpression(attr.Expr, ctx, &top.TimeoutSeconds)
			diags = append(diags, decodeDiags...)
		case "start_at":
			decodeDiags := decodeExpression(attr.Expr, ctx, &top.StartAt)
			diags = append(diags, decodeDiags...)
		}
	}
	return diags
}

func (top *AmazonStatesLanguage) EncodeBody(body *hclwrite.Body) error {
	if top.Version != nil {
		body.SetAttributeValue("version", cty.StringVal(*top.Version))
	}
	if top.Comment != nil {
		body.SetAttributeValue("comment", cty.StringVal(*top.Comment))
	}
	if top.TimeoutSeconds != nil {
		body.SetAttributeValue("timeout_seconds", cty.NumberIntVal(*top.TimeoutSeconds))
	}
	startAtTraversal, err := top.States.getTraversal(top.StartAt)
	if err != nil {
		return fmt.Errorf("start_at:%w", err)
	}
	body.SetAttributeTraversal("start_at", startAtTraversal)
	for _, state := range top.States {
		block, err := state.EncodeAsBlock(top.States)
		if err != nil {
			return fmt.Errorf("%s:%w", state.Name, err)
		}
		body.AppendNewline()
		body.AppendBlock(block)
	}
	return nil
}

func (states States) getTraversal(stateName string) (hcl.Traversal, error) {
	for _, state := range states {
		if state.Name == stateName {
			return hcl.Traversal{
				hcl.TraverseRoot{
					Name: "state",
				},
				hcl.TraverseAttr{
					Name: strings.ToLower(state.Type),
				},
				hcl.TraverseAttr{
					Name: state.Name,
				},
			}, nil
		}
	}
	return nil, fmt.Errorf("state `%s` not found", stateName)
}

type State struct {
	Type           string                  `json:"Type,omitempty" hcl:"type,label"`
	Name           string                  `json:"-" hcl:"name,label"`
	Comment        *string                 `json:"Comment,omitempty" hcl:"comment"`
	Resource       *string                 `json:"Resource,omitempty" hcl:"resource"`
	Default        *string                 `json:"Default,omitempty" hcl:"default"`
	Seconds        *int64                  `json:"Seconds,omitempty" hcl:"seconds"`
	MaxConcurrency *int64                  `json:"MaxConcurrency,omitempty" hcl:"max_concurrency"`
	Next           *string                 `json:"Next,omitempty" hcl:"next"`
	ItemsPath      *string                 `json:"ItemsPath,omitempty" hcl:"items_path"`
	InputPath      *string                 `json:"InputPath,omitempty" hcl:"input_path"`
	OutputPath     *string                 `json:"OutputPath,omitempty" hcl:"output_path"`
	ResultPath     *string                 `json:"ResultPath,omitempty" hcl:"result_path"`
	End            *bool                   `json:"End,omitempty" hcl:"end"`
	Error          *string                 `json:"Error,omitempty" hcl:"error"`
	Cause          *string                 `json:"Cause,omitempty" hcl:"cause"`
	Retry          RawMessages             `json:"Retry,omitempty" hcl:"retry,optional"`
	Catch          RawMessages             `json:"Catch,omitempty" hcl:"catch,optional"`
	Parameters     RawMessage              `json:"Parameters,omitempty" hcl:"parameters,optional"`
	Result         RawMessage              `json:"Result,omitempty" hcl:"result,optional"`
	ResultSelector RawMessage              `json:"ResultSelector,omitempty" hcl:"result_selector,optional"`
	Choices        RawMessages             `json:"Choices,omitempty" hcl:"choices,optional"`
	Branches       []*AmazonStatesLanguage `json:"Branches,omitempty" hcl:"branch,block"`
	Iterator       *AmazonStatesLanguage   `json:"Iterator,omitempty" hcl:"iterator,block"`
}

func (state *State) unmarshalHCLContent(content *hcl.BodyContent, _ hcl.Body, ctx *hcl.EvalContext) hcl.Diagnostics {
	var diags hcl.Diagnostics
	for _, attr := range content.Attributes {
		switch attr.Name {
		case "comment":
			decodeDiags := decodeExpression(attr.Expr, ctx, &state.Comment)
			diags = append(diags, decodeDiags...)
		case "resource":
			decodeDiags := decodeExpression(attr.Expr, ctx, &state.Resource)
			diags = append(diags, decodeDiags...)
		case "default":
			decodeDiags := decodeExpression(attr.Expr, ctx, &state.Default)
			diags = append(diags, decodeDiags...)
		case "next":
			decodeDiags := decodeExpression(attr.Expr, ctx, &state.Next)
			diags = append(diags, decodeDiags...)
		case "end":
			decodeDiags := decodeExpression(attr.Expr, ctx, &state.End)
			diags = append(diags, decodeDiags...)
		case "max_concurrency":
			decodeDiags := decodeExpression(attr.Expr, ctx, &state.MaxConcurrency)
			diags = append(diags, decodeDiags...)
		case "items_path":
			decodeDiags := decodeExpression(attr.Expr, ctx, &state.ItemsPath)
			diags = append(diags, decodeDiags...)
		case "input_path":
			decodeDiags := decodeExpression(attr.Expr, ctx, &state.InputPath)
			diags = append(diags, decodeDiags...)
		case "output_path":
			decodeDiags := decodeExpression(attr.Expr, ctx, &state.OutputPath)
			diags = append(diags, decodeDiags...)
		case "result_path":
			decodeDiags := decodeExpression(attr.Expr, ctx, &state.ResultPath)
			diags = append(diags, decodeDiags...)
		case "result":
			decodeDiags := decodeExpression(attr.Expr, ctx, &state.Result)
			diags = append(diags, decodeDiags...)
		case "result_selector":
			decodeDiags := decodeExpression(attr.Expr, ctx, &state.ResultSelector)
			diags = append(diags, decodeDiags...)
		case "error":
			decodeDiags := decodeExpression(attr.Expr, ctx, &state.Error)
			diags = append(diags, decodeDiags...)
		case "cause":
			decodeDiags := decodeExpression(attr.Expr, ctx, &state.Cause)
			diags = append(diags, decodeDiags...)
		case "retry":
			decodeDiags := decodeExpression(attr.Expr, ctx, &state.Retry)
			diags = append(diags, decodeDiags...)
		case "catch":
			decodeDiags := decodeExpression(attr.Expr, ctx, &state.Catch)
			diags = append(diags, decodeDiags...)
		case "choices":
			decodeDiags := decodeExpression(attr.Expr, ctx, &state.Choices)
			diags = append(diags, decodeDiags...)
		case "parameters":
			decodeDiags := decodeExpression(attr.Expr, ctx, &state.Parameters)
			diags = append(diags, decodeDiags...)
		case "seconds":
			decodeDiags := decodeExpression(attr.Expr, ctx, &state.Seconds)
			diags = append(diags, decodeDiags...)
		}
	}
	var iteratorRange *hcl.Range
	for _, block := range content.Blocks {
		switch block.Type {
		case "branch":
			var asl AmazonStatesLanguage
			decodeDiags := asl.DecodeBody(block.Body, ctx.NewChild())
			diags = append(diags, decodeDiags...)
			state.Branches = append(state.Branches, &asl)
		case "iterator":
			if iteratorRange != nil {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  `Duplicate "iterator" block`,
					Detail:   fmt.Sprintf(`Only one "iterator" block is allowed. Another was defined at %s`, iteratorRange.String()),
					Subject:  block.DefRange.Ptr(),
				})
				continue
			}
			var asl AmazonStatesLanguage
			decodeDiags := asl.DecodeBody(block.Body, ctx.NewChild())
			diags = append(diags, decodeDiags...)
			state.Iterator = &asl
			iteratorRange = block.DefRange.Ptr()
		}
	}
	return diags
}

func (state *State) EncodeAsBlock(states States) (*hclwrite.Block, error) {
	cloned := *state
	cloned.Branches = nil
	cloned.Iterator = nil
	block := gohcl.EncodeAsBlock(&cloned, "state")
	block.SetLabels([]string{strings.ToLower(state.Type), state.Name})
	body := block.Body()
	if state.Default != nil {
		traversal, err := states.getTraversal(*state.Default)
		if err != nil {
			return nil, err
		}
		body.SetAttributeTraversal("default", traversal)
	}
	if state.Next != nil {
		traversal, err := states.getTraversal(*state.Next)
		if err != nil {
			return nil, err
		}
		body.SetAttributeTraversal("next", traversal)
	}
	if state.Parameters != nil {
		bs, err := json.Marshal(state.Parameters)
		if err != nil {
			return nil, fmt.Errorf("parameter:%w", err)
		}
		body.SetAttributeValue("parameters", cty.StringVal(string(bs)))
	} else {
		body.RemoveAttribute("parameters")
	}
	if state.Result != nil {
		bs, err := json.Marshal(state.Result)
		if err != nil {
			return nil, fmt.Errorf("result:%w", err)
		}
		body.SetAttributeValue("result", cty.StringVal(string(bs)))
	} else {
		body.RemoveAttribute("result")
	}
	if state.ResultSelector != nil {
		bs, err := json.Marshal(state.ResultSelector)
		if err != nil {
			return nil, fmt.Errorf("result_selector:%w", err)
		}
		body.SetAttributeValue("result_selector", cty.StringVal(string(bs)))
	} else {
		body.RemoveAttribute("result_selector")
	}
	if len(state.Retry) > 0 {
		values := make([]cty.Value, 0, len(state.Choices))
		for i, c := range state.Retry {
			bs, err := json.Marshal(c)
			if err != nil {
				return nil, fmt.Errorf("retry[%d]:%w", i, err)
			}
			values = append(values, cty.StringVal(string(bs)))
		}
		body.SetAttributeValue("retry", cty.ListVal(values))
	} else {
		body.RemoveAttribute("retry")
	}
	if len(state.Catch) > 0 {
		values := make([]cty.Value, 0, len(state.Choices))
		for i, c := range state.Catch {
			bs, err := json.Marshal(c)
			if err != nil {
				return nil, fmt.Errorf("catch[%d]:%w", i, err)
			}
			values = append(values, cty.StringVal(string(bs)))
		}
		body.SetAttributeValue("catch", cty.ListVal(values))
	} else {
		body.RemoveAttribute("catch")
	}
	if len(state.Choices) > 0 {
		values := make([]cty.Value, 0, len(state.Choices))
		for i, c := range state.Choices {
			bs, err := json.Marshal(c)
			if err != nil {
				return nil, fmt.Errorf("choices[%d]:%w", i, err)
			}
			values = append(values, cty.StringVal(string(bs)))
		}
		body.SetAttributeValue("choices", cty.ListVal(values))
	} else {
		body.RemoveAttribute("choices")
	}
	if len(state.Branches) > 0 {
		for _, branch := range state.Branches {
			body.AppendNewline()
			branchBlock := body.AppendNewBlock("branch", []string{})
			if err := branch.EncodeBody(branchBlock.Body()); err != nil {
				return nil, err
			}
		}
	}
	if state.Iterator != nil {
		body.AppendNewline()
		iteratorhBlock := body.AppendNewBlock("iterator", []string{})
		if err := state.Iterator.EncodeBody(iteratorhBlock.Body()); err != nil {
			return nil, err
		}
	}
	return block, nil
}

type RawMessage json.RawMessage

func (m RawMessage) MarshalJSON() ([]byte, error) {
	return json.RawMessage(m).MarshalJSON()
}

func (m *RawMessage) UnmarshalJSON(data []byte) error {
	var raw json.RawMessage
	if err := raw.UnmarshalJSON(data); err != nil {
		return err
	}
	*m = RawMessage(raw)
	return nil
}

func (m *RawMessage) decodeExpression(expr hcl.Expression, ctx *hcl.EvalContext) hcl.Diagnostics {
	var raw string
	diags := decodeExpression(expr, ctx, &raw)
	*m = RawMessage(raw)
	return diags
}

type RawMessages []RawMessage

func (ms *RawMessages) decodeExpression(expr hcl.Expression, ctx *hcl.EvalContext) hcl.Diagnostics {
	exprList, diags := hcl.ExprList(expr)
	*ms = make([]RawMessage, len(exprList))
	for i, e := range exprList {
		decodeDiags := decodeExpression(e, ctx, &(*ms)[i])
		diags = append(diags, decodeDiags...)
	}
	return diags
}
