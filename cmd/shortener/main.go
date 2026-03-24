package main

//9
import (
	"fmt"
	"io"
	"net/http"
	"net/url"
)

var urlMap = make(map[string]string)

func apiPost(res http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(res, "400 Bad Request", http.StatusBadRequest)
		return
	}

	//url := req.FormValue("url")
	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(res, "400 Bad Request", http.StatusBadRequest)
		return
	}
	url := string(body)
	shortUrla := fmt.Sprintf("/%d", len(url)+1)
	urlMap[shortUrla] = url
	shortUrlares := "http://" + req.Host + fmt.Sprintf("/%d", len(url)+1)

	res.WriteHeader(http.StatusCreated)
	fmt.Fprintf(res, "%s", shortUrlares)

}

func apiGet(res http.ResponseWriter, req *http.Request) {

	id := req.URL.Path

	asb := urlMap[id]
	if asb == "" {
		http.NotFound(res, req)
		return
	}

	http.Redirect(res, req, urlMap[id], http.StatusTemporaryRedirect)
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(res http.ResponseWriter, req *http.Request) {

		rawURL := req.URL.String()
		_, err := url.ParseRequestURI(rawURL)
		if err != nil {
			http.Error(res, "400 Bad Request", http.StatusBadRequest)
			return
		}

		switch req.Method {
		case http.MethodPost:
			apiPost(res, req)
		case http.MethodGet:
			apiGet(res, req)
		default:
			http.Error(res, "400 Bad Request", http.StatusBadRequest)
		}
	})

	err := http.ListenAndServe(`localhost:8080`, mux)
	if err != nil {
		panic(err)
	}
}
