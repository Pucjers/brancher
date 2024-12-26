package handlers

import (
	"brancher/internal/services"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

type RequestBody struct {
	Token      string `json:"token"`
	BranchName string `json:"branch_name"`
}

type BranchHandler struct {
	BranchService *services.BranchService
}

func NewBranchHandler(branchService *services.BranchService) *BranchHandler {
	return &BranchHandler{BranchService: branchService}
}

func (h *BranchHandler) CheckTokenHandler(w http.ResponseWriter, r *http.Request) {

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

	if body.Token == "" || body.BranchName == "" {
		log.Println("Missing token or branch name in request")
		http.Error(w, "Token and branch name are required", http.StatusBadRequest)
		return
	}

	if err := h.BranchService.CreateGitBranch(body.BranchName, body.Token); err != nil {
		log.Printf("Failed to create branch: %v", err)
		http.Error(w, fmt.Sprintf("Failed to create branch: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	if err := h.BranchService.CreateServiceFile(body.BranchName); err != nil {
		log.Printf("Failed to create service file: %v", err)
		http.Error(w, fmt.Sprintf("Failed to create service file: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	if err := h.BranchService.UpdateAndStartService(body.BranchName); err != nil {
		log.Printf("Failed to start service: %v", err)
		http.Error(w, fmt.Sprintf("Failed to start service: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{
		"status": "success",
		"branch": h.BranchService.Config.BranchPrefix + body.BranchName,
	}); err != nil {
		log.Printf("Failed to write response: %v", err)
	}
}
