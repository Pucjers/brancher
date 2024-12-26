package main

import (
	"brancher/config"
	"brancher/internal/handlers"
	"brancher/internal/services"
	"log"
	"net/http"
)

func main() {
	config, err := config.LoadConfig()
	if err != nil || config == nil {
		log.Fatalf("Error loading configuration: %v", err)
	}

	branchService := services.NewBranchService(config)
	branchHandler := handlers.NewBranchHandler(branchService)

	http.HandleFunc("/check-token", branchHandler.CheckTokenHandler)
	http.HandleFunc("/merge-main", branchHandler.MergeMainHandler)
	http.HandleFunc("/delete-branch", branchHandler.DeleteBranchHandler)

	log.Println("Server running on port 5000")
	if err := http.ListenAndServe(":5000", nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
