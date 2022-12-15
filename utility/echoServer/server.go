package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

func main() {
	port := flag.Int("p", 8082, "listen port")
	flag.Parse()

	http.HandleFunc("/", manageRequest)
	listen := fmt.Sprintf(":%v", *port)

	server := &http.Server{
		Addr:              listen,
		ReadHeaderTimeout: 3 * time.Second,
	}

	err := server.ListenAndServe()

	if err != nil {
		log.Print("ListenAndServe")
	}
}

func manageRequest(w http.ResponseWriter, r *http.Request) {
	log.Printf("Http request: %v %v", r.Method, r.RequestURI)
	w.WriteHeader(http.StatusOK)
	bodyContent, err := io.ReadAll(r.Body)

	if len(bodyContent) == 0 || err != nil {
		_, err = w.Write([]byte("Hello!"))
	} else {
		_, err = w.Write(bodyContent)
	}

	if err != nil {
		log.Print("Write body response error")
	}
}
