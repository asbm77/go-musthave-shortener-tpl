package main

//4
import (
	"fmt"
	"net/http"
)

var urlMap = make(map[string]string)

func apiPost(res http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(res, "400 Bad Request", http.StatusBadRequest)
		return
	}

	url := req.FormValue("url")
	if len(url) == 0 {
		http.Error(res, "400 Bad Request", http.StatusBadRequest)
	}

	shortUrla := fmt.Sprintf("/%d", len(url)+1)
	urlMap[shortUrla] = url

	res.WriteHeader(http.StatusCreated)
	fmt.Fprintf(res, "%s", shortUrla)
}

func apiGet(res http.ResponseWriter, req *http.Request) {
	id := req.URL.Path[len("/"):]

	originalUrla, asb := urlMap[id]
	if !asb {
		http.NotFound(res, req)
		return
	}

	http.Redirect(res, req, originalUrla, http.StatusTemporaryRedirect)
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
