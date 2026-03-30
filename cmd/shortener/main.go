package main

//10
import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

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
