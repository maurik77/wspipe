package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/maurik77/wspipe/connection"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func main() {
	setup()
	destination := flag.String("s", "http://localhost:9222", "http destination")
	port := flag.Int("p", 8082, "listen port")
	role := flag.String("r", "client", "role: client\\server")
	wsURL := flag.String("w", "", "websocket server url")
	debug := flag.Bool("debug", true, "sets log level to debug")

	flag.Parse()
	log.Debug().Msgf("port: %v, role: %v, wsUrl: %v, destination: %v, debug: %v", *port, *role, *wsURL, *destination, debug)

	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if *debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	connectionManager := connection.New(*role)

	if *role == connection.ClientRole {
		manageClient(wsURL, connectionManager, destination)
	} else {
		manageServer(connectionManager, destination)
	}

	// The base url used to route all requests
	http.HandleFunc("/route/", routingHandler(connectionManager))
	listen := fmt.Sprintf(":%v", *port)

	server := &http.Server{
		Addr:              listen,
		ReadHeaderTimeout: 3 * time.Second,
	}

	err := server.ListenAndServe()

	if err != nil {
		log.Err(err).Msg("ListenAndServe")
	}
}

func manageServer(connectionManager *connection.Manager, destination *string) {
	http.HandleFunc("/ws/", func(w http.ResponseWriter, r *http.Request) {
		log.Debug().Msgf("Received websocket connection request: %v", r.URL)
		connectionID, ok := getConnectionID(r)
		log.Debug().Msgf("Connection established. Connection Id: %v", *connectionID)

		if !ok {
			writeMissingConnectionID(w)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)

		if err != nil {
			log.Err(err).Msg("Upgrade error")
			return
		}

		connectionManager.AddConnectionToPool(destination, conn, *connectionID)
	})
}

func manageClient(wsURL *string, connectionManager *connection.Manager, destination *string) {
	uuid := uuid.NewString()
	wsTargetURL := fmt.Sprintf("%v/%v", *wsURL, uuid)
	log.Debug().Msgf("Connection establishing with url: %v", wsTargetURL)

	conn, _, err := websocket.DefaultDialer.Dial(wsTargetURL, nil)
	if err != nil {
		log.Err(err).Msg("Upgrade error")
		log.Fatal().Msg("Upgrade")
	}

	log.Debug().Msg("Connection established")
	connectionManager.AddConnectionToPool(destination, conn, uuid)
}

func getConnectionID(r *http.Request) (*string, bool) {
	uriSegments := strings.Split(r.URL.Path, "/")

	if len(uriSegments) > 1 {
		return &uriSegments[2], true
	}

	return nil, false
}

func routingHandler(conn *connection.Manager) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Debug().Msgf("Http request: %v %v", r.Method, r.RequestURI)

		var connectionID *string

		if conn.Role == connection.ServerRole {
			connID, ok := getConnectionID(r)

			if !ok {
				writeMissingConnectionID(w)
				return
			}

			connectionID = connID
		}

		response, err := conn.SendRequest(r, connectionID, 10*time.Second)

		if err != nil {
			log.Err(err).Msg("SendRequest")
			w.WriteHeader(http.StatusInternalServerError)
			_, err = w.Write([]byte(fmt.Sprint(err)))

			if err != nil {
				log.Err(err).Msg("Write body response error")
			}
		} else {
			writeResponse(w, response)
		}
	}
}

func writeMissingConnectionID(w http.ResponseWriter) {
	w.WriteHeader(http.StatusInternalServerError)
	_, err := w.Write([]byte("Missing connection id"))

	if err != nil {
		log.Err(err).Msg("Write body response error")
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
		log.Err(err).Msg("Read original response body")
		return
	}

	_, err = w.Write(bodyContent)

	if err != nil {
		log.Err(err).Msg("Write response body error")
		return
	}
}

func setup() {

	zerolog.TimeFieldFormat = ""

	zerolog.TimestampFunc = func() time.Time {
		return time.Date(2008, 1, 8, 17, 5, 05, 0, time.UTC)
	}
	log.Logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
}
