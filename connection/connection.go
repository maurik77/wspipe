package connection

import (
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

const (
	// Time allowed to write a message to the peer.
	defaultWriteWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	defaultPongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	defaultPingPeriod = (defaultPongWait * 9) / 10

	// Maximum message size allowed from peer.
	defaultMaxMessageSize = 512
)

type connectionInstance struct {
	// The websocket connection.
	conn                   *websocket.Conn
	httpDestinationBaseURL string
	send                   chan []byte
	id                     string
	responseChannels       map[uuid.UUID]chan http.Response
	writeWait              time.Duration
	pongWait               time.Duration
	pingPeriod             time.Duration
	maxMessageSize         int64
	unregister             func()
}

func newInstance(conn *websocket.Conn, httpDestinationBaseURL string, id string, unregister func(), opts ...Option) *connectionInstance {
	connection := &connectionInstance{
		conn:                   conn,
		httpDestinationBaseURL: httpDestinationBaseURL,
		id:                     id,
		send:                   make(chan []byte, 256),
		responseChannels:       make(map[uuid.UUID]chan http.Response),
		writeWait:              defaultWriteWait,
		pongWait:               defaultPongWait,
		pingPeriod:             defaultPingPeriod,
		maxMessageSize:         defaultMaxMessageSize,
		unregister:             unregister,
	}

	for _, opt := range opts {
		connection = opt.applyOption(connection)
	}

	return connection
}

func (c *connectionInstance) start() {
	go c.writePump()
	go c.readPump()
}

func (c *connectionInstance) sendResponseAsync(response *payload) error {
	log.Debug().Msgf("[%v] sendResponseAsync -> payload messageType: %v, messageId: %v", c.id, response.MessageType, response.MessageID)
	binary, err := response.marshal()

	if err == nil {
		c.send <- binary
	}

	return err
}

func (c *connectionInstance) sendRequestAsync(request *payload) (chan http.Response, error) {
	log.Debug().Msgf("[%v] sendRequestAsync -> payload messageType: %v, messageId: %v", c.id, request.MessageType, request.MessageID)
	binary, err := request.marshal()
	var responseChannel chan http.Response

	if err == nil {
		responseChannel = make(chan http.Response)
		c.responseChannels[request.MessageID] = responseChannel
		c.send <- binary
		log.Debug().Msgf("[%v] sendRequestAsync -> created response channel for messageId: %v", c.id, request.MessageID)
	} else {
		log.Err(err).Msgf("[%v] sendRequestAsync -> messageId: %v", c.id, request.MessageID)
	}

	return responseChannel, err
}

func (c *connectionInstance) sendRequest(request *payload, maxWait time.Duration) (*http.Response, error) {
	log.Debug().Msgf("[%v] sendRequest -> payload messageType: %v, messageId: %v", c.id, request.MessageType, request.MessageID)

	responseChannel, err := c.sendRequestAsync(request)

	defer func() {
		delete(c.responseChannels, request.MessageID)
	}()

	if err != nil {
		log.Err(err).Msgf("[%v] sendRequest -> messageId: %v", c.id, request.MessageID)
		return nil, err
	}

	ticker := time.NewTicker(maxWait)
	defer func() {
		ticker.Stop()
	}()

	select {
	case message, ok := <-responseChannel:
		log.Debug().Msgf("[%v] sendRequest -> received message from response channel. %v, %v", c.id, message, ok)

		if !ok {
			return nil, fmt.Errorf("[%v] sendRequest -> response channel for message id %v has been closed", c.id, request.MessageID)
		}

		return &message, nil
	case <-ticker.C:
		log.Warn().Msgf("[%v] sendRequest -> time out for message id %v", c.id, request.MessageID)
	}

	return nil, fmt.Errorf("[%v] sendRequest -> time out for message id %v", c.id, request.MessageID)
}

// readPump pumps messages from the websocket connection to the hub.
//
// The application runs readPump in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (c *connectionInstance) readPump() {
	log.Debug().Msgf("[%v] readPump", c.id)

	defer c.close()
	c.setConnection()

	for {
		messageType, message, err := c.conn.ReadMessage()
		log.Debug().Msgf("[%v] readPump -> messageType: %v, body length: %v, err: %v", c.id, messageType, len(message), err)

		if err != nil {
			log.Err(err).Msgf("[%v] readPump -> messageType: %v", c.id, messageType)
			break
		}

		switch messageType {
		case websocket.BinaryMessage:
			c.manageBinaryMessage(message)
		case websocket.CloseMessage:
			break
		}
	}
}

func (c *connectionInstance) close() {
	log.Info().Msgf("[%v] close", c.id)
	c.unregister()
	close(c.send)
	err := c.conn.Close()

	if err != nil {
		log.Err(err).Msgf("[%v] close", c.id)
	}
}

func (c *connectionInstance) setConnection() {
	log.Debug().Msgf("[%v] setConnection", c.id)
	c.conn.SetReadLimit(c.maxMessageSize)
	err := c.conn.SetReadDeadline(time.Now().Add(c.pongWait))
	if err != nil {
		log.Err(err).Msgf("[%v] setConnection:-> conn.SetReadDeadline", c.id)
	}
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(c.pongWait))
	})
}

func (c *connectionInstance) manageBinaryMessage(message []byte) {
	log.Debug().Msgf("[%v] manageBinaryMessage", c.id)
	payload, err := unmarshalPayload(message)

	if err != nil {
		log.Err(err).Msgf("[%v] manageBinaryMessage", c.id)
		return
	}

	log.Debug().Msgf("[%v] manageBinaryMessage -> payload messageType: %v, messageId: %v", c.id, payload.MessageType, payload.MessageID)

	switch payload.MessageType {
	case RequestMessage:
		c.manageRequestMessage(payload)
	case ResponseMessage:
		c.manageResponseMessage(payload)
	}
}

func (c *connectionInstance) writePump() {
	log.Debug().Msgf("[%v] writePump", c.id)
	ticker := time.NewTicker(c.pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			log.Debug().Msgf("[%v] writePump -> received message from the send channel", c.id)
			c.setWriteDeadline()

			if !ok {
				// The hub closed the channel.
				log.Debug().Msgf("[%v] writePump -> close message sent", c.id)
				err := c.conn.WriteMessage(websocket.CloseMessage, []byte{})

				if err != nil {
					log.Err(err).Msgf("[%v] writePump -> write close message", c.id)
				}

				return
			}

			w, err := c.conn.NextWriter(websocket.BinaryMessage)
			if err != nil {
				return
			}

			_, err = w.Write(message)

			if err != nil {
				log.Err(err).Msgf("[%v] writePump -> write message", c.id)
			}

			if err := w.Close(); err != nil {
				log.Debug().Msgf("[%v] writePump -> error on closing, exit from routine", c.id)
				return
			}

			log.Debug().Msgf("[%v] writePump -> message sent", c.id)
		case <-ticker.C:
			c.setWriteDeadline()

			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *connectionInstance) manageRequestMessage(payload *payload) {
	log.Debug().Msgf("[%v] manageRequestMessage -> messageId: %v", c.id, payload.MessageID)
	r, err := payload.getRequestWithDestinationURL(c.httpDestinationBaseURL)

	if err != nil {
		log.Err(err).Msgf("[%v] manageRequestMessage", c.id)
		return
	}

	res, err := http.DefaultClient.Do(r)

	if err != nil {
		log.Err(err).Msgf("[%v] manageRequestMessage", c.id)
		return
	}

	responsePayload, err := createResponseMessage(res, payload.MessageID)

	if err != nil {
		log.Err(err).Msgf("[%v] manageRequestMessage", c.id)
		return
	}

	err = c.sendResponseAsync(responsePayload)

	if err != nil {
		log.Err(err).Msgf("[%v] manageRequestMessage", c.id)
		return
	}
}

func (c *connectionInstance) manageResponseMessage(payload *payload) {
	log.Debug().Msgf("[%v] manageResponseMessage -> messageId: %v", c.id, payload.MessageID)
	res, err := payload.getResponse()

	if err != nil {
		log.Err(err).Msgf("[%v] manageResponseMessage", c.id)
		return
	}

	if channel, ok := c.responseChannels[payload.MessageID]; ok {
		log.Debug().Msgf("[%v] manageResponseMessage -> channel (%v) found for message id %v: %v %v", c.id, channel, payload.MessageID, res.Status, res.StatusCode)
		channel <- *res
	} else {
		log.Debug().Msgf("[%v] manageResponseMessage -> channel NOT found for message id %v", c.id, payload.MessageID)
	}
}

func (c *connectionInstance) setWriteDeadline() {
	err := c.conn.SetWriteDeadline(time.Now().Add(c.writeWait))

	if err != nil {
		log.Err(err).Msgf("[%v] setWriteDeadline", c.id)
	}
}
