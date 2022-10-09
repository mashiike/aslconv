package aslconv

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/zclconf/go-cty/cty"
)

type bodySchemaProvider interface {
	provideBodySchema() (*hcl.BodySchema, bool)
}

type variablesEvaluter interface {
	evaluteVariables(body *hcl.BodyContent, ctx *hcl.EvalContext) (map[string]cty.Value, hcl.Diagnostics)
}

func getBodySchema(v interface{}) (*hcl.BodySchema, bool) {
	if provider, ok := v.(bodySchemaProvider); ok {
		return provider.provideBodySchema()
	}
	return gohcl.ImpliedBodySchema(v)
}

func getBodyContent(body hcl.Body, ctx *hcl.EvalContext, v interface{}) (*hcl.BodyContent, hcl.Body, hcl.Diagnostics) {
	schema, partial := getBodySchema(v)
	if partial {
		return body.PartialContent(schema)
	}
	content, diags := body.Content(schema)
	return content, nil, diags
}

func evaluteVariables(body hcl.Body, ctx *hcl.EvalContext, v interface{}) (map[string]cty.Value, hcl.Diagnostics) {
	content, _, diags := getBodyContent(body, ctx, v)
	if diags.HasErrors() {
		return nil, diags
	}
	variables := make(map[string]cty.Value, len(content.Attributes))
	for _, attr := range content.Attributes {
		variables[attr.Name], _ = attr.Expr.Value(ctx)
	}
	if evaluter, ok := v.(variablesEvaluter); ok {
		v, evaluteDiags := evaluter.evaluteVariables(content, ctx)
		diags = append(diags, evaluteDiags...)
		for key, value := range v {
			variables[key] = value
		}
	}
	return variables, diags
}

type bodyUnmarshaler interface {
	unmarshalHCLContent(content *hcl.BodyContent, remain hcl.Body, ctx *hcl.EvalContext) hcl.Diagnostics
}

func unmarshalHCLBody(body hcl.Body, ctx *hcl.EvalContext, v bodyUnmarshaler) hcl.Diagnostics {
	content, remain, diags := getBodyContent(body, ctx, v)
	if diags.HasErrors() {
		return diags
	}
	return v.unmarshalHCLContent(content, remain, ctx)
}

type expressionDecoder interface {
	decodeExpression(expr hcl.Expression, ctx *hcl.EvalContext) hcl.Diagnostics
}

func decodeExpression(expr hcl.Expression, ctx *hcl.EvalContext, v interface{}) hcl.Diagnostics {
	if decoder, ok := v.(expressionDecoder); ok {
		return decoder.decodeExpression(expr, ctx)
	}
	return gohcl.DecodeExpression(expr, ctx, v)
}
