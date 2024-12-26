package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

func (h *BranchHandler) DeleteBranchHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		log.Printf("Invalid request method: %s", r.Method)
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	var body RequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		log.Printf("Invalid JSON body: %v", err)
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if err := h.BranchService.DeleteBranchAndService(body.BranchName); err != nil {
		log.Printf("Failed to delete branch or service: %v", err)
		http.Error(w, fmt.Sprintf("Failed to delete branch or service: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(`{"status": "success"}`)); err != nil {
		log.Printf("Failed to write response: %v", err)
	}
}
