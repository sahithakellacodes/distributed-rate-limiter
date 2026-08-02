package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type Product struct {
	ID    int     `json:"id"`
	Name  string  `json:"name"`
	Price float64 `json:"price"`
}

func main() {
	http.HandleFunc("/products", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		products := []Product{
			{ID: 1, Name: "MacBook Air", Price: 999.99},
			{ID: 2, Name: "PlayStation 5", Price: 499.99},
			{ID: 3, Name: "Coffee", Price: 4.99},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(products); err != nil {
			fmt.Println("failed to encode response:", err)
		}
	})

	fmt.Println("Backend listening on :8081")

	if err := http.ListenAndServe(":8081", nil); err != nil {
		panic(err)
	}
}