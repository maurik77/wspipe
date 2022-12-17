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
	log.Debug().Msgf("SendResponseAsync [%v]-> payload MessageType:%v, MessageId:%v", c.id, response.MessageType, response.MessageID)
	binary, err := response.marshal()

	if err == nil {
		c.send <- binary
	}

	return err
}

func (c *connectionInstance) sendRequestAsync(request *payload) (chan http.Response, error) {
	log.Debug().Msgf("SendRequestAsync [%v]-> payload MessageType:%v, MessageId:%v", c.id, request.MessageType, request.MessageID)
	binary, err := request.marshal()
	var responseChannel chan http.Response

	if err == nil {
		responseChannel = make(chan http.Response)
		c.responseChannels[request.MessageID] = responseChannel
		c.send <- binary
	}

	log.Debug().Msgf("SendRequestAsync [%v]-> Created response channel %v for MessageId:%v. Err:%v", c.id, responseChannel, request.MessageID, err)
	return responseChannel, err
}

func (c *connectionInstance) sendRequest(request *payload, maxWait time.Duration) (*http.Response, error) {
	log.Debug().Msgf("SendRequest [%v]-> payload MessageType:%v, MessageId:%v", c.id, request.MessageType, request.MessageID)

	responseChannel, err := c.sendRequestAsync(request)

	if err != nil {
		return nil, err
	}

	ticker := time.NewTicker(maxWait)
	defer func() {
		ticker.Stop()
	}()

	log.Debug().Msgf("SendRequest [%v]-> Received response channel %v for MessageId:%v.", c.id, responseChannel, request.MessageID)

	select {
	case message, ok := <-responseChannel:
		log.Debug().Msgf("SendRequest [%v]: Received message from response channel. %v, %v", c.id, message, ok)

		if !ok {
			return nil, fmt.Errorf("Response channel for connection id %v and message id %v has been closed", c.id, request.MessageID)
		}

		return &message, nil
	case <-ticker.C:
		log.Debug().Msgf("SendRequest [%v]: Time out for message id %v", c.id, request.MessageID)
	}

	delete(c.responseChannels, request.MessageID)
	return nil, fmt.Errorf("Time out for connection id %v and message id %v", c.id, request.MessageID)
}

// readPump pumps messages from the websocket connection to the hub.
//
// The application runs readPump in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (c *connectionInstance) readPump() {
	log.Debug().Msgf("Read pump routine started: [%v]", c.id)

	defer c.close()
	c.setConnection()

	for {
		messageType, message, err := c.conn.ReadMessage()
		log.Debug().Msgf("readPump routine [%v]-> MessageType:%v, Body:%v, Err:%+v", c.id, messageType, len(message), err)

		if err != nil {
			log.Err(err).Msgf("readPump routine [%v]-> MessageType:%v", c.id, messageType)
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
	log.Info().Msgf("Close: unregistered, send channel closed, websocket connection closed [%v]", c.id)
	c.unregister()
	close(c.send)
	err := c.conn.Close()

	if err != nil {
		log.Err(err).Msg("close")
	}
}

func (c *connectionInstance) setConnection() {
	c.conn.SetReadLimit(c.maxMessageSize)
	err := c.conn.SetReadDeadline(time.Now().Add(c.pongWait))
	if err != nil {
		log.Err(err).Msg("SetReadDeadline")
	}
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(c.pongWait))
	})
}

func (c *connectionInstance) manageBinaryMessage(message []byte) {
	payload, err := unmarshalPayload(message)

	if err != nil {
		log.Err(err).Msg("manageBinaryMessage")
		return
	}

	log.Debug().Msgf("readPump routine [%v]-> payload MessageType:%v, MessageId:%v", c.id, payload.MessageType, payload.MessageID)

	switch payload.MessageType {
	case RequestMessage:
		c.manageRequestMessage(payload)
	case ResponseMessage:
		c.manageResponseMessage(payload)
	}
}

func (c *connectionInstance) writePump() {
	log.Debug().Msg("Write pump routine started")
	ticker := time.NewTicker(c.pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			log.Debug().Msgf("writePump routine [%v]: received message from the send channel", c.id)
			err := c.conn.SetWriteDeadline(time.Now().Add(c.writeWait))

			if err != nil {
				log.Err(err).Msg("SetWriteDeadline")
			}

			if !ok {
				// The hub closed the channel.
				log.Debug().Msgf("writePump routine [%v]: close message sent", c.id)
				err := c.conn.WriteMessage(websocket.CloseMessage, []byte{})

				if err != nil {
					log.Err(err).Msg("Write close message error")
				}

				return
			}

			w, err := c.conn.NextWriter(websocket.BinaryMessage)
			if err != nil {
				return
			}

			_, err = w.Write(message)

			if err != nil {
				log.Err(err).Msg("Write message error")
			}

			if err := w.Close(); err != nil {
				log.Debug().Msgf("writePump routine [%v]: error on closing, exit from routine", c.id)
				return
			}

			log.Debug().Msgf("writePump routine [%v]: message sent", c.id)
		case <-ticker.C:
			// log.Debug().Msgf("writePump routine [%v]: ping message", c.id)
			err := c.conn.SetWriteDeadline(time.Now().Add(c.writeWait))

			if err != nil {
				log.Err(err).Msg("SetWriteDeadline")
			}

			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *connectionInstance) manageRequestMessage(payload *payload) {
	log.Debug().Msgf("manageRequestMessage [%v] %v", c.id, payload.MessageID)
	r, err := payload.getRequestWithDestinationURL(c.httpDestinationBaseURL)

	if err != nil {
		log.Err(err).Msg("manageRequestMessage")
		return
	}

	res, err := http.DefaultClient.Do(r)

	if err != nil {
		log.Debug().Msgf("manageRequestMessage [%v]-> Err: %v", c.id, err)
		return
	}

	responsePayload, err := createResponseMessage(res, payload.MessageID)

	if err != nil {
		log.Debug().Msgf("manageRequestMessage [%v]-> Err: %v", c.id, err)
		return
	}

	err = c.sendResponseAsync(responsePayload)

	if err != nil {
		log.Debug().Msgf("manageRequestMessage [%v]-> Err: %v", c.id, err)
		return
	}
}

func (c *connectionInstance) manageResponseMessage(payload *payload) {
	log.Debug().Msgf("manageResponseMessage [%v] %v", c.id, payload.MessageID)
	res, err := payload.getResponse()

	if err != nil {
		log.Err(err).Msg("manageResponseMessage")
		return
	}

	if channel, ok := c.responseChannels[payload.MessageID]; ok {
		log.Debug().Msgf("manageResponseMessage [%v]-> channel (%v) found for message id %v: %v %v", c.id, channel, payload.MessageID, res.Status, res.StatusCode)
		channel <- *res
	} else {
		log.Debug().Msgf("manageResponseMessage [%v] channel NOT found for message id %v", c.id, payload.MessageID)
	}

	log.Debug().Msgf("manageResponseMessage [%v] Exit. %v", c.id, payload.MessageID)
}
