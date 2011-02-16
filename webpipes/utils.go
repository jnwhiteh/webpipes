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

func ErlangChain(components ...Component) *_ErlangChain {
	return &_ErlangChain{components}
}

//////////////////////////////////////////////////////////////////////////////
// Spawned goroutine networks of components
//
// There might be an advantage to writing your servers in this way, where each
// component has a new goroutine for each incoming request.

type _ProcChain struct {
	in chan *Conn
	out chan *Conn
	done map[*Conn]chan bool
}

func (ch *_ProcChain) GetDone() map[*Conn]chan bool{
	return ch.done
}

func (ch *_ProcChain) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	conn := NewConn(w, req)

	// Create an entry in the 'done' table so we can be notified when our
	// connection has been served, so we can return to the HTTP server.
	ch.done[conn] = make(chan bool)

	// Send the connection into the network
	ch.in <- conn

	// Wait for the connection to come out the other end
	<-ch.done[conn]
	ch.done[conn] = nil

	// TODO: There is no way to 'drop' a connection right now, and this is
	// probably undesirable.

	// Always return to the http package, where the underlying package
	// will attempt to re-use the network connection. This means that
	// currently there is no way for a process chain to DROP a connection.
	return
}

// There will be exactly one of these running for every component in
// each 'ProcChain' structure. This allows them to be re-used as
// handlers for multiple paths without contention.
func (ch *_ProcChain) Loop(component Component, in, out chan *Conn) {
	for conn := range in {
		go ch.Handle(component, conn, out)
	}
}

func (ch *_ProcChain) Handle(component Component, conn *Conn, out chan *Conn) {
	pass := component.HandleHTTPRequest(conn, conn.Request)

	if out == ch.out {
		ch.done[conn] <- pass
		// Previously this code passed the connection out the output channel.
		// We don't actually want to do that, so drop it here.
	} else {
		out <- conn
	}
}

// Create a chain of processess connected via channels where the input channel
// is used to receive the incoming request and the output channel is used
// to send the request back into the main server loop.
func ProcChainInOut(in, out chan *Conn, components ...Component) (*_ProcChain, chan *Conn, chan *Conn) {
	if in == nil {
		in = make(chan *Conn)
	}

	var chain *_ProcChain = new(_ProcChain)
	chain.done = make(map[*Conn]chan bool)

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
		go chain.Loop(comp, prev, next)

		prev = next
	}

	chain.in = in
	chain.out = next
	return chain, in, out
}

func ProcChain(components ...Component) (*_ProcChain) {
	chain, _, _ := ProcChainInOut(nil, nil, components...)
	return chain
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
	in chan *Conn
	out chan *Conn
	done map[*Conn]chan bool
}

// Take in an input and an output channel and return an object that fulfills
// the http.Handler interface
func NewNetworkHandler(in, out chan *Conn) *_NetworkHandler {
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
	nh.done[conn] = nil

}
