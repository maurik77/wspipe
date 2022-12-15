package connection

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http"
	"net/url"

	"github.com/google/uuid"
)

const (
	RequestMessage  = 0
	ResponseMessage = 1
)

type payload struct {
	MessageType uint8
	MessageID   uuid.UUID
	Message     []byte
}

func createRequestMessage(r *http.Request) (*payload, error) {
	buf := new(bytes.Buffer)
	err := r.WriteProxy(buf)

	if err != nil {
		return nil, err
	}

	return &payload{
		MessageType: RequestMessage,
		Message:     buf.Bytes(),
		MessageID:   uuid.New(),
	}, nil
}

func createResponseMessage(r *http.Response, messageID uuid.UUID) (*payload, error) {
	buf := new(bytes.Buffer)
	err := r.Write(buf)

	if err != nil {
		return nil, err
	}

	return &payload{
		MessageType: ResponseMessage,
		Message:     buf.Bytes(),
		MessageID:   messageID,
	}, nil
}

func (m *payload) marshal() ([]byte, error) {
	buf := new(bytes.Buffer)
	buf.WriteByte(m.MessageType)
	buf.Write(m.MessageID[:])
	buf.Write(m.Message)
	return buf.Bytes(), nil
}

func unmarshalPayload(message []byte) (*payload, error) {
	messageID, err := uuid.FromBytes(message[1:17])

	payload := payload{
		MessageType: message[0],
		MessageID:   messageID,
		Message:     message[17:],
	}

	return &payload, err
}

func (m *payload) getResponse() (*http.Response, error) {
	if m.MessageType != ResponseMessage {
		return nil, fmt.Errorf("Invalid message type: %v. Expected: %v", m.MessageType, ResponseMessage)
	}

	reader := bufio.NewReader(bytes.NewReader(m.Message))
	r, err := http.ReadResponse(reader, nil)

	return r, err
}

func (m *payload) getRequest() (*http.Request, error) {
	if m.MessageType != RequestMessage {
		return nil, fmt.Errorf("Invalid message type: %v. Expected: %v", m.MessageType, RequestMessage)
	}

	reader := bufio.NewReader(bytes.NewReader(m.Message))
	r, err := http.ReadRequest(reader)

	return r, err
}

func (m *payload) getRequestWithDestinationURL(httpDestinationBaseURL string) (*http.Request, error) {
	r, err := m.getRequest()

	if err != nil {
		return r, err
	}

	u, err := url.Parse(fmt.Sprintf("%v%v", httpDestinationBaseURL, r.RequestURI))
	if err != nil {
		return r, err
	}

	r.RequestURI = ""
	r.URL = u

	return r, err
}
