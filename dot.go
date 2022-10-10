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
			g.AddAttr(g.Name, "ranksep", "0.5")
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
				"label":     fmt.Sprintf(`"%s"`, s.Name),
				"labeljust": `"l"`,
			}
		},
	}
	for _, optFn := range optFns {
		optFn(opts)
	}
	g := gographviz.NewGraph()
	g.AddAttr(graphName, "compound", "true")
	if err := g.SetDir(true); err != nil {
		return "", err
	}
	if err := g.SetName(graphName); err != nil {
		return "", err
	}
	if err := opts.PrepareGraph(g); err != nil {
		return "", err
	}

	terminalNodeAttrs := opts.TerminalNodeAttrs()
	if err := g.AddNode(graphName, `"start"`, terminalNodeAttrs); err != nil {
		return "", err
	}
	if err := g.AddNode(graphName, `"end"`, terminalNodeAttrs); err != nil {
		return "", err
	}
	startNodes, err := top.writeGraphNodes(g, graphName, opts)
	if err != nil {
		return "", err
	}
	edgeAttrs := opts.EdgeAttrs("")
	for _, next := range startNodes[top.StartAt] {
		if err := g.AddEdge(`"start"`, `"`+next+`"`, true, edgeAttrs); err != nil {
			return "", err
		}
	}
	if err := top.writeGraphEdges(g, graphName, []string{"end"}, startNodes, opts); err != nil {
		return "", err
	}
	return g.String(), nil
}

func (top *AmazonStatesLanguage) writeGraphNodes(g *gographviz.Graph, graphName string, opts *MarshalDOTOptions) (map[string][]string, error) {
	if len(top.States) == 0 {
		return nil, errors.New("states not found")
	}
	startNodes := make(map[string][]string)
	for _, state := range top.States {
		stateStartNodes, err := state.writeGraphNodes(g, graphName, opts)
		if err != nil {
			return nil, err
		}
		for key, value := range stateStartNodes {
			startNodes[key] = append(startNodes[key], value...)
		}
	}
	return startNodes, nil
}

func (state *State) writeGraphNodes(g *gographviz.Graph, graphName string, opts *MarshalDOTOptions) (map[string][]string, error) {
	if len(state.Branches) > 0 {
		startNodes := make(map[string][]string)
		subGraphAttrs := opts.BranchesSubGraphAttrs(state)
		if err := g.AddSubGraph(graphName, `"cluster_`+state.Name+`"`, subGraphAttrs); err != nil {
			return nil, err
		}
		for _, branch := range state.Branches {
			branchStartNodes, err := branch.writeGraphNodes(g, `"cluster_`+state.Name+`"`, opts)
			if err != nil {
				return nil, err
			}
			for key, value := range branchStartNodes {
				startNodes[key] = append(startNodes[key], value...)
			}
			startNodes[state.Name] = append(startNodes[state.Name], startNodes[branch.StartAt]...)
		}
		return startNodes, nil
	}
	if state.Iterator != nil {
		subGraphAttrs := opts.IteratorSubGraphAttrs(state)
		label, ok := subGraphAttrs["label"]
		if ok {
			delete(subGraphAttrs, "label")
		}
		if err := g.AddSubGraph(graphName, `"cluster_`+state.Name+`_1"`, subGraphAttrs); err != nil {
			return nil, err
		}
		if err := g.AddSubGraph(`"cluster_`+state.Name+`_1"`, `"cluster_`+state.Name+`_2"`, subGraphAttrs); err != nil {
			return nil, err
		}
		if ok {
			subGraphAttrs["label"] = label
		}
		if err := g.AddSubGraph(`"cluster_`+state.Name+`_2"`, `"cluster_`+state.Name+`_3"`, subGraphAttrs); err != nil {
			return nil, err
		}
		startNodes, err := state.Iterator.writeGraphNodes(g, `"cluster_`+state.Name+`_3"`, opts)
		if err != nil {
			return nil, err
		}
		startNodes[state.Name] = append(startNodes[state.Name], startNodes[state.Iterator.StartAt]...)
		return startNodes, nil
	}
	nodeAttrs := opts.StateNodeAttrs(state)
	if err := g.AddNode(graphName, `"`+state.Name+`"`, nodeAttrs); err != nil {
		return nil, err
	}
	return map[string][]string{state.Name: {state.Name}}, nil
}

func (top *AmazonStatesLanguage) writeGraphEdges(g *gographviz.Graph, graphName string, endNames []string, startNodes map[string][]string, opts *MarshalDOTOptions) error {
	if len(top.States) == 0 {
		return errors.New("states not found")
	}
	for _, state := range top.States {
		if err := state.writeGraphEdges(g, graphName, endNames, startNodes, opts); err != nil {
			return err
		}
	}
	return nil
}

func (state *State) writeGraphEdges(g *gographviz.Graph, graphName string, endNames []string, startNodes map[string][]string, opts *MarshalDOTOptions) error {
	if len(state.Branches) > 0 {
		for _, branch := range state.Branches {
			branchNext := endNames
			if state.Next != nil && *state.Next != "" {
				branchNext = startNodes[*state.Next]
			}
			if err := branch.writeGraphEdges(g, "cluster_"+state.Name, branchNext, startNodes, opts); err != nil {
				return err
			}
		}
		return nil
	}
	if state.Iterator != nil {
		iteratorNext := endNames
		if state.Next != nil && *state.Next != "" {
			iteratorNext = startNodes[*state.Next]
		}
		if err := state.Iterator.writeGraphEdges(g, "cluster_"+state.Name, iteratorNext, startNodes, opts); err != nil {
			return err
		}
		return nil
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
		if err := g.AddEdge(`"`+state.Name+`"`, `"`+next+`"`, true, edgeAttrs); err != nil {
			return err
		}
	}
	if len(nextStates) == 0 || (state.End != nil && *state.End) {
		edgeAttrs := opts.EdgeAttrs("")
		if strings.HasPrefix(graphName, "cluster_") {
			edgeAttrs["ltail"] = `"` + graphName + `"`
		}
		for _, endName := range endNames {
			if err := g.AddEdge(`"`+state.Name+`"`, `"`+endName+`"`, true, edgeAttrs); err != nil {
				return err
			}
		}
	}
	return nil
}
