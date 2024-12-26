package handlers

import (
	"fmt"
	"log"
	"net/http"
)

func (h *BranchHandler) MergeMainHandler(w http.ResponseWriter, r *http.Request) {
	if err := h.BranchService.MergeMainToDirectories(); err != nil {
		log.Printf("Failed to merge main: %v", err)
		http.Error(w, fmt.Sprintf("Failed to merge main: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(`{"status": "success"}`)); err != nil {
		log.Printf("Failed to write response: %v", err)
		return
	}
}
