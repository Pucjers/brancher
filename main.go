package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strings"

	"gopkg.in/yaml.v2"
)

type Config struct {
	GitHubAPIURL string `yaml:"github_api_url"`
	RepoOwner    string `yaml:"repo_owner"`
	RepoName     string `yaml:"repo_name"`
	GitHubToken  string `yaml:"github_token"`
	BranchPrefix string `yaml:"branch_prefix"`
}
type GitHubTree struct {
	Tree []struct {
		Path string `json:"path"`
		Type string `json:"type"`
		SHA  string `json:"sha"`
	} `json:"tree"`
}

type RequestBody struct {
	Token      string `json:"token"`
	BranchName string `json:"branch_name"`
}

var config Config

func main() {
	loadConfig()

	http.HandleFunc("/check-token", checkTokenHandler)
	http.HandleFunc("/merge-main", mergeMainHandler)
	http.HandleFunc("/delete-branch", deleteBranchHandler)

	log.Println("Server running on port 5000")
	log.Fatal(http.ListenAndServe(":5000", nil))
}

func loadConfig() {
	file, err := os.Open("config/config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config.yaml: %v", err)
	}
	defer file.Close()

	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		log.Fatalf("Failed to parse config.yaml: %v", err)
	}
}

func checkTokenHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	var body RequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if body.Token == "" || body.BranchName == "" {
		http.Error(w, "Token and branch name are required", http.StatusBadRequest)
		return
	}

	branchName := config.BranchPrefix + body.BranchName

	if err := createBranch(branchName, body.Token); err != nil {
		http.Error(w, fmt.Sprintf("Failed to create branch: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	if err := createServiceFile(branchName); err != nil {
		http.Error(w, fmt.Sprintf("Failed to create service file: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	if err := updateAndStartService(branchName); err != nil {
		http.Error(w, fmt.Sprintf("Failed to start service: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "success",
		"branch": branchName,
	})
}

func mergeMainHandler(w http.ResponseWriter, r *http.Request) {
	if err := mergeMainToDirectories(); err != nil {
		http.Error(w, fmt.Sprintf("Failed to merge main: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "success"}`))
}

func deleteBranchHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	var body RequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	branchName := config.BranchPrefix + body.BranchName

	if err := deleteBranchAndService(branchName); err != nil {
		http.Error(w, fmt.Sprintf("Failed to delete branch or service: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "success"}`))
}

func createBranch(branchName, token string) error {
	headers := map[string]string{
		"Authorization": fmt.Sprintf("token %s", config.GitHubToken),
		"Accept":        "application/vnd.github.v3+json",
	}

	baseBranchURL := fmt.Sprintf("%s/repos/%s/%s/git/ref/heads/main", config.GitHubAPIURL, config.RepoOwner, config.RepoName)
	baseBranchResp, err := makeRequest("GET", baseBranchURL, headers, nil)
	if err != nil {
		return fmt.Errorf("failed to get base branch: %w", err)
	}

	var baseBranchData map[string]interface{}
	json.Unmarshal(baseBranchResp, &baseBranchData)
	baseSHA := baseBranchData["object"].(map[string]interface{})["sha"].(string)

	createBranchURL := fmt.Sprintf("%s/repos/%s/%s/git/refs", config.GitHubAPIURL, config.RepoOwner, config.RepoName)
	createBranchPayload := map[string]string{
		"ref": fmt.Sprintf("refs/heads/%s", branchName),
		"sha": baseSHA,
	}

	_, err = makeRequest("POST", createBranchURL, headers, createBranchPayload)
	if err != nil {
		return fmt.Errorf("failed to create branch: %w", err)
	}

	treeURL := fmt.Sprintf("%s/repos/%s/%s/git/trees/%s?recursive=1", config.GitHubAPIURL, config.RepoOwner, config.RepoName, baseSHA)
	treeResp, err := makeRequest("GET", treeURL, headers, nil)
	if err != nil {
		return fmt.Errorf("failed to fetch repository tree: %w", err)
	}
	var tree GitHubTree
	json.Unmarshal(treeResp, &tree)

	for _, item := range tree.Tree {
		if item.Type == "blob" {
			if path.Base(item.Path) == "config.yaml" {
				if err := modifyConfig(item.Path, item.SHA, branchName, token, headers); err != nil {
					return err
				}
			} else {
				if err := copyFile(item.Path, item.SHA, branchName, headers); err != nil {
					return err
				}
			}
		}
	}

	if err := cloneBranchToDirectory(branchName); err != nil {
		return fmt.Errorf("failed to clone branch locally: %w", err)
	}
	dirName := fmt.Sprintf("repo-%s", strings.ReplaceAll(branchName, "/", "_"))
	buildCmd := fmt.Sprintf("cd %s && go build -o main main.go", dirName)

	log.Printf("Building main.go in directory '%s'...", dirName)
	if err := executeCommand(buildCmd); err != nil {
		return fmt.Errorf("failed to build main.go in '%s': %w", dirName, err)
	}

	log.Printf("Successfully built main.go in directory '%s'", dirName)

	return nil
}

func cloneBranchToDirectory(branchName string) error {
	dirName := fmt.Sprintf("repo-%s", strings.ReplaceAll(branchName, "/", "_"))

	repoURL := fmt.Sprintf("https://%s@github.com/%s/%s.git", config.GitHubToken, config.RepoOwner, config.RepoName)

	cloneCmd := fmt.Sprintf("git clone -b %s %s %s", branchName, repoURL, dirName)

	log.Printf("Cloning branch '%s' into directory '%s'...", branchName, dirName)

	if err := executeCommand(cloneCmd); err != nil {
		return fmt.Errorf("failed to clone branch '%s': %w", branchName, err)
	}

	log.Printf("Branch '%s' successfully cloned into '%s'", branchName, dirName)
	return nil
}

func modifyConfig(filePath, fileSHA, branchName, telegramToken string, headers map[string]string) error {
	fileURL := fmt.Sprintf("%s/repos/%s/%s/contents/%s?ref=%s", config.GitHubAPIURL, config.RepoOwner, config.RepoName, filePath, branchName)
	fileResp, err := makeRequest("GET", fileURL, headers, nil)
	if err != nil {
		return fmt.Errorf("failed to fetch file %s: %w", filePath, err)
	}

	var fileData map[string]interface{}
	json.Unmarshal(fileResp, &fileData)
	decodedContent, _ := base64.StdEncoding.DecodeString(fileData["content"].(string))

	var configFile map[string]interface{}
	yaml.Unmarshal(decodedContent, &configFile)
	configFile["bot_token"] = telegramToken
	configFile["redis_token"] = ""

	updatedContent, _ := yaml.Marshal(configFile)
	encodedContent := base64.StdEncoding.EncodeToString(updatedContent)

	commitURL := fmt.Sprintf("%s/repos/%s/%s/contents/%s", config.GitHubAPIURL, config.RepoOwner, config.RepoName, filePath)
	updatePayload := map[string]string{
		"message": fmt.Sprintf("Update %s in branch %s", filePath, branchName),
		"content": encodedContent,
		"sha":     fileSHA,
		"branch":  branchName,
	}
	_, err = makeRequest("PUT", commitURL, headers, updatePayload)
	if err != nil {
		return fmt.Errorf("failed to update %s: %w", filePath, err)
	}

	return nil
}

func copyFile(filePath, fileSHA, branchName string, headers map[string]string) error {
	fileURL := fmt.Sprintf("%s/repos/%s/%s/contents/%s?ref=main", config.GitHubAPIURL, config.RepoOwner, config.RepoName, filePath)
	fileResp, err := makeRequest("GET", fileURL, headers, nil)
	if err != nil {
		return fmt.Errorf("failed to fetch file %s: %w", filePath, err)
	}

	var fileData map[string]interface{}
	json.Unmarshal(fileResp, &fileData)
	encodedContent := fileData["content"].(string)

	commitURL := fmt.Sprintf("%s/repos/%s/%s/contents/%s", config.GitHubAPIURL, config.RepoOwner, config.RepoName, filePath)
	updatePayload := map[string]string{
		"message": fmt.Sprintf("Add %s to branch %s", filePath, branchName),
		"content": encodedContent,
		"sha":     fileSHA,
		"branch":  branchName,
	}
	_, err = makeRequest("PUT", commitURL, headers, updatePayload)
	if err != nil {
		return fmt.Errorf("failed to copy %s: %w", filePath, err)
	}

	return nil
}

func createServiceFile(branchName string) error {
	serviceFileContent := fmt.Sprintf(`[Unit]
Description=Service for branch %s
After=network.target
StartLimitIntervalSec=10

[Service]
Type=simple
ExecStart=/path/to/executable --branch %s
Restart=always
RestartSec=3
User=root

[Install]
WantedBy=multi-user.target
`, branchName, branchName)
	safeBranchName := strings.ReplaceAll(branchName, "/", "_")
	serviceFilePath := fmt.Sprintf("/etc/systemd/system/%s.service", safeBranchName)
	return ioutil.WriteFile(serviceFilePath, []byte(serviceFileContent), 0644)
}

func updateAndStartService(branchName string) error {
	safeBranchName := strings.ReplaceAll(branchName, "/", "_")
	cmds := []string{
		"systemctl daemon-reload",
		fmt.Sprintf("systemctl enable %s.service", safeBranchName),
		fmt.Sprintf("systemctl start %s.service", safeBranchName),
	}

	for _, cmd := range cmds {
		if err := executeCommand(cmd); err != nil {
			return err
		}
	}

	return nil
}

func mergeMainToDirectories() error {
	files, err := ioutil.ReadDir(".")
	if err != nil {
		return fmt.Errorf("failed to list directories: %w", err)
	}

	for _, file := range files {
		if file.IsDir() && strings.HasPrefix(file.Name(), "repo-") {
			dir := file.Name()
			branchName := strings.Replace(dir, "repo-", "", 1)
			safeBranchName := strings.ReplaceAll(branchName, "_", "/")
			log.Printf("Merging 'main' into branch '%s' in GitHub...", branchName)

			mergeURL := fmt.Sprintf("%s/repos/%s/%s/merges", config.GitHubAPIURL, config.RepoOwner, config.RepoName)
			payload := map[string]string{
				"base":           safeBranchName,
				"head":           "main",
				"commit_message": fmt.Sprintf("Merging 'main' into '%s'", safeBranchName),
			}

			headers := map[string]string{
				"Authorization": fmt.Sprintf("token %s", config.GitHubToken),
				"Accept":        "application/vnd.github.v3+json",
			}

			if _, err := makeRequest("POST", mergeURL, headers, payload); err != nil {
				log.Printf("Failed to merge 'main' into branch '%s': %v", safeBranchName, err)
			} else {
				log.Printf("Successfully merged 'main' into branch '%s'", safeBranchName)
			}

			log.Printf("Merging 'main' into local directory: %s", dir)
			mergeCmd := fmt.Sprintf("cd %s && git checkout main && git pull origin main && git checkout . && git merge main", dir)
			if err := executeCommand(mergeCmd); err != nil {
				log.Printf("Local merge failed for directory %s: %v", dir, err)
			} else {
				log.Printf("Successfully merged 'main' locally in directory %s", dir)
			}

			if err := buildMainFile(dir); err != nil {
				log.Printf("Failed to build main file in directory %s: %v", dir, err)
			} else {
				log.Printf("Successfully built main file in directory %s", dir)
			}
		}
	}

	return nil
}

func deleteBranchAndService(branchName string) error {
	headers := map[string]string{
		"Authorization": fmt.Sprintf("token %s", config.GitHubToken),
		"Accept":        "application/vnd.github.v3+json",
	}
	url := fmt.Sprintf("%s/repos/%s/%s/git/refs/heads/%s", config.GitHubAPIURL, config.RepoOwner, config.RepoName, branchName)
	_, err := makeRequest("DELETE", url, headers, nil)
	if err != nil {
		return fmt.Errorf("failed to delete branch %s: %w", branchName, err)
	}

	safeBranchName := strings.ReplaceAll(branchName, "/", "_")
	cmds := []string{
		fmt.Sprintf("systemctl stop %s.service", safeBranchName),
		fmt.Sprintf("systemctl disable %s.service", safeBranchName),
		fmt.Sprintf("rm /etc/systemd/system/%s.service", safeBranchName),
		"systemctl daemon-reload",
	}
	for _, cmd := range cmds {
		if err := executeCommand(cmd); err != nil {
			return fmt.Errorf("failed to execute %s: %w", cmd, err)
		}
	}

	// Delete cron job
	// cronFilePath := fmt.Sprintf("/etc/cron.d/%s", branchName)
	// if err := os.Remove(cronFilePath); err != nil && !os.IsNotExist(err) {
	// 	return fmt.Errorf("failed to delete cron file: %w", err)
	// }

	return nil
}

func buildMainFile(directory string) error {
	mainFilePath := path.Join(directory, "main")
	if err := os.Remove(mainFilePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove old binary: %w", err)
	}

	buildCmd := fmt.Sprintf("cd %s && go build -o main main.go", directory)
	if err := executeCommand(buildCmd); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	log.Printf("New binary built at %s", mainFilePath)
	return nil
}

func executeCommand(command string) error {
	cmd := exec.Command("bash", "-c", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("command failed: %s, output: %s", err.Error(), string(output))
	}
	return nil
}

func makeRequest(method, url string, headers map[string]string, payload interface{}) ([]byte, error) {
	var reqBody *strings.Reader
	if payload != nil {
		jsonPayload, _ := json.Marshal(payload)
		reqBody = strings.NewReader(string(jsonPayload))
	} else {
		reqBody = strings.NewReader("")
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, err
	}

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}
