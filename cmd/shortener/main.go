package main

//10
import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func main() {
	parseFlags()

	store := NewInMemoryStorage()

	r := chi.NewRouter()

	r.Get("/{id}", apiGet(store))
	r.Post("/", apiPost(store))

	err := http.ListenAndServe(flagRunAddr, r)
	if err != nil {
		panic(err)
	}
}
