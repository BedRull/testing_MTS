package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"golang.org/x/net/netutil"
)

const (
	serverAddress          = ":8080"
	serverTimeout          = 10 * time.Second
	serverConnectionsLimit = 100
	clientTimeout          = 500 * time.Millisecond
)

type URLs struct {
	List []string `json:"urls"`
}

type ServerResponse struct {
	URL  string `json:"URL"`
	Data []byte `json:"Data"`
}

func main() {
	// configuring server
	serv := http.Server{
		Addr:        serverAddress,
		ReadTimeout: serverTimeout,
	}

	// handler
	http.HandleFunc("/", HandlerGetData())

	// limit connections with listener
	listener, err := net.Listen("tcp", serv.Addr)
	if err != nil {
		log.Fatal(err)
	}

	defer listener.Close()

	listener = netutil.LimitListener(listener, serverConnectionsLimit)

	// run server
	go func() {
		err := serv.Serve(listener)
		if err != http.ErrServerClosed {
			log.Fatalf("Serve error: %v", err)
		}
	}()

	// wait for signal from os
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop

	ctxWithTimeout, cancel := context.WithTimeout(context.Background(), 10*clientTimeout)

	defer cancel()

	err = serv.Shutdown(ctxWithTimeout)
	if err != nil {
		log.Fatal("Server shutdown failed:", err.Error())
	}
}

func HandlerGetData() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}

		// read request body
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.Println("reading body error:", err)

			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("Invalid body"))

			return
		}

		defer r.Body.Close()

		urls := URLs{}

		// unmarshalling body
		err = json.Unmarshal(body, &urls)
		if err != nil {
			log.Println("marshalling body error:", err)

			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("Error marshalling body"))

			return
		}

		if len(urls.List) > 20 {
			log.Println("Too many urls error")

			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("Total urls count limited to 20."))

			return
		}

		responses := []ServerResponse{}

		client := &http.Client{Timeout: clientTimeout}

		// run through list of urls
		for _, url := range urls.List {
			// get response from each url
			response, err := GetResponse(client, url)
			if err != nil {
				log.Println(err)

				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(err.Error()))

				return
			}

			responses = append(responses, ServerResponse{
				URL:  url,
				Data: response,
			})
		}

		// marshal general response
		response, err := json.Marshal(responses)
		if err != nil {
			log.Println("marshalling response error:", err)

			w.WriteHeader(http.StatusInternalServerError)
		}

		// write response
		_, _ = w.Write(response)
	}
}

func GetResponse(client *http.Client, url string) ([]byte, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("getting %s error: %v", url, err)
	}

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading %s response body error: %v", url, err)
	}

	defer resp.Body.Close()

	return respBody, nil
}
