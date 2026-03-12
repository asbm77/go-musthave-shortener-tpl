package main

//4
import (
	"fmt"
	"net/http"
)

//var urlMap = make(map[string]string)

func apiPost(res http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(res, "400 Bad Request", http.StatusBadRequest)
		return
	}

	url := req.FormValue("url")
	if len(url) == 0 {
		http.Error(res, "400 Bad Request", http.StatusBadRequest)
	}

	shortUrl := fmt.Sprintf("/%d", len(url)+1)

	res.WriteHeader(http.StatusCreated)
	fmt.Fprintf(res, "%s", shortUrl)
}

func apiGet(res http.ResponseWriter, req *http.Request) {
	url := req.URL.Path[len("/"):]

	if url == "" {
		http.NotFound(res, req)
		return
	}

}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(res http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodPost:
			apiPost(res, req)
		case http.MethodGet:

			apiGet(res, req)
		default:
			http.Error(res, "400 Bad Request", http.StatusBadRequest)
		}
	})

	err := http.ListenAndServe(`:8080`, mux)
	if err != nil {
		panic(err)
	}
}
