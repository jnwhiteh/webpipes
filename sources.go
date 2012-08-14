package webpipes

import "net/http"
import "net/http/cgi"
import "io"

// Serve files from 'root', stripping 'prefix' from the URL being requested
func FileServer(root, prefix string) Component {
	handler := http.StripPrefix(prefix, http.FileServer(http.Dir(root)))
	return NewHandlerComponent(handler)
}

// Serve a CGI application 'path', stripping 'prefix' from the URL being
// requested
func CGIServer(path, prefix string) Component {
	handler := new(cgi.Handler)
	handler.Path = path
	handler.Root = prefix
	return NewHandlerComponent(handler)
}

// Respond with a string as text/plain output
func TextStringSource(str string) Source {
	return func(conn *Conn, req *http.Request, writer io.WriteCloser) bool {
		conn.status = http.StatusOK
		conn.SetHeader("Content-type", "text/plain; charset=utf-8")
		go func() {
			io.WriteString(writer, str)
			writer.Close()
		}()

		return true
	}
}
