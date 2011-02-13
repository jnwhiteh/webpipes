package main

import "http"
import "io"
import "log"
import "runtime"
import "webpipes"

var helloworld string = "Hello, world!\n"

func HelloServer(w http.ResponseWriter, req *http.Request) {
	w.SetHeader("Content-Type", "text/plain; charset=utf-8")
	io.WriteString(w, helloworld)
}

func GCServer(w http.ResponseWriter, req *http.Request) {
}

func MemStatsServer(w http.ResponseWriter, req *http.Request) {
}

func DebugPipe(prefix string) webpipes.Pipe {
	return func(conn *webpipes.Conn, req *http.Request) bool {
		log.Printf("%s [%p] Request for URL: %s", prefix, conn, req.URL.Path)
		return true
	}
}

func main() {
	// Debug URLs to reload or restart the system
	http.Handle("/debug/gc", http.HandlerFunc(GCServer))
	http.Handle("/debug/memstats", http.HandlerFunc(MemStatsServer))

	// Go 'http' package
	http.Handle("/go/hello", http.HandlerFunc(HelloServer))
	http.Handle("/go/example/", http.FileServer("../http-data", "/go"))
	http.Handle("/go/ipsum.txt", http.FileServer("../http-data", "/go"))

	// Webpipes with Erlang chains
	http.Handle("/webpipe/erlang/hello", webpipes.ErlangChain(
		webpipes.TextStringSource(helloworld),
		webpipes.OutputPipe,
	))
	http.Handle("/webpipe/erlang/example/", webpipes.ErlangChain(
		webpipes.FileServer("../http-data", "/webpipe/erlang"),
		webpipes.OutputPipe,
	))
	http.Handle("/webpipe/erlang/ipsum.txt", webpipes.ErlangChain(
		webpipes.FileServer("../http-data", "/webpipe/erlang"),
		webpipes.OutputPipe,
	))

	// Webpipes with Proc chains
	http.Handle("/webpipe/proc/hello", webpipes.ProcChain(nil, nil,
		webpipes.TextStringSource(helloworld),
		webpipes.OutputPipe,
	))
	http.Handle("/webpipe/proc/example/", webpipes.ProcChain(nil, nil,
		webpipes.FileServer("../http-data", "/webpipe/proc"),
		webpipes.OutputPipe,
	))
	http.Handle("/webpipe/proc/ipsum.txt", webpipes.ProcChain(nil, nil,
		webpipes.FileServer("../http-data", "/webpipe/proc"),
		webpipes.OutputPipe,
	))

	address := ":12345"
	log.Printf("Starting test server on %s", address)
	log.Printf("Running on %d processes\n", runtime.GOMAXPROCS(0))
	err := http.ListenAndServe(address, nil)
	if err != nil {
		log.Fatalf("Error: %s", err.String())
	}
}
