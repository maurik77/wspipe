package connection

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const (
	ClientRole = "client"
	ServerRole = "server"
)

type Manager struct {
	Role              string
	clients           map[string]*connectionInstance
	connectionOptions []Option
}

func New(role string, opts ...Option) *Manager {
	return &Manager{
		clients:           make(map[string]*connectionInstance),
		Role:              role,
		connectionOptions: opts,
	}
}

func (c *Manager) add(connectionID string, conn *connectionInstance) {
	c.clients[connectionID] = conn
}

func (c *Manager) get(connectionID string) *connectionInstance {
	if val, ok := c.clients[connectionID]; ok {
		return val
	}

	return nil
}

func (c *Manager) SendRequestAsync(r *http.Request, connectionID *string) (chan http.Response, error) {
	var connection *connectionInstance

	if connectionID == nil {
		for k := range c.clients {
			connection = c.clients[k]
			break
		}
	} else {
		connection = c.get(*connectionID)
	}

	if connection == nil {
		return nil, fmt.Errorf("Unable to find connection with id %v", *connectionID)
	}

	message, err := createRequestMessage(r)

	if err != nil {
		return nil, err
	}

	return connection.sendRequestAsync(message)
}

func (c *Manager) SendRequest(r *http.Request, connectionID *string, maxWait time.Duration) (*http.Response, error) {
	var connection *connectionInstance

	if connectionID == nil {
		for k := range c.clients {
			connection = c.clients[k]
			break
		}
	} else {
		connection = c.get(*connectionID)
	}

	if connection == nil {
		return nil, fmt.Errorf("Unable to find connection with id %v", *connectionID)
	}

	message, err := createRequestMessage(r)

	if err != nil {
		return nil, err
	}

	return connection.sendRequest(message, maxWait)
}

func (c *Manager) AddConnectionToPool(destination *string, conn *websocket.Conn, connectionID string) {
	connection := newInstance(
		conn,
		*destination,
		connectionID,
		c.connectionOptions...,
	)

	c.add(connectionID, connection)
	connection.start()
}
