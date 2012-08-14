# Webpipes

Webpipes is a process-oriented, compositional web server toolkit written using
the Go programming language. This toolkit utilizes utilizes an architecture
where multiple functional components respond to requests, rather than the
traditional monolithic web server model. The abstractions provided by this
toolkit allow servers to be deployed using several concurrency strategies.

## Paper

This toolkit is presented in detail by the paper "Serving Web Content with
Dynamic Process Networks in Go". You can find a copy of this paper on the
[Communicating Process Architectures 2011 conference website][1].

[1]: http://www.wotug.org/paperdb/show_proc.php?f=4&num=28

## Usage

You can goinstall the package by running:

     go get -u -v github.com/jnwhiteh/webpipes

Once the package is installed you can import it using the same path:

     import github.com/jnwhiteh/webpipes

A "Hello World" server might look like this:

     package main
     
     import "github.com/jnwhiteh/webpipes"
     import "net/http"
     import "log"
     
     func main() {
     	http.Handle("/", webpipes.Chain(
     		webpipes.TextStringSource("Hello, World!"),
     		webpipes.OutputPipe,
     	))
     	server := &http.Server{
     		Addr: ":12345",
     		Handler: http.DefaultServeMux,
     	}
     
     	log.Printf("Starting server on %s", server.Addr)
     	err := server.ListenAndServe()
     	if err != nil {
     		log.Fatalf("Error: %s", err)
     	}
     }



