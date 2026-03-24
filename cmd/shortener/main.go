package main

//8
import (
	"fmt"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
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
	r := chi.NewRouter()

	r.Get("/", apiGet)
	r.Post("/", apiPost)

	err := http.ListenAndServe(`localhost:8080`, r)
	if err != nil {
		panic(err)
	}
}
