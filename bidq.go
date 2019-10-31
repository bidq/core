package bidq

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/bidq/bidq/connset"
	"github.com/bidq/bidq/enums"
)

type reply struct {
	Data   []byte
	Origin *net.TCPConn
	To     enums.Target
}

type request struct {
	Data   []byte
	Origin *net.TCPConn
}

type job struct {
	Conn *net.TCPConn
	Bid  bool
}

var incoming = make(chan request)
var outgoing = make(chan reply)
var pending = make(map[string]*job)

type server struct {
	host    string
	port    uint16
	handler func(c *net.TCPConn) error
}

func (s *server) Listen() error {
	addr := net.JoinHostPort(s.host, strconv.Itoa(int(s.port)))
	tcpaddr, err := net.ResolveTCPAddr(enums.Tcp, addr)
	if err != nil {
		return err
	}

	ln, err := net.ListenTCP(enums.Tcp, tcpaddr)
	if err != nil {
		return err
	}
	for {
		conn, err := ln.AcceptTCP()
		if err != nil {
			continue
		}
		go s.handler(conn)
	}
}

func readFromConn(c *net.TCPConn, cs *connset.ConnectionsSet) error {
	var buff = make([]byte, 1500)
	for {
		n, err := c.Read(buff)
		if err != nil {
			c.Close()
			cs.Delete(c)
			return err
		}
		incoming <- request{Origin: c, Data: buff[0:n]}
	}
}

func makeConnectionHandler(cs *connset.ConnectionsSet) func(c *net.TCPConn) error {
	return func(c *net.TCPConn) error {
		cs.Add(c)
		return readFromConn(c, cs)
	}
}

type BaseRequest struct {
	Type enums.Message `json:"type,omitempty"`
	Id   string        `json:"id"`
}

type SubmitRequest struct {
	Type    enums.Message `json:"type,omitempty"`
	Id      string        `json:"id"`
	Topic   string        `json:"topic,omitempty"`
	Payload interface{}   `json:"payload"`
}

type AckSubmitReply struct {
	Type     enums.Message `json:"type,omitempty"`
	Id       string        `json:"id"`
	ClientId string        `json:"clientId"`
}

type FailureReply struct {
	Type   enums.Message `json:"type,omitempty"`
	Id     string        `json:"id"`
	Reason string        `json:"reason"`
}

type SuccessReply struct {
	Type  enums.Message `json:"type,omitempty"`
	Id    string        `json:"id"`
	Value interface{}   `json:"reason"`
}

type CancelRequest struct {
	Type enums.Message `json:"type,omitempty"`
	Id   string        `json:"id"`
}

type BidRequest struct {
	Type enums.Message `json:"type,omitempty"`
	Id   string        `json:"id"`
}

func cancelJob(jobId string, req request, reason string) {
	_, ok := pending[jobId]
	if ok {
		failureReply, err := json.Marshal(FailureReply{Type: enums.JobFailure, Id: jobId, Reason: reason})
		if err == nil {
			outgoing <- reply{Origin: req.Origin, Data: failureReply, To: enums.Direct}
			delete(pending, jobId)
		}
	}
}

func setupCleanup(jobId string, req request, timeout time.Duration) {
	time.Sleep(timeout)
	cancelJob(jobId, req, "Queue timeout")
}

func handleSubmit(req request, timeout time.Duration) error {
	var sr SubmitRequest
	err := json.Unmarshal(req.Data, &sr)
	if err != nil {
		return err
	}
	id := uuid.New().String()
	submitAck, err := json.Marshal(AckSubmitReply{Id: id, ClientId: sr.Id, Type: enums.SubmitAck})
	if err != nil {
		return err
	}
	outgoing <- reply{Origin: req.Origin, Data: submitAck, To: enums.Direct}
	pending[id] = &job{Conn: req.Origin, Bid: false}
	go setupCleanup(id, req, timeout)
	sr.Id = id
	submitReply, err := json.Marshal(sr)
	if err != nil {
		return err
	}
	outgoing <- reply{Origin: req.Origin, Data: submitReply, To: enums.Spread}
	return nil
}

func handleCancel(req request) error {
	var cr CancelRequest
	err := json.Unmarshal(req.Data, &cr)
	if err != nil {
		return err
	}
	cancelJob(cr.Id, req, "Client cancel")
	return nil
}

func handleBid(req request) error {
	var br BidRequest
	err := json.Unmarshal(req.Data, &br)
	if err != nil {
		return err
	}
	job, ok := pending[br.Id]
	if ok {
		var msg enums.Message
		if msg = enums.BidReject; !job.Bid {
			msg = enums.BidAck
			job.Bid = true
		}
		re, err := json.Marshal(BidRequest{Type: msg, Id: br.Id})
		if err == nil {
			outgoing <- reply{Origin: req.Origin, Data: re, To: enums.Direct}
		}
	}
	return nil
}

func handleJobResult(req request) error {
	var br BaseRequest
	err := json.Unmarshal(req.Data, &br)
	if err != nil {
		return err
	}
	job, ok := pending[br.Id]
	if ok {
		outgoing <- reply{Origin: job.Conn, Data: req.Data, To: enums.Direct}
	}
	return nil
}

func processMessage(req request, timeout time.Duration) error {
	var m BaseRequest
	err := json.Unmarshal(req.Data, &m)
	if err != nil {
		return err
	}
	switch m.Type {
	case enums.Submit:
		return handleSubmit(req, timeout)
	case enums.Cancel:
		return handleCancel(req)
	case enums.Bid:
		return handleBid(req)
	case enums.JobFailure, enums.JobSuccess:
		return handleJobResult(req)
	default:
		return fmt.Errorf("Bad request type: %s", m.Type)
	}
}

func handleMessages(timeout time.Duration) {
	for {
		req, ok := <-incoming
		if !ok {
			return
		}
		processMessage(req, timeout)
	}
}

func publish(cs *connset.ConnectionsSet) {
	for {
		reply, ok := <-outgoing
		if !ok {
			return
		}
		cs.ForEach(func(c *net.TCPConn) {
			switch reply.To {
			case enums.Direct:
				if reply.Origin == c {
					c.Write(reply.Data)
				}
			case enums.Spread:
				if reply.Origin != c {
					c.Write(reply.Data)
				}
			case enums.Broadcast:
				c.Write(reply.Data)
			}
		})
	}
}

func Start(host string, port uint16, timeout time.Duration) {
	cs := connset.MakeConnectionSet()
	handler := makeConnectionHandler(cs)
	srv := &server{host: host, port: port, handler: handler}
	go srv.Listen()
	go publish(cs)
	handleMessages(timeout)
}
