package connection

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 512
)

type connectionInstance struct {
	// The websocket connection.
	conn                   *websocket.Conn
	httpDestinationBaseUrl string
	send                   chan []byte
	id                     string
	responseChannels       map[uuid.UUID]chan http.Response
}

func newInstance(conn *websocket.Conn, httpDestinationBaseUrl string, id string) *connectionInstance {
	connection := &connectionInstance{
		conn:                   conn,
		httpDestinationBaseUrl: httpDestinationBaseUrl,
		id:                     id,
		send:                   make(chan []byte, 256),
		responseChannels:       make(map[uuid.UUID]chan http.Response),
	}

	return connection
}

func (c *connectionInstance) Start() {
	go c.writePump()
	go c.readPump()
}

func (c *connectionInstance) sendResponseAsync(response *payload) error {
	fmt.Printf("SendResponseAsync [%v]-> payload MessageType:%v, MessageId:%v\n", c.id, response.MessageType, response.MessageID)
	binary, err := response.marshal()

	if err == nil {
		c.send <- binary
	}

	return err
}

func (c *connectionInstance) sendRequestAsync(request *payload) (chan http.Response, error) {
	fmt.Printf("SendRequestAsync [%v]-> payload MessageType:%v, MessageId:%v\n", c.id, request.MessageType, request.MessageID)
	binary, err := request.marshal()
	var responseChannel chan http.Response

	if err == nil {
		responseChannel = make(chan http.Response)
		c.responseChannels[request.MessageID] = responseChannel
		c.send <- binary
	}

	fmt.Printf("SendRequestAsync [%v]-> Created response channel %v for MessageId:%v. Err:%v\n", c.id, responseChannel, request.MessageID, err)
	return responseChannel, err
}

func (c *connectionInstance) sendRequest(request *payload, maxWait time.Duration) (*http.Response, error) {
	fmt.Printf("SendRequest [%v]-> payload MessageType:%v, MessageId:%v\n", c.id, request.MessageType, request.MessageID)

	responseChannel, err := c.sendRequestAsync(request)

	if err != nil {
		return nil, err
	}

	ticker := time.NewTicker(maxWait)
	defer func() {
		ticker.Stop()
	}()

	fmt.Printf("SendRequest [%v]-> Received response channel %v for MessageId:%v.", c.id, responseChannel, request.MessageID)

	select {
	case message, ok := <-responseChannel:
		log.Printf("SendRequest [%v]: Received message from response channel. %v, %v\n", c.id, message, ok)

		if !ok {
			return nil, fmt.Errorf("Response channel for connection id %v and message id %v has been closed", c.id, request.MessageID)
		}

		return &message, nil
	case <-ticker.C:
		log.Printf("SendRequest [%v]: Time out for message id %v\n", c.id, request.MessageID)
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
	log.Printf("Read pump routine started: [%v]", c.id)

	defer func() {
		// c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		messageType, message, err := c.conn.ReadMessage()
		fmt.Printf("readPump routine [%v]-> MessageType:%v, Body:%v, Err:%v\n", c.id, messageType, len(message), err)
		if err != nil {
			break
		}
		if messageType == websocket.BinaryMessage {
			payload, err := unmarshalPayload(message)

			if err != nil {
				log.Println(err)
				break
			}

			fmt.Printf("readPump routine [%v]-> payload MessageType:%v, MessageId:%v\n", c.id, payload.MessageType, payload.MessageID)

			switch payload.MessageType {
			case RequestMessage:
				c.manageRequestMessage(payload)
			case ResponseMessage:
				c.manageResponseMessage(payload)
			}
		}
	}
}

func (c *connectionInstance) writePump() {
	log.Println("Write pump routine started")
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			log.Printf("writePump routine [%v]: received message from the send channel\n", c.id)
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))

			if !ok {
				// The hub closed the channel.
				log.Printf("writePump routine [%v]: close message sent\n", c.id)
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.BinaryMessage)
			if err != nil {
				return
			}

			w.Write(message)

			if err := w.Close(); err != nil {
				log.Printf("writePump routine [%v]: error on closing, exit from routine\n", c.id)
				return
			}

			log.Printf("writePump routine [%v]: message sent\n", c.id)
		case <-ticker.C:
			// log.Printf("writePump routine [%v]: ping message\n", c.id)
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *connectionInstance) manageRequestMessage(payload *payload) {
	log.Printf("manageRequestMessage [%v] %v\n", c.id, payload.MessageID)
	r, err := payload.getRequestWithDestinationUrl(c.httpDestinationBaseUrl)

	if err != nil {
		log.Println(err)
		return
	}

	res, err := http.DefaultClient.Do(r)

	if err != nil {
		log.Printf("manageRequestMessage [%v]-> Err: %v\n", c.id, err)
		return
	}

	responsePayload, err := createResponseMessage(res, payload.MessageID)

	if err != nil {
		log.Printf("manageRequestMessage [%v]-> Err: %v\n", c.id, err)
		return
	}

	err = c.sendResponseAsync(responsePayload)

	if err != nil {
		log.Printf("manageRequestMessage [%v]-> Err: %v\n", c.id, err)
		return
	}
}

func (c *connectionInstance) manageResponseMessage(payload *payload) {
	log.Printf("manageResponseMessage [%v] %v\n", c.id, payload.MessageID)
	res, err := payload.getResponse()

	if err != nil {
		log.Println(err)
		return
	}

	if channel, ok := c.responseChannels[payload.MessageID]; ok {
		log.Printf("manageResponseMessage [%v]-> channel (%v) found for message id %v: %v %v\n", c.id, channel, payload.MessageID, res.Status, res.StatusCode)
		channel <- *res
	} else {
		log.Printf("manageResponseMessage [%v] channel NOT found for message id %v\n", c.id, payload.MessageID)
	}

	log.Printf("manageResponseMessage [%v] Exit. %v\n", c.id, payload.MessageID)
}
