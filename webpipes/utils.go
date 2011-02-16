package webpipes

import "http"

//////////////////////////////////////////////////////////////////////////////
// Erlang-style component chains

type _ErlangChain struct {
	components []Component
}

func (ch *_ErlangChain) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	conn := NewConn(w, req)

	for _, component := range ch.components {
		pass := component.HandleHTTPRequest(conn, req)
		if !pass {
			panic("this should never happen")
		}
	}
}

func Chain(components ...Component) *_ErlangChain {
	return &_ErlangChain{components}
}

//////////////////////////////////////////////////////////////////////////////
// Spawned goroutine networks of components
//
// There might be an advantage to writing your servers in this way, where each
// component has a new goroutine for each incoming request.

func ProcNetwork(components ...Component) (chan *Conn, chan *Conn) {
	return ProcNetworkInOut(nil, nil, components...)
}

func ProcNetworkInOut(in, out chan *Conn, components ...Component) (chan *Conn, chan *Conn) {
	if in == nil {
		in = make(chan *Conn)
	}

	var prev chan *Conn = in
	var next chan *Conn = out

	for idx, comp := range components {
		// If this is the last process and an out channel has been specified
		// alter the loop so it is used instead of creating a new one.
		if idx == len(components)-1 && out != nil {
			next = out
		} else {
			next = make(chan *Conn)
		}

		// Spawn a server farm process for this component.
		go componentLoop(comp, prev, next)

		prev = next
	}

	return in, next
}

// There will be exactly one of these running for every component in each
// process chain. This allows them to be re-used as handlers for multiple paths
// without contention.
func componentLoop(component Component, in, out chan *Conn) {
	for conn := range in {
		go componentHandle(component, conn, out)
	}
}

func componentHandle(component Component, conn *Conn, out chan *Conn) {
	pass := component.HandleHTTPRequest(conn, conn.Request)

	if !pass {
		return
	}

	out <- conn
}

//////////////////////////////////////////////////////////////////////////////
// Component network adapter
//
// This allows the developer to create a process network to respond to requests
// and this adapter just listens for new requests in a separate goroutine. When
// a request arrives, it is injected into the user connected network. This
// adapter then waits for the same connection to exit the network (via the out
// channel), which lets the ServeHTTP call return.
//
// This is experimental

type _NetworkHandler struct {
	in chan *Conn            // the input channel for the entire network
	out chan *Conn           // the output channel for the entire network
	done map[*Conn]chan bool // a map for tracking non-finished connections
}

// Take in an input and an output channel and return an object that fulfills
// the http.Handler interface
func NetworkChain(components ...Component) *_NetworkHandler {
	in, out := ProcNetwork(components...)
	return NetworkChainInOut(in, out)
}

func NetworkChainInOut(in, out chan *Conn) *_NetworkHandler {
	nh := &_NetworkHandler{in, out, make(map[*Conn]chan bool)}
	go nh.Sink()
	return nh
}

func (nh *_NetworkHandler) Sink() {
	for conn := range nh.out {
		nh.done[conn] <- true
	}
}

func (nh *_NetworkHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	conn := NewConn(w, req)

	nh.done[conn] = make(chan bool)

	// Send the connection into the network
	nh.in <- conn

	// Wait for a response
	<-nh.done[conn]
	nh.done[conn] = nil, false
}
