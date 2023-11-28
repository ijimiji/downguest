package main

import (
	"downguest/internal/models"
	"downguest/internal/proxy"
	"encoding/json"
	"flag"
	"log"
	"log/slog"
)

func main() {
	port := flag.String("port", "8080", "downguest port")
	flag.Parse()

	var graph models.Graph
	if err := json.Unmarshal([]byte(`{
		"name": "test",
		"nodes": [
			{
				"name": "Echo",
				"inputs": ["http_request"],
				"output": "http_response",
				"host": "localhost:8081"
			},
			{
				"name": "Foo",
				"inputs": ["http_request"],
				"output": "http_response",
				"host": "localhost:8081"
			},
			{
				"name": "Bar",
				"inputs": ["http_request"],
				"output": "http_response",
				"host": "localhost:8081"
			},
			{
				"name": "Spam",
				"inputs": ["http_request"],
				"output": "http_response",
				"host": "localhost:8081"
			},
			{
				"name": "Bobik",
				"inputs": ["http_request"],
				"output": "http_response",
				"host": "localhost:8081"
			},
			{
				"name": "Zhopa",
				"inputs": ["http_request"],
				"output": "http_response",
				"host": "localhost:8081"
			},
			{
				"name": "Hui",
				"inputs": ["http_request"],
				"output": "http_response",
				"host": "localhost:8081"
			}
		],
		"edges": [
			{
				"source": "origin_http_request",
				"destination": "Echo"
			},
			{
				"source": "Bobik",
				"destination": "origin_http_response"
			},
			{
				"source": "Spam",
				"destination": "Bobik"
			},
			{
				"source": "Bar",
				"destination": "Spam"
			},
			{
				"source": "Foo",
				"destination": "Spam"
			},
			{
				"source": "Hui",
				"destination": "Zhopa"
			},
			{
				"source": "Zhopa",
				"destination": "Bobik"
			},
			{
				"source": "Echo",
				"destination": "Hui"
			},
			{
				"source": "Echo",
				"destination": "Foo"
			},
			{
				"source": "Echo",
				"destination": "Bar"
			}

		]
	}`), &graph); err != nil {
		slog.Error("bad graph")
		return
	}

	proxy, err := proxy.New(graph)
	if err != nil {

	}

	log.Fatal(proxy.Serve(":" + *port))

	// servant, err := servant.New(servant.Config{
	// 	Port: *port,
	// })
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// log.Fatal(servant.Serve())
}
