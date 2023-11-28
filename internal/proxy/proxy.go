package proxy

import (
	"downguest/internal/models"
	dproto "downguest/proto"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"slices"

	svg "github.com/ajstarks/svgo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func New(graph models.Graph) (*proxy, error) {
	clients := make(map[string]*grpc.ClientConn, len(graph.Nodes))
	for _, node := range graph.Nodes {
		conn, err := grpc.Dial(node.Host, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return nil, err
		}
		clients[node.Name] = conn
	}

	return &proxy{
		graph:   graph,
		clients: clients,
	}, nil
}

type proxy struct {
	graph   models.Graph
	clients map[string]*grpc.ClientConn
}

func (p *proxy) Serve(addr string) error {
	defer func() {
		for _, conn := range p.clients {
			conn.Close()
		}
	}()
	http.HandleFunc("/", p.HandleRequest)
	http.HandleFunc("/graph", p.HandleGraph)

	return http.ListenAndServe(addr, nil)
}

func (p *proxy) HandleGraph(w http.ResponseWriter, r *http.Request) {
	graph := p.graph

	w.Header().Set("Content-Type", "image/svg+xml")

	width := 800
	height := 600

	canvas := svg.New(w)
	canvas.Start(width, height)
	canvas.Grid(0, 0, width, height, 10, "stroke:gray;opacity:0.8;stroke-width:0.5")
	canvas.Title(graph.Name)

	// Draw nodes
	nodeRadius := 20
	nodeCoords := make(map[string][2]int)

	for i, node := range graph.Nodes {
		x := (i + 1) * 100

		sign := []int{-1, 1}
		rand.Shuffle(2, func(i, j int) {
			sign[i], sign[j] = sign[j], sign[i]
		})

		y := height/2 + (rand.Int()%(height/4))*sign[0]

		nodeCoords[node.Name] = [2]int{x, y}

		canvas.Circle(x, y, nodeRadius, "fill:#ebcb8b")
		canvas.Text(x, y, node.Name, "text-anchor:middle;font-size:11px;fill:black")
	}

	// Draw edges
	for _, edge := range graph.Edges {
		if srcCoords, ok := nodeCoords[edge.Source]; ok {
			if dstCoords, ok := nodeCoords[edge.Destination]; ok {
				canvas.Line(srcCoords[0], srcCoords[1], dstCoords[0], dstCoords[1], "stroke:black")
				midX := (srcCoords[0] + dstCoords[0]) / 2
				midY := (srcCoords[1] + dstCoords[1]) / 2
				canvas.Text(midX, midY, "+", "text-anchor:middle;font-size:11px;fill:black")
			}
		}
	}

	canvas.End()
}

func (p *proxy) HandleRequest(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic encountered", r)
		}
	}()
	ctx := r.Context()

	visited := map[string]struct{}{}
	originNode, _ := p.origin(p.graph)
	routeContext := map[string]*anypb.Any{}

	var httpRequest dproto.THTTPRequest
	var response dproto.TContext
	if err := p.clients[originNode.Name].Invoke(ctx, fmt.Sprintf("/proto.echo.%s/Serve", originNode.Name), &httpRequest, &response); err != nil {
		slog.Error(err.Error())
		return
	}

	visited[originNode.Name] = struct{}{}
	routeContext[originNode.Output] = response.Data["http_response"]

	sorted := TopologicalSort(p.graph)
	for _, node := range sorted {
		if _, ok := visited[node.Name]; ok {
			continue
		}

		var request dproto.TContext
		request.Data = routeContext

		var response dproto.TContext
		if err := p.clients[node.Name].Invoke(ctx, fmt.Sprintf("/%s/Serve", node.Name), &request, &response); err != nil {
			return
		}

		for key, value := range response.Data {
			routeContext[key] = value
		}

		visited[node.Name] = struct{}{}
	}

	var body dproto.THTTPResponse
	if err := routeContext["http_response"].UnmarshalTo(&body); err != nil {
		slog.Error(err.Error())
	}

	w.Write([]byte(body.GetBody()))
}

func (p *proxy) origin(graph models.Graph) (models.Node, models.Edge) {
	nodes := graph.Nodes
	edges := graph.Edges

	var retNode models.Node
	var retEdge models.Edge

	for _, node := range nodes {
		if slices.Contains(node.Inputs, "http_request") {
			retNode = node
		}
	}

	for _, edge := range edges {
		if edge.Source == "origin_http_request" && edge.Destination == retNode.Name {
			retEdge = edge
		}
	}

	return retNode, retEdge
}

func TopologicalSort(graph models.Graph) []models.Node {
	inDegree := make(map[string]int)
	for _, edge := range graph.Edges {
		inDegree[edge.Destination]++
	}

	stack := make([]string, 0)
	for _, node := range graph.Nodes {
		if inDegree[node.Name] == 0 {
			stack = append(stack, node.Name)
		}
	}

	result := make([]models.Node, 0)
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		result = append(result, getNodeByName(node, graph.Nodes))

		for _, edge := range graph.Edges {
			if edge.Source == node {
				inDegree[edge.Destination]--
				if inDegree[edge.Destination] == 0 {
					stack = append(stack, edge.Destination)
				}
			}
		}
	}

	return result
}

func getNodeByName(name string, nodes []models.Node) models.Node {
	for _, node := range nodes {
		if node.Name == name {
			return node
		}
	}
	return models.Node{}
}

func ConvertToAnyMap(inputMap map[string]proto.Message) (map[string]*anypb.Any, error) {
	outputMap := make(map[string]*anypb.Any)

	for key, value := range inputMap {
		anyValue, err := anypb.New(value)
		if err != nil {
			return nil, err
		}

		outputMap[key] = anyValue
	}

	return outputMap, nil
}
