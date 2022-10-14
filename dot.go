package aslconv

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/awalterschulze/gographviz"
)

type MarshalDOTOptions struct {
	PrepareGraph          func(*gographviz.Graph) error
	TerminalNodeAttrs     func() map[string]string
	StateNodeAttrs        func(*State) map[string]string
	EdgeAttrs             func(label string) map[string]string
	ChoiceEdgeAttrs       func(condition map[string]interface{}, i int) map[string]string
	BranchesSubGraphAttrs func(*State) map[string]string
	IteratorSubGraphAttrs func(*State) map[string]string
}

func (top *AmazonStatesLanguage) MarshalDOT(graphName string, optFns ...func(*MarshalDOTOptions)) (string, error) {
	opts := &MarshalDOTOptions{
		PrepareGraph: func(g *gographviz.Graph) error {
			g.AddAttr(g.Name, "ranksep", "0.8")
			g.AddAttr(g.Name, "nodesep", "0.8")
			return nil
		},
		TerminalNodeAttrs: func() map[string]string {
			return map[string]string{
				"shape": `"circle"`,
				"style": `"filled"`,
			}
		},
		StateNodeAttrs: func(_ *State) map[string]string {
			return map[string]string{
				"shape":     `"box"`,
				"style":     `"rounded,dashed"`,
				"fillcolor": `"#00000080"`,
			}
		},
		EdgeAttrs: func(label string) map[string]string {
			attrs := map[string]string{
				"arrowhead": `"vee"`,
			}
			if label != "" {
				attrs["label"] = `"` + label + `"`
			}
			return attrs
		},
		ChoiceEdgeAttrs: func(_ map[string]interface{}, i int) map[string]string {
			attrs := map[string]string{
				"arrowhead": "vee",
				"label":     fmt.Sprintf(`"Rule #%d"`, i+1),
			}
			return attrs
		},
		BranchesSubGraphAttrs: func(s *State) map[string]string {
			return map[string]string{
				"shape":     `"box"`,
				"style":     `"rounded,dashed"`,
				"fillcolor": `"#00000080"`,
				"label":     fmt.Sprintf(`"%s"`, s.Name),
				"labeljust": `"l"`,
			}
		},
		IteratorSubGraphAttrs: func(s *State) map[string]string {
			return map[string]string{
				"shape":     `"box"`,
				"style":     `"dashed"`,
				"fillcolor": `"#00000080"`,
				"label":     fmt.Sprintf(`"%s(iterator)"`, s.Name),
				"labeljust": `"l"`,
			}
		},
	}
	for _, optFn := range optFns {
		optFn(opts)
	}
	g := gographviz.NewGraph()
	if err := g.SetDir(true); err != nil {
		return "", err
	}
	if err := g.SetName(quoteForNode(graphName)); err != nil {
		return "", err
	}
	if err := opts.PrepareGraph(g); err != nil {
		return "", err
	}
	if err := g.AddAttr(quoteForNode(graphName), "compound", "true"); err != nil {
		return "", err
	}
	if err := top.marshalDOT(g, graphName, "start", "end", opts); err != nil {
		return "", err
	}
	g.Edges.Edges = g.Edges.Sorted()
	return g.String(), nil
}
func quoteForNode(str string) string {
	return `"` + str + `"`
}

func (top *AmazonStatesLanguage) marshalDOT(g *gographviz.Graph, graphName string, startName string, endName string, opts *MarshalDOTOptions) error {
	if len(top.States) == 0 {
		return errors.New("states not found")
	}
	terminalNodeAttrs := opts.TerminalNodeAttrs()
	if strings.HasPrefix(graphName, `cluster_`) {
		terminalNodeAttrs["label"] = `""`
	}
	if err := g.AddNode(quoteForNode(graphName), quoteForNode(startName), terminalNodeAttrs); err != nil {
		return err
	}
	if err := g.AddNode(quoteForNode(graphName), quoteForNode(endName), terminalNodeAttrs); err != nil {
		return err
	}

	for _, state := range top.States {
		err := state.marshalDOT(g, graphName, startName, endName, opts)
		if err != nil {
			return err
		}
	}
	_, exists := g.Edges.SrcToDsts[quoteForNode(startName)]
	if exists {
		_, exists = g.Edges.SrcToDsts[quoteForNode(startName)][quoteForNode(top.StartAt)]
	}
	if !exists {
		edgeAttrs := opts.EdgeAttrs("")
		if err := g.AddEdge(quoteForNode(startName), quoteForNode(top.StartAt), true, edgeAttrs); err != nil {
			return err
		}
	}
	return nil
}

func (state *State) marshalDOT(g *gographviz.Graph, graphName string, startName string, endName string, opts *MarshalDOTOptions) error {
	if len(state.Branches) > 0 {
		subGraphAttrs := opts.BranchesSubGraphAttrs(state)
		subGraphName := "cluster_" + state.Name
		if err := g.AddSubGraph(quoteForNode(graphName), quoteForNode(subGraphName), subGraphAttrs); err != nil {
			return err
		}
		for _, branch := range state.Branches {
			err := branch.marshalDOT(g, subGraphName, state.Name, subGraphName+"_end", opts)
			if err != nil {
				return err
			}
		}

		edgeAttrs := opts.EdgeAttrs("")
		edgeAttrs["lhead"] = quoteForNode(subGraphName)
		if err := g.AddEdge(quoteForNode(startName), quoteForNode(state.Name), true, edgeAttrs); err != nil {
			return err
		}
		delete(edgeAttrs, "lhead")
		edgeAttrs["ltail"] = quoteForNode(subGraphName)
		if err := g.AddEdge(`"`+subGraphName+`_end"`, quoteForNode(endName), true, edgeAttrs); err != nil {
			return err
		}
		return nil
	}
	if state.Iterator != nil {
		subGraphAttrs := opts.IteratorSubGraphAttrs(state)
		subGraphName := "cluster_" + state.Name
		if err := g.AddSubGraph(quoteForNode(graphName), quoteForNode(subGraphName), subGraphAttrs); err != nil {
			return err
		}
		err := state.Iterator.marshalDOT(g, subGraphName, state.Name, subGraphName+"_end", opts)
		if err != nil {
			return err
		}
		edgeAttrs := opts.EdgeAttrs("")
		edgeAttrs["lhead"] = quoteForNode(subGraphName)
		if err := g.AddEdge(quoteForNode(startName), quoteForNode(state.Name), true, edgeAttrs); err != nil {
			return err
		}
		delete(edgeAttrs, "lhead")
		edgeAttrs["ltail"] = quoteForNode(subGraphName)
		if err := g.AddEdge(`"`+subGraphName+`_end"`, quoteForNode(endName), true, edgeAttrs); err != nil {
			return err
		}
		return nil
	}
	nodeAttrs := opts.StateNodeAttrs(state)
	if err := g.AddNode(quoteForNode(graphName), quoteForNode(state.Name), nodeAttrs); err != nil {
		return err
	}
	nextStates := make(map[string]map[string]string)
	if state.Next != nil && *state.Next != "" {
		nextStates[*state.Next] = opts.EdgeAttrs("")
	}
	if state.Default != nil && *state.Default != "" {
		nextStates[*state.Default] = opts.EdgeAttrs("default")
	}
	for i, rawMessage := range state.Choices {
		var choice map[string]interface{}
		if err := json.Unmarshal([]byte(rawMessage), &choice); err != nil {
			return err
		}
		if next, ok := choice["Next"].(string); ok {
			nextStates[next] = opts.ChoiceEdgeAttrs(choice, i)

		}
	}
	for next, edgeAttrs := range nextStates {
		if err := g.AddEdge(quoteForNode(state.Name), quoteForNode(next), true, edgeAttrs); err != nil {
			return err
		}
	}
	if len(nextStates) == 0 || (state.End != nil && *state.End) {
		edgeAttrs := opts.EdgeAttrs("")
		if strings.HasPrefix(graphName, "cluster_") {
			edgeAttrs["ltail"] = quoteForNode(graphName)
		}
		if err := g.AddEdge(quoteForNode(state.Name), quoteForNode(endName), true, edgeAttrs); err != nil {
			return err
		}
	}

	return nil
}
