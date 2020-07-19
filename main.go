package main

import (
	"net/http"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
)

var fileName *string
var port *string
var contentType *string

func serveFile (w http.ResponseWriter, req *http.Request) {
	fmt.Println("[INFO]Opening " + *fileName)
	fileContents, err := ioutil.ReadFile(*fileName)
	if err != nil {
		fmt.Println("[FATAL]Unable to load file "+*fileName+": ", err)
		os.Exit(1)
	}
	w.Header().Set("Content-Type", *contentType)
	fmt.Fprintf(w, string(fileContents))
}

func main () {
	port = flag.String("port", "8080", "The port to listen on")
	fileName = flag.String("file", "", "File to serve")
	contentType = flag.String("contentType", "text/plain", "The content type to put into the Content-Type header")
	flag.Parse()
	http.HandleFunc("/", serveFile)
	err := http.ListenAndServe(":"+*port, nil)
	if err != nil {
		fmt.Println("[FATAL]Unable to serve on port "+*port, err)
	}
}
