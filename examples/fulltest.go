package main

import "fmt"
import "http"
import "io"
import "log"
import "os"
import "path"
import "runtime"
import "webpipes"

var helloworld string = "Hello, world!\n"

func HelloServer(w http.ResponseWriter, req *http.Request) {
	w.SetHeader("Content-Type", "text/plain; charset=utf-8")
	io.WriteString(w, helloworld)
}

func GCServer(w http.ResponseWriter, req *http.Request) {
	w.SetHeader("Content-Type", "text/plain; charset=utf-8")
	StatsServer(w, req)
	io.WriteString(w, "=============================\n")
	runtime.GC()
	StatsServer(w, req)
}

func StatsServer(w http.ResponseWriter, req *http.Request) {
	stats := runtime.MemStats

	w.SetHeader("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "Total allocated: %d bytes, In-use: %d bytes\n", stats.TotalAlloc, stats.Alloc)
	fmt.Fprintf(w, "Heap in-use: %d bytes, Number of heap objects: %d\n", stats.HeapAlloc, stats.HeapObjects)
	fmt.Fprintf(w, "There are %d Goroutines in the system\n", runtime.Goroutines())
}

func DebugPipe(prefix string) webpipes.Pipe {
	return func(conn *webpipes.Conn, req *http.Request) bool {
		log.Printf("%s [%p] Request for URL: %s", prefix, conn, req.URL.Path)
		return true
	}
}

func LimitNetwork(limit int, components ...webpipes.Component) http.Handler {
	in := make(chan *webpipes.Conn, limit)
	out := make(chan *webpipes.Conn, limit)
	chain, _, _ := webpipes.ProcChainInOut(in, out, components...)
	return chain
}

func main() {
	// Debug URLs to reload or restart the system
	http.Handle("/debug/gc", http.HandlerFunc(GCServer))
	http.Handle("/debug/stats", http.HandlerFunc(StatsServer))

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
	http.Handle("/webpipe/proc/hello", LimitNetwork(500,
		webpipes.TextStringSource(helloworld),
		webpipes.OutputPipe,
	))
	http.Handle("/webpipe/proc/example/", LimitNetwork(500,
		webpipes.FileServer("../http-data", "/webpipe/proc"),
		webpipes.OutputPipe,
	))
	http.Handle("/webpipe/proc/ipsum.txt", LimitNetwork(500,
		webpipes.FileServer("../http-data", "/webpipe/proc"),
		webpipes.OutputPipe,
	))

	// Construct a process network that limits the number of concurrent connections
	// Webpipes with LIMITED Proc chains
	http.Handle("/webpipe/lproc/hello", webpipes.ProcChain(
		webpipes.TextStringSource(helloworld),
		webpipes.OutputPipe,
	))
	http.Handle("/webpipe/lproc/example/", webpipes.ProcChain(
		webpipes.FileServer("../http-data", "/webpipe/proc"),
		webpipes.OutputPipe,
	))
	http.Handle("/webpipe/lproc/ipsum.txt", webpipes.ProcChain(
		webpipes.FileServer("../http-data", "/webpipe/proc"),
		webpipes.OutputPipe,
	))

	// CGI Examples
	pwd, pwderr := os.Getwd()
	if pwderr != nil {
		log.Fatalf("Cannot find pwd: %s", pwderr)
	}

	cgipath := path.Clean(path.Join(pwd, "../http-data/cgi-bin"))
	http.Handle("/cgi-bin/", webpipes.ErlangChain(
		webpipes.CGIDirSource(cgipath, "/cgi-bin"),
		webpipes.OutputPipe,
	))

	http.Handle("/wiki/", webpipes.ErlangChain(
		webpipes.CGISource("/tmp/gorows-sputnik/sputnik.cgi", "/wiki/"),
		webpipes.OutputPipe,
	))

	http.Handle("/zip/", webpipes.ErlangChain(
		webpipes.FileServer("../http-data", "/zip/"),
		webpipes.CompressionPipe,
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
