package webpipes

import "bufio"
import "fmt"
import "http"
import "io"
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

// TODO: Implement this function
func (c *Conn) Close() {
}

// TODO: Implement this function
func (c *Conn) Hijack() {
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

// HandlerComponent is a wrapper type that converts an http.Handler into a
// Component for use in this package. Each Handler is assumed to fill the
// role of source. This will NOT work if a handler hijacks the response,
// and will panic accordingly. This is a bit of a 'hack' to ensure we can
// reuse existing http package code.
//
// It does not technically violate the http.ResponseWriter interface, it
// just models it with an infinite buffer that is not written immediately.
//
// This type implements the http.ResponseWriter interface, to avoid having
// yet another wrapper type.
type HandlerComponent struct {
	handler http.Handler
	conn *Conn
	rwriter http.ResponseWriter
	cwriter io.WriteCloser
	done chan bool
	wroteHeaders bool
}

func (hc *HandlerComponent) HandleHTTPRequest(c *Conn, r *http.Request) bool {
	writer := c.NewContentWriter()
	if writer == nil {
		// TODO: What should happen here?
		c.HTTPStatusResponse(http.StatusInternalServerError)
		return true
	}

	hc.conn = c
	hc.rwriter = c.rwriter
	hc.cwriter = writer
	hc.done = make(chan bool)
	hc.wroteHeaders = false

	// Handler logic is as follows:
	//   1. Set header using SetHeader
	//   2. Writer headers and status code using WriteHeader
	//   3. Output content using Write

	// What we need is actually
	//   1. Set header using SetHeader
	//   2. On first write or writeheader, return so next component can proceed
	//      but ensure that writes happen in a new goroutine.

	// Run the handler in a new goroutine and rely on the specified semantics
	// to ensure it works properly.
	go func() {
		// Run the handler
		hc.handler.ServeHTTP(hc, r)
		// At this point we need to close the cwriter
		hc.cwriter.Close()
	}()

	// Wait for either a Write or a WriteHeader, and return. Since the handler is
	// still running in a separate goroutine, writes will happen as we would expect
	// them to.
	<-hc.done
	return true
}

func NewHandlerComponent(h http.Handler) *HandlerComponent {
	hc := new(HandlerComponent)
	hc.handler = h
	return hc
}

func (hc *HandlerComponent) RemoteAddr() string { return hc.rwriter.RemoteAddr() }
func (hc *HandlerComponent) UsingTLS() bool { return hc.rwriter.UsingTLS() }
func (hc *HandlerComponent) SetHeader(key, value string) { hc.rwriter.SetHeader(key, value)}

func (hc *HandlerComponent) Hijack() (io.ReadWriteCloser, *bufio.ReadWriter, os.Error) {
	// This should never happen, if so, developer needs to be notified
	panic("Handler called 'Hijack' on a HandlerComponent")
}

func (hc *HandlerComponent) Write(data []byte) (int, os.Error) {
	if !hc.wroteHeaders {
		// The headers need to be written with the appropriate status code
		hc.WriteHeader(http.StatusOK)
	}

	return hc.cwriter.Write(data)
}

func (hc *HandlerComponent) WriteHeader(status int) {
	hc.conn.status = status
	hc.wroteHeaders = true
	hc.done <- true
}

func (hc *HandlerComponent) Flush() {
	if !hc.wroteHeaders {
		hc.WriteHeader(http.StatusOK)
	}
	hc.rwriter.Flush()
}

