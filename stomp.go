package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/google/uuid"
	"nhooyr.io/websocket"
)

type ACK int

const (
	AUTO   ACK = 1
	CLIENT ACK = 2
)

type StompConnection struct {
	ctx           context.Context
	conn          *websocket.Conn
	lock          sync.RWMutex
	subscriptions map[uuid.UUID]chan StompMessage
}

type StompMessage struct {
	Body    []byte
	Headers map[string]string
	Verb    string
}

func (sm *StompMessage) GetBytes() (b []byte) {
	return []byte(sm.GetString())
}

func (sm *StompMessage) GetString() (s string) {
	var builder = strings.Builder{}
	builder.WriteString(sm.Verb + "\n")
	for k, v := range sm.Headers {
		builder.WriteString(k + ":" + v + "\n")
	}

	builder.WriteString("\n")
	if len(sm.Body) > 0 {
		builder.WriteString(string(sm.Body))
	}

	builder.WriteString("\x00\n")
	return builder.String()
}

func NewStompConnection(url string) (sc *StompConnection, err error) {
	var ctx = context.Background()
	var dialOptions = websocket.DialOptions{
		HTTPClient: http.DefaultClient,
	}

	conn, _, err := websocket.Dial(
		ctx,
		url,
		&dialOptions,
	)

	if err == nil {
		sc = &StompConnection{
			ctx:           ctx,
			conn:          conn,
			lock:          sync.RWMutex{},
			subscriptions: map[uuid.UUID]chan StompMessage{},
		}
	} else {
		sc = nil
	}

	return sc, err
}

func (sc *StompConnection) AddSubscription(dest string) (
	c chan StompMessage,
	u uuid.UUID,
	err error,
) {
	u, err = uuid.NewUUID()
	if err != nil {
		return nil, uuid.UUID{}, err
	}

	if sc.subscribe(dest, u.String(), AUTO) != nil {
		return nil, u, err
	}

	c = make(chan StompMessage)
	defer sc.lock.Unlock()
	sc.lock.Lock()
	sc.subscriptions[u] = c
	return c, u, nil
}

func (sc *StompConnection) Connect(receipt uuid.UUID) (err error) {
	var stompMessage = StompMessage{
		Verb: "CONNECT",
		Headers: map[string]string{
			"accept-version": "1.0,1.1,2.0",
		},
	}

	return sc.conn.Write(sc.ctx, websocket.MessageText, stompMessage.GetBytes())
}

func (sc *StompConnection) Disconnect(receipt uuid.UUID) (err error) {
	var stompMessage = StompMessage{
		Verb: "DISCONNECT",
		Headers: map[string]string{
			"receipt": receipt.String(),
		},
	}

	return sc.conn.Write(sc.ctx, websocket.MessageText, stompMessage.GetBytes())
}

func (sc *StompConnection) Listen() {
	for {
		messageType, reader, err := sc.conn.Reader(sc.ctx)
		if err == nil {
			var sm = processReceive(messageType, reader)
			subscriptionId, ok := sm.Headers["subscription"]
			if ok {
				subscription, ok := sc.getSubscription(subscriptionId)
				if ok {
					subscription <- sm
				}
			}
		} else {
			log.Print(err)
		}
	}
}

func (sc *StompConnection) RemoveSubscription(u uuid.UUID) (
	err error,
) {
	if sc.unsubscribe(u.String()) != nil {
		return err
	}

	defer sc.lock.Unlock()
	sc.lock.Lock()
	close(sc.subscriptions[u])
	delete(sc.subscriptions, u)
	return nil
}

func (sc *StompConnection) Send(
	dest string,
	body []byte,
	contentType string,
	transactionId string,
) (err error) {
	if len(contentType) == 0 {
		contentType = "text/plain"
	}

	var stompMessage = StompMessage{
		Verb: "SEND",
		Headers: map[string]string{
			"destination":  dest,
			"content-type": contentType,
		},
		Body: body,
	}

	if len(transactionId) > 0 {
		stompMessage.Headers["transaction"] = transactionId
	}

	return sc.conn.Write(sc.ctx, websocket.MessageText, []byte(stompMessage.GetBytes()))
}

func processReceive(
	messageType websocket.MessageType,
	ioReader io.Reader,
) (sm StompMessage) {
	var headers = map[string]string{}
	var scanner = bufio.NewScanner(ioReader)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		var text = scanner.Text()
		if len(text) == 0 {
			break
		}

		var parts = strings.Split(text, ":")
		if len(parts) > 1 {
			var key = strings.ToLower(strings.TrimSpace(parts[0]))
			var value = strings.TrimSpace(parts[1])
			headers[key] = value
		}
	}

	var bodyBuffer = bytes.NewBuffer([]byte{})
	for scanner.Scan() {
		bodyBuffer.Write(scanner.Bytes())
	}

	var length uint64 = 0
	var contentLength, ok = headers["content-length"]
	if ok {
		var err error
		length, err = strconv.ParseUint(contentLength, 10, 64)
		if err != nil {
			log.Println("Error parsing value content-length header")
		}
	}

	if length > 0 {
		return StompMessage{
			Headers: headers,
			Body:    bodyBuffer.Bytes()[:length],
		}
	} else {
		return StompMessage{
			Headers: headers,
			Body:    bodyBuffer.Bytes(),
		}
	}
}

func (sc *StompConnection) getSubscription(subscriptionId string) (
	c chan StompMessage,
	ok bool,
) {
	u, err := uuid.Parse(subscriptionId)
	if err != nil {
		log.Println(fmt.Sprintf("Ignoring subscription ID: %s. Only valid UUID as processed", u.String()))
		return nil, false
	}

	sc.lock.RLock()
	defer sc.lock.RUnlock()
	c, ok = sc.subscriptions[u]
	return c, ok
}

func (sc *StompConnection) subscribe(
	dest string,
	idx string,
	ack ACK,
) (err error) {
	var ackStr string
	switch ack {
	case AUTO:
		ackStr = "auto"
	case CLIENT:
		ackStr = "client"
	}

	var stompMessage = StompMessage{
		Verb: "SUBSCRIBE",
		Headers: map[string]string{
			"id":          idx,
			"destination": dest,
			"ack":         ackStr,
		},
	}

	return sc.conn.Write(sc.ctx, websocket.MessageText, stompMessage.GetBytes())
}

func (sc *StompConnection) unsubscribe(
	idx string,
) (err error) {
	var stompMessage = StompMessage{
		Verb: "UNSUBSCRIBE",
		Headers: map[string]string{
			"id": idx,
		},
	}

	return sc.conn.Write(sc.ctx, websocket.MessageText, stompMessage.GetBytes())
}
