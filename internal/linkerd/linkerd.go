package linkerd

import (
	"context"
	"linkerd-nodegraph/internal/nodegraph"

	"github.com/prometheus/common/model"
)

type graphSource interface {
	Nodes(ctx context.Context) (*[]Node, error)
	Edges(ctx context.Context) (*[]Edge, error)
}

type Stats struct {
	Server graphSource
}

type Parameters struct {
	Depth           int      `schema:"depth"`
	IgnoreResources []string `schema:"ignore_resources"`
	NoOrphans       bool     `schema:"no_orphans"`
	RootResource    string   `schema:"root_resource"`
}

var (
	GraphSpec = nodegraph.NodeFields{
		Edge: []nodegraph.Field{
			{Name: "id", Type: nodegraph.FieldTypeString},
			{Name: "source", Type: nodegraph.FieldTypeString},
			{Name: "target", Type: nodegraph.FieldTypeString},
		},
		Node: []nodegraph.Field{
			{Name: "id", Type: nodegraph.FieldTypeString},
			{Name: "title", Type: nodegraph.FieldTypeString, DisplayName: "Resource"},
			{Name: "mainStat", Type: nodegraph.FieldTypeString, DisplayName: "Success Rate"},
			{Name: "detail__type", Type: nodegraph.FieldTypeString, DisplayName: "Type"},
			{Name: "detail__namespace", Type: nodegraph.FieldTypeString, DisplayName: "Namespace"},
			{Name: "detail__name", Type: nodegraph.FieldTypeString, DisplayName: "Name"},
			{
				Name:        "arc__failed",
				Type:        nodegraph.FieldTypeNumber,
				Color:       "red",
				DisplayName: "Failed",
			},
			{
				Name:        "arc__success",
				Type:        nodegraph.FieldTypeNumber,
				Color:       "green",
				DisplayName: "Success",
			},
		},
	}

	namespaceLabel      = model.LabelName("namespace")
	dstNamespaceLabel   = model.LabelName("dst_namespace")
	deploymentLabel     = model.LabelName("deployment")
	statefulsetLabel    = model.LabelName("statefulset")
	dstDeploymentLabel  = model.LabelName("dst_deployment")
	dstStatefulsetLabel = model.LabelName("dst_statefulset")
)

func (m Stats) Graph(ctx context.Context, parameters Parameters) (*nodegraph.Graph, error) {
	graph := nodegraph.Graph{Spec: GraphSpec}

	nodes, err := m.Server.Nodes(ctx)
	if err != nil {
		return nil, err
	}

	edges, err := m.Server.Edges(ctx)
	if err != nil {
		return nil, err
	}

	err = runFilters(edges, nodes, parameters)
	if err != nil {
		return nil, err
	}

	seenIds := map[string]bool{}

	for _, node := range *nodes {
		nodegraphNode := node.nodegraphNode()

		err := graph.AddNode(nodegraphNode)
		if err != nil {
			return nil, err
		}

		seenIds[node.Resource.id()] = true
	}

	for _, edge := range *edges {
		nographEdge := edge.nodegraphEdge()

		err := graph.AddEdge(nographEdge)
		if err != nil {
			return nil, err
		}

		// if we don't have a node for the source/destination (ie not meshed stuff)
		// add a node for it
		for _, resource := range []Resource{edge.Source, edge.Destination} {
			if _, ok := seenIds[resource.id()]; !ok {
				err := graph.AddNode(Node{Resource: resource}.nodegraphNode())
				if err != nil {
					return nil, err
				}
			}
		}
	}

	return &graph, nil
}

func removeOrphans(edges *[]Edge, nodes *[]Node) {
	seenIds := map[string]bool{}
	newNodes := []Node{}

	for _, edge := range *edges {
		seenIds[edge.Destination.id()] = true
		seenIds[edge.Source.id()] = true
	}

	for _, node := range *nodes {
		if _, ok := seenIds[node.Resource.id()]; ok {
			newNodes = append(newNodes, node)
		}
	}

	*nodes = newNodes
}

func removeId(id string, edges *[]Edge, nodes *[]Node) {
	newNodes := []Node{}
	newEdges := []Edge{}

	for _, node := range *nodes {
		if node.Resource.id() != id {
			newNodes = append(newNodes, node)
		}
	}

	for _, edge := range *edges {
		if edge.Source.id() != id && edge.Destination.id() != id {
			newEdges = append(newEdges, edge)
		}
	}

	*nodes = newNodes
	*edges = newEdges
}

func setRoot(id string, depth int, edges *[]Edge, nodes *[]Node) {
	rootExists := false

	for _, node := range *nodes {
		if node.Resource.id() == id {
			rootExists = true

			break
		}
	}

	if !rootExists {
		return
	}

	currentDepth := 0
	connectedNodeIds := map[string]bool{}
	connectedNodeIds[id] = true

	for currentDepth != depth {
		iterationNodeIds := map[string]bool{}
		currentDepth++

		for root := range connectedNodeIds {
			ids := findNodesConnectedTo(root, *edges)
			for _, id := range ids {
				iterationNodeIds[id] = true
			}
		}

		for id := range iterationNodeIds {
			connectedNodeIds[id] = true
		}
	}

	for _, node := range *nodes {
		if _, ok := connectedNodeIds[node.Resource.id()]; !ok {
			removeId(node.Resource.id(), edges, nodes)
		}
	}
}

func findNodesConnectedTo(id string, edges []Edge) []string {
	nodeIdsMap := map[string]bool{}
	nodeIds := []string{}

	for _, edge := range edges {
		if edge.Source.id() == id {
			if _, ok := nodeIdsMap[edge.Destination.id()]; !ok {
				nodeIdsMap[edge.Destination.id()] = true
			}
		} else if edge.Destination.id() == id {
			if _, ok := nodeIdsMap[edge.Source.id()]; !ok {
				nodeIdsMap[edge.Source.id()] = true
			}
		}
	}

	for k := range nodeIdsMap {
		nodeIds = append(nodeIds, k)
	}

	return nodeIds
}

func runFilters(edges *[]Edge, nodes *[]Node, params Parameters) error {
	if params.NoOrphans {
		removeOrphans(edges, nodes)
	}

	for _, idToIgnore := range params.IgnoreResources {
		removeId(idToIgnore, edges, nodes)
	}

	if params.RootResource != "" {
		depth := 1
		if params.Depth != 0 {
			depth = params.Depth
		}

		setRoot(params.RootResource, depth, edges, nodes)
	}

	return nil
}
