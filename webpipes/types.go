package webpipes

import "bufio"
import "fmt"
import "http"
import "io"
import "log"
import "os"
import "strconv"

// The http package requires that every http.Handler provide a single method:
//
// ServeHTTP(http.ResponseWriter, *http.Request)
//
// In this package, we provide this method for a sequence of components, i.e.
// sources, pipes and filters. We also provide a way to convert an
// http.Handler type into one that comforms to our desired semantics. We do
// this by taking in an http.Handler and providing a new http.ResponseWriter
// that prevents the write from going directly to the socket.

type Conn struct {
	Request *http.Request
	rwriter http.ResponseWriter
	body io.ReadCloser
	status int
	header map[string]string
	written int64
}

func NewConn(rwriter http.ResponseWriter, request *http.Request) *Conn {
	conn := new(Conn)
	conn.Request = request
	conn.rwriter = rwriter
	conn.header = make(map[string]string)
	return conn
}

func (c *Conn) NewContentWriter() io.WriteCloser {
	if c.body != nil {
		// There is a dangling reader that needs to be consumed first
		return nil
	}
	reader, writer := io.Pipe()
	c.body = reader
	return writer
}

func (c *Conn) NewContentReader() io.ReadCloser {
	if c.body == nil {
		// There is no reader waiting to be consumed
		return nil
	}
	reader := c.body
	c.body = nil
	return reader
}

// Unfortunately due to non-exported methods in the ResponseWriter, we need to
// track the state of headers outselves. This duplication is not great, but the
// overhead should be minimal

func (c *Conn) SetHeader(key, value string) {
	c.rwriter.SetHeader(key, value)
}

func (c *Conn) SetStatus(status int) {
	c.status = status
}

func (c *Conn) HTTPStatusResponse(status int) {
	c.SetHeader("Content-Type", "text/plain; charset=utf-8")
	c.SetStatus(status)

	writer := c.NewContentWriter()
	if writer == nil {
		// There was an application error, but we still want to send our
		// status response, so break the existing pipe and close it
		reader := c.NewContentReader()
		reader.Close()

		// Continue as normal, inserting our own content writer
		writer = c.NewContentWriter()
	}

	statusText := http.StatusText(status)
	if statusText == "" {
		statusText = "status code " + strconv.Itoa(status)
	}

	content := fmt.Sprintf("%s\n", statusText)

	go func(writer io.WriteCloser, content string) {
		io.WriteString(writer, content)
		writer.Close()
	}(writer, content)
}

// This function will forcibly close the underlying network connection and shut
// down the connection object. Once this has been called, the connection and
// the network should not be used at all

func (c *Conn) Close() {
	rwc, _, _ := c.Hijack()
	if rwc != nil {
		rwc.Close()
	}
}

// Enable a component to break encapsulation and get at the underlying network
// connections that correspond to the connection.

func (c *Conn) Hijack() (rwc io.ReadWriteCloser, buf *bufio.ReadWriter, err os.Error) {
	return c.rwriter.Hijack()
}

// A Component is a type that implements the HandleHTTPRequest method, which
// is subtly different from the ServeHTTP method required by the http.Handler
// interface. Specifically content is not written directly to the socket, but
// instead each component can request a 'content reader' or 'content writer'
// which it can use to output the content.
//
// This output should be done after the component has returned by spawning a
// new goroutine. This allows components to interact in a decoupled way.

type Component interface {
	HandleHTTPRequest(*Conn, *http.Request) bool
}

type Source func(*Conn, *http.Request, io.WriteCloser) bool
type Filter func(*Conn, *http.Request, io.ReadCloser, io.WriteCloser) bool
type Pipe func(*Conn, *http.Request) bool

func (fn Source) HandleHTTPRequest(c *Conn, req *http.Request) bool {
	// Allocate a content writer for this source
	writer := c.NewContentWriter()
	if writer == nil {
		// TODO: Output to error log here with relevant information
		c.HTTPStatusResponse(http.StatusInternalServerError)
		return true
	}

	return fn(c, req, writer)
}

func (fn Filter) HandleHTTPRequest(c *Conn, req *http.Request) bool {
	// Allocate new content reader/writer for the filter
	reader := c.NewContentReader()
	writer := c.NewContentWriter()

	if reader == nil || writer == nil {
		// TODO: Output to error log here with relevant information
		c.HTTPStatusResponse(http.StatusInternalServerError)
		return true
	}

	return fn(c, req, reader, writer)
}

func (fn Pipe) HandleHTTPRequest(c *Conn, req *http.Request) bool {
	return fn(c, req)
}

//////////////////////////////////////////////////////////////////////////////
// ResponseWriter adapter that allows you to use http.Handlers as Components
// in a webpipes system. This is a rather elaborate wrapper, but gives us the
// ability to re-use code rather than having to rewrite main functionaly such
// as serving files, etc.
//
// For the purposes of our code (and sanity) we refer to this as an 'adapter'
// throughout the package.

type HandlerRWAdapter struct {
	rwriter http.ResponseWriter // The response writer being wrapped
	done chan bool              // Channel to signal setup stage completion
	conn *Conn                  // The connection object (used to set status)
}

func (adapter *HandlerRWAdapter) RemoteAddr() string {
	return adapter.rwriter.RemoteAddr()
}

func (adapter *HandlerRWAdapter) UsingTLS() bool {
	return adapter.rwriter.UsingTLS()
}

func (adapter *HandlerRWAdapter) SetHeader(key, value string) {
	adapter.rwriter.SetHeader(key, value)
}

func (adapter *HandlerRWAdapter) Hijack() (io.ReadWriteCloser, *bufio.ReadWriter, os.Error) {
	// This should never happen, if so, developer needs to be notified
	panic("Handler called 'Hijack' on a HandlerComponent")
}

// This function is rather odd since it does not actually cause the headers to
// be written, instead it just signals to the component pipeline that the setup
// is complete and the next component can proceed. The headers will actually be
// written in the OutputPipe component in the same way it is always done. This
// function just signals down the done channel and uses the presence of this
// channel to indicate whether not this signalling has been done.

func (adapter *HandlerRWAdapter) WriteHeader(status int) {
	log.Printf("    [%p] WriteHeader was just called", adapter.conn)
	// This should only ever be called once, log an error message if this isn't the case
	if adapter.done == nil {
		log.Print("webpipes: multiple response.WriteHeader calls")
		return
	}

	adapter.conn.SetStatus(status)
	done := adapter.done
	adapter.done = nil
	done <- true
}

func (adapter *HandlerRWAdapter) Write(data []byte) (int, os.Error) {
	// If we haven't written headers yet, do so
	if adapter.done != nil {
		adapter.WriteHeader(http.StatusOK)
	}

	log.Printf("    [%p] Write called with %d bytes", adapter.conn, len(data))

	return adapter.rwriter.Write(data)
}

func (adapter *HandlerRWAdapter) Flush() {
	// If we haven't written headers yet, do so
	if adapter.done != nil {
		adapter.WriteHeader(http.StatusOK)
	}
	adapter.rwriter.Flush()
}

//////////////////////////////////////////////////////////////////////////////
// This is a type definition that allows us to convert an http.Handler into a
// webpipes.Component so it can be used in pipelines. This utilizes a
// HandlerRWAdapter to accomplish this.
//
// Each Handler is assumed to fill the role of source. This will NOT work if a
// handler hijacks the response, and will panic accordingly. This is a bit of a
// 'hack' to ensure we can reuse existing http package code.


type HandlerComponent struct {
	handler http.Handler
}

func NewHandlerComponent(h http.Handler) *HandlerComponent {
	hc := &HandlerComponent{handler: h}
	return hc
}

func (hc *HandlerComponent) HandleHTTPRequest(c *Conn, req *http.Request) bool {
	writer := c.NewContentWriter()
	if writer == nil {
		// TODO: What should happen here?
		c.HTTPStatusResponse(http.StatusInternalServerError)
		return true
	}

	// Handler logic is as follows:
	//   1. Set header using SetHeader
	//   2. Writer headers and status code using WriteHeader
	//   3. Output content using Write

	// What we need is actually
	//   1. Set header using SetHeader
	//   2. On first write or writeheader, return from this call so the next
	//      component can proceed; must ensure that any writes happen in a
	//      new goroutine.

	adapter := &HandlerRWAdapter{
		rwriter: c.rwriter,
		done: make(chan bool),
		conn: c,
	}

	go func() {
		// Run the handler
		hc.handler.ServeHTTP(adapter, req)
		// Writing is done, so close the cwriter
		writer.Close()
	}()

	// We need to wait for a Write or a WriteHeader on the adapter (which is
	// being used as the response writer for the handler invocation). Since
	// the handler is still running in a separate goroutine (the one above),
	// semantically the content generation is still working properly.
	<-adapter.done
	return true
}
