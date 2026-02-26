package main

import (
	"fmt"
	"net/http"
)

// helloHandler responds with "hello world" for GET /hello requests.
func helloHandler(responseWriter http.ResponseWriter, request *http.Request) {
	fmt.Fprintln(responseWriter, "hello world")
}

func main() {
	http.HandleFunc("/hello", helloHandler)

	fmt.Println("Server listening on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Printf("Server error: %v\n", err)
	}
}

