package webpipes

import "http"
import "io"

func FileServer(root, prefix string) Component {
	return NewHandlerComponent(http.FileServer(root, prefix))
}

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

func CGISource(filename, prefix string) Component {
	return &CGIComponent{filename: filename, prefix: prefix, dir: false}
}

func CGIDirSource(directory, prefix string) Component {
	return &CGIComponent{filename: directory, prefix: prefix, dir: true}
}
