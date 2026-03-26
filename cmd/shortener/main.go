package main

//10
import (
	"fmt"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
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
	shortUrla := "/" + uuid.NewString()[:8]
	urlMap[shortUrla] = url
	shortUrlares := flagShortAddr + "/" + shortUrla

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
	parseFlags()

	r := chi.NewRouter()

	r.Get("/{id}", apiGet)
	r.Post("/", apiPost)

	err := http.ListenAndServe(flagRunAddr, r)
	if err != nil {
		panic(err)
	}
}
