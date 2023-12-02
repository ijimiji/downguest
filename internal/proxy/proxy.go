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

func calculateLevels(nodes []models.Node, edges []models.Edge) map[string]int {
	levels := make(map[string]int)
	visited := make(map[string]bool)
	var dfs func(string, int)
	dfs = func(nodeName string, level int) {
		if visited[nodeName] {
			return
		}
		visited[nodeName] = true
		if level > levels[nodeName] {
			levels[nodeName] = level
		}
		for _, edge := range edges {
			if edge.Source == nodeName {
				dfs(edge.Destination, level+1)
			}
		}
	}
	for _, node := range nodes {
		if _, exists := visited[node.Name]; !exists {
			dfs(node.Name, 0)
		}
	}
	return levels
}

func (p *proxy) HandleGraph(w http.ResponseWriter, r *http.Request) {
	graph := p.graph

	levels := calculateLevels(graph.Nodes, graph.Edges)
	maxLevel := 0
	for _, l := range levels {
		if l > maxLevel {
			maxLevel = l
		}
	}

	w.Header().Set("Content-Type", "image/svg+xml")
	width := 800
	height := 600

	canvas := svg.New(w)
	canvas.Start(width, height)
	canvas.Title(graph.Name)

	canvas.Def()
	canvas.Marker("arrow", 10, 10, 10, 10, "orient:auto")
	canvas.Path("M0,0 L10,5 L0,10 z", "fill:black")
	canvas.MarkerEnd()
	canvas.DefEnd()

	xSpacing := width / (maxLevel + 1)
	nodeCoords := make(map[string][2]int)

	// Сначала проинициализируем координаты узлов
	for level := 0; level <= maxLevel; level++ {
		nodesInLevel := make([]models.Node, 0)
		for _, node := range graph.Nodes {
			if levels[node.Name] == level {
				nodesInLevel = append(nodesInLevel, node)
			}
		}
		ySpacing := height / (len(nodesInLevel) + 1)
		for i, node := range nodesInLevel {
			x := (level + 1) * xSpacing
			y := (i + 1) * ySpacing
			nodeCoords[node.Name] = [2]int{x, y}
		}
	}

	// Рисуем линии
	nodeRadius := 20

	for _, edge := range graph.Edges {
		srcCoords := nodeCoords[edge.Source]
		dstCoords := nodeCoords[edge.Destination]

		if slices.Equal(srcCoords[:], []int{0, 0}) || slices.Equal(dstCoords[:], []int{0, 0}) {
			continue
		}

		canvas.Line(srcCoords[0], srcCoords[1], dstCoords[0], dstCoords[1], "stroke:black;marker-end:url(#arrow)")
		midX := (srcCoords[0] + dstCoords[0]) / 2
		midY := (srcCoords[1] + dstCoords[1]) / 2

		text := getNodeByName(edge.Source, p.graph.Nodes).Output
		textWidth := len(text) * 4 // Подбор числа для оценки ширины текста
		textHeight := 10           // Высота текста, может потребовать подгонки

		canvas.Rect(midX-textWidth/2, midY-textHeight/2-2, textWidth, textHeight, fmt.Sprintf("fill:%s;stroke:black;stroke-width:1", randomSecondaryColour()))
		canvas.Text(midX, midY, text, fmt.Sprintf("text-anchor:middle;font-size:%dpx;fill:black", textHeight-4))
	}

	// Рисуем узлы поверх линий и метки рёбер
	for _, node := range graph.Nodes {
		x := nodeCoords[node.Name][0]
		y := nodeCoords[node.Name][1]

		canvas.Circle(x, y, nodeRadius, fmt.Sprintf("fill:%s;stroke:black;stroke-width:1", randomColour()))
		canvas.Text(x, y+5, node.Name, "text-anchor:middle;font-size:11px;fill:black")
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

func randomColour() string {
	colors := []string{
		"#bf616a",
		"#ebcb8b",
		"#ebcb8b",
		"#a3be8c",
		"#81a1c1",
	}
	return colors[rand.Intn(len(colors))]
}

func randomSecondaryColour() string {
	colors := []string{
		"#88c0d0",
	}
	return colors[rand.Intn(len(colors))]
}
