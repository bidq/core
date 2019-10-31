package connset

import "net"

type void struct{}

var member void

type ConnectionsSet struct {
	set map[*net.TCPConn]void
}

func (cs *ConnectionsSet) Add(conn *net.TCPConn) {
	cs.set[conn] = member
}

func (cs *ConnectionsSet) Has(conn *net.TCPConn) bool {
	_, ok := cs.set[conn]
	return ok
}

func (cs *ConnectionsSet) Size(conn *net.TCPConn) int {
	return len(cs.set)
}

func (cs *ConnectionsSet) Delete(conn *net.TCPConn) {
	delete(cs.set, conn)
}

func (cs *ConnectionsSet) ForEach(handler func(*net.TCPConn)) {
	for conn := range cs.set {
		handler(conn)
	}
}

func MakeConnectionSet() *ConnectionsSet {
	set := make(map[*net.TCPConn]void)
	return &ConnectionsSet{set: set}
}
