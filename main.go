package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/maurik77/wspipe/connection"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func main() {
	destination := flag.String("s", "http://localhost:9222", "http destination")
	port := flag.Int("p", 8082, "listen port")
	role := flag.String("r", "client", "role: client\\server")
	wsUrl := flag.String("w", "", "websocket server url")

	flag.Parse()

	log.Println("Port:", *port, "Role:", *role, "WsUrl:", *wsUrl)

	connectionManager := connection.New(*role)

	if *role == connection.ClientRole {
		manageClient(wsUrl, connectionManager, destination)
	} else {
		manageServer(connectionManager, destination)
	}

	//The base url used to route all requests
	http.HandleFunc("/route/", requestHandler(connectionManager))
	listen := fmt.Sprintf(":%v", *port)
	http.ListenAndServe(listen, nil)
}

func manageServer(connectionManager *connection.Manager, destination *string) {
	http.HandleFunc("/ws/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Received websocket connection request: %v", r.URL)
		connectionId, ok := getConnectionId(r)
		log.Printf("Connection established. Connection Id: %v", *connectionId)

		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Missing connection id"))
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)

		if err != nil {
			log.Println(err)
			return
		}

		connectionManager.AddConnectionToPool(destination, conn, *connectionId)
	})
}

func manageClient(wsUrl *string, connectionManager *connection.Manager, destination *string) {
	uuid := uuid.NewString()
	wsTargetUrl := fmt.Sprintf("%v/%v", *wsUrl, uuid)
	log.Printf("Connection establishing with url: %v", wsTargetUrl)
	conn, _, err := websocket.DefaultDialer.Dial(wsTargetUrl, nil)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Connection established")
	connectionManager.AddConnectionToPool(destination, conn, uuid)
}

func getConnectionId(r *http.Request) (*string, bool) {
	uriSegments := strings.Split(r.URL.Path, "/")

	if len(uriSegments) > 1 {
		return &uriSegments[2], true
	}

	return nil, false
}

func requestHandler(conn *connection.Manager) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Http request: %v %v", r.Method, r.RequestURI)

		var connectionID *string = nil

		if conn.Role == connection.ServerRole {
			connID, ok := getConnectionId(r)

			if !ok {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("Missing connection id"))
				return
			}

			connectionID = connID
		}

		response, err := conn.SendRequest(r, connectionID, 10*time.Second)

		if err != nil {
			log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(fmt.Sprint(err)))
		} else {
			writeResponse(w, response)
		}
	}
}

func writeResponse(w http.ResponseWriter, res *http.Response) {
	// Write the status code
	w.WriteHeader(res.StatusCode)

	// Write all headers
	for key, values := range res.Header {
		for _, value := range values {
			w.Header().Set(key, value)
		}
	}

	// Write the response body
	bodyContent, err := io.ReadAll(res.Body)
	if err != nil {
		log.Println(err)
	} else {
		w.Write(bodyContent)
	}
}
