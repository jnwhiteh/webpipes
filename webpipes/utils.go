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
			return
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

func (ch *_ProcChain) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	conn := NewConn(w, req)

	// Create an entry in the 'done' table so we can be notified when our
	// connection has been served, so we can return to the HTTP server.
	ch.done[conn] = make(chan bool)

	// Send the connection into the network, asychronously
	go ch.Inject(conn)

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

// Inject a connection into the process network chain
func (ch *_ProcChain) Inject(conn *Conn) {
	ch.in <- conn
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
		out <- conn
	} else {
		out <- conn
	}
}

// Create a chain of processess connected via channels where the input channel
// is used to receive the incoming request and the output channel is used
// to send the request back into the main server loop.
func ProcChain(in, out chan *Conn, components ...Component) *_ProcChain {
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
	return chain
}
