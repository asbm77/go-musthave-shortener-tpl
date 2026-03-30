package main

import (
	"fmt"
	"io"
	"log"
	"net/http"

	//"shortener/storage"

	"github.com/google/uuid"
)

//var urlMap = make(map[string]string)

func apiPost(store Storage) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
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

		defer req.Body.Close()

		url := string(body)
		shortUrla := "/" + uuid.NewString()[:8]
		//urlMap[shortUrla] = url
		shortUrlares := flagShortAddr + shortUrla

		if err := store.Set(shortUrla, url); err != nil {
			http.Error(res, "Internal Server Error", http.StatusInternalServerError)
			return

		}

		res.WriteHeader(http.StatusCreated)
		fmt.Fprintf(res, "%s", shortUrlares)

	}
}

func apiGet(store Storage) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {

		id := req.URL.Path

		originalURL, err := store.Get(id)
		if err != nil || originalURL == "" {
			if err == ErrNotFound {
				http.NotFound(res, req)
				return
			}
			http.Error(res, "Internal Server Error", http.StatusInternalServerError)
			log.Printf("Ошибка получения из хранилища: %v", err)
			return
		}

		http.Redirect(res, req, originalURL, http.StatusTemporaryRedirect)
	}
}
