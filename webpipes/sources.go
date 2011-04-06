package webpipes

import "http"
import "io"

// Serve files from 'root', stripping 'prefix' from the URL being requested
func FileServer(root, prefix string) Component {
	return NewHandlerComponent(http.FileServer(root, prefix))
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

// Serve a CGI script 'filename', stripping 'prefix' from the URL
func CGISource(filename, prefix string) Component {
	return &CGIComponent{filename: filename, prefix: prefix, dir: false}
}

// Serve CGI scripts from 'directory', stripping 'prefix' from the URL
func CGIDirSource(directory, prefix string) Component {
	return &CGIComponent{filename: directory, prefix: prefix, dir: true}
}
