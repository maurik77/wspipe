package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
)

func main() {
	port := flag.Int("p", 8082, "listen port")
	flag.Parse()

	http.HandleFunc("/", manageRequest)
	listen := fmt.Sprintf(":%v", *port)
	http.ListenAndServe(listen, nil)
}

func manageRequest(w http.ResponseWriter, r *http.Request) {
	log.Printf("Http request: %v %v", r.Method, r.RequestURI)
	w.WriteHeader(http.StatusOK)
	bodyContent, err := io.ReadAll(r.Body)

	if len(bodyContent) == 0 || err != nil {
		w.Write([]byte("Hello!"))
	} else {
		w.Write(bodyContent)
	}
}
